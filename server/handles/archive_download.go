package handles

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	stdpath "path"
	"path/filepath"
	"strings"
	"time"

	"github.com/OpenListTeam/OpenList/v4/cmd/flags"
	"github.com/OpenListTeam/OpenList/v4/internal/conf"
	"github.com/OpenListTeam/OpenList/v4/internal/errs"
	"github.com/OpenListTeam/OpenList/v4/internal/fs"
	"github.com/OpenListTeam/OpenList/v4/internal/model"
	"github.com/OpenListTeam/OpenList/v4/internal/op"
	"github.com/OpenListTeam/OpenList/v4/pkg/http_range"
	"github.com/OpenListTeam/OpenList/v4/pkg/utils"
	"github.com/OpenListTeam/OpenList/v4/server/common"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

const (
	maxArchiveSize int64 = 500 * 1024 * 1024
)

type ArchiveDownloadReq struct {
	Path     string `json:"path" form:"path"`
	Password string `json:"password" form:"password"`
	Format   string `json:"format" form:"format"`
	Confirm  bool   `json:"confirm" form:"confirm"`
}

func getArchiveTimeout() time.Duration {
	if flags.ArchiveTimeout > 0 {
		return flags.ArchiveTimeout
	}
	return 30 * time.Second
}

func formatFileSize(size int64) string {
	if size < utils.KB {
		return fmt.Sprintf("%d B", size)
	}
	if size < utils.MB {
		return fmt.Sprintf("%.2f KB", float64(size)/float64(utils.KB))
	}
	if size < utils.GB {
		return fmt.Sprintf("%.2f MB", float64(size)/float64(utils.MB))
	}
	if size < utils.TB {
		return fmt.Sprintf("%.2f GB", float64(size)/float64(utils.GB))
	}
	return fmt.Sprintf("%.2f TB", float64(size)/float64(utils.TB))
}

func estimateDirSize(ctx context.Context, reqPath string) (int64, error) {
	var totalSize int64
	info, err := fs.Get(ctx, reqPath, &fs.GetArgs{NoLog: true})
	if err != nil {
		return 0, err
	}
	if !info.IsDir() {
		return info.GetSize(), nil
	}
	err = fs.WalkFS(ctx, -1, reqPath, info, func(path string, obj model.Obj) error {
		if !obj.IsDir() {
			totalSize += obj.GetSize()
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return totalSize, nil
}

func ArchiveDownload(c *gin.Context) {
	var req ArchiveDownloadReq
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	req.Format = strings.ToLower(strings.TrimSpace(req.Format))
	if req.Format == "" {
		req.Format = "zip"
	}
	if req.Format != "zip" && req.Format != "tar.gz" && req.Format != "tgz" {
		common.ErrorStrResp(c, "unsupported format, use zip or tar.gz", 400)
		return
	}

	user := c.Request.Context().Value(conf.UserKey).(*model.User)
	if user.IsGuest() && user.Disabled {
		common.ErrorStrResp(c, "Guest user is disabled, login please", 401)
		return
	}

	reqPath, err := user.JoinPath(req.Path)
	if err != nil {
		common.ErrorResp(c, err, 403)
		return
	}

	meta, err := op.GetNearestMeta(reqPath)
	if err != nil && !errors.Is(errors.Cause(err), errs.MetaNotFound) {
		common.ErrorResp(c, err, 500, true)
		return
	}
	common.GinAppendValues(c, conf.MetaKey, meta)

	if !common.CanAccess(user, meta, reqPath, req.Password) {
		common.ErrorStrResp(c, "password is incorrect or you have no permission", 403)
		return
	}

	info, err := fs.Get(c.Request.Context(), reqPath, &fs.GetArgs{})
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	if !info.IsDir() {
		common.ErrorStrResp(c, "path is not a directory", 400)
		return
	}

	totalSize, err := estimateDirSize(c.Request.Context(), reqPath)
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}

	if totalSize > maxArchiveSize && !req.Confirm {
		msg := fmt.Sprintf("directory size is %s, exceeds 500MB limit, add confirm=true to proceed", formatFileSize(totalSize))
		common.ErrorWithDataResp(c, errors.New(msg), 402, gin.H{
			"size":      totalSize,
			"size_str":  formatFileSize(totalSize),
			"limit":     maxArchiveSize,
			"limit_str": formatFileSize(maxArchiveSize),
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), getArchiveTimeout())
	defer cancel()

	baseName := stdpath.Base(reqPath)
	var fileName string
	var contentType string
	if req.Format == "zip" {
		fileName = baseName + ".zip"
		contentType = "application/zip"
	} else {
		fileName = baseName + ".tar.gz"
		contentType = "application/gzip"
	}

	c.Header("Content-Disposition", utils.GenerateContentDisposition(fileName))
	c.Header("Content-Type", contentType)
	c.Header("Cache-Control", "max-age=0, no-cache, no-store, must-revalidate")

	c.Status(200)

	if req.Format == "zip" {
		err = streamZipArchive(ctx, c.Writer, reqPath)
	} else {
		err = streamTarGzArchive(ctx, c.Writer, reqPath)
	}

	if err != nil {
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			log.Errorf("archive download error: %+v", err)
		}
	}
}

func streamZipArchive(ctx context.Context, w io.Writer, reqPath string) error {
	zw := zip.NewWriter(w)
	defer func() {
		if err := zw.Close(); err != nil {
			log.Warnf("zip writer close error: %v", err)
		}
	}()

	info, err := fs.Get(ctx, reqPath, &fs.GetArgs{NoLog: true})
	if err != nil {
		return err
	}

	return fs.WalkFS(ctx, -1, reqPath, info, func(path string, obj model.Obj) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if obj.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(reqPath, path)
		if err != nil {
			return err
		}
		relPath = strings.ReplaceAll(relPath, "\\", "/")

		header := &zip.FileHeader{
			Name:     relPath,
			Method:   zip.Deflate,
			Modified: obj.ModTime(),
		}
		header.SetMode(0644)

		fw, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}

		return streamFileContent(ctx, fw, path)
	})
}

func streamTarGzArchive(ctx context.Context, w io.Writer, reqPath string) error {
	gw := gzip.NewWriter(w)
	defer func() {
		if err := gw.Close(); err != nil {
			log.Warnf("gzip writer close error: %v", err)
		}
	}()

	tw := tar.NewWriter(gw)
	defer func() {
		if err := tw.Close(); err != nil {
			log.Warnf("tar writer close error: %v", err)
		}
	}()

	info, err := fs.Get(ctx, reqPath, &fs.GetArgs{NoLog: true})
	if err != nil {
		return err
	}

	return fs.WalkFS(ctx, -1, reqPath, info, func(path string, obj model.Obj) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if obj.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(reqPath, path)
		if err != nil {
			return err
		}
		relPath = strings.ReplaceAll(relPath, "\\", "/")

		header := &tar.Header{
			Name:    relPath,
			Size:    obj.GetSize(),
			Mode:    0644,
			ModTime: obj.ModTime(),
		}

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		return streamFileContent(ctx, tw, path)
	})
}

func streamFileContent(ctx context.Context, w io.Writer, filePath string) error {
	link, _, err := fs.Link(ctx, filePath, model.LinkArgs{
		Header: http.Header{},
	})
	if err != nil {
		return err
	}
	defer link.Close()

	if link.RangeReader != nil {
		reader, err := link.RangeReader.RangeRead(ctx, http_range.Range{Start: 0, Length: -1})
		if err != nil {
			return err
		}
		defer reader.Close()
		return utils.CopyWithCtx(ctx, w, reader, link.ContentLength, nil)
	}

	if link.URL != "" {
		return errors.New("remote URL streaming not supported for archive, please use proxy storage")
	}

	return errors.New("no content available for streaming")
}
