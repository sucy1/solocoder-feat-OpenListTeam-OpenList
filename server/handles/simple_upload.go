package handles

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/OpenListTeam/OpenList/v4/cmd/flags"
	"github.com/OpenListTeam/OpenList/v4/server/common"
	"github.com/gin-gonic/gin"
)

func SimpleUpload(c *gin.Context) {
	if !flags.Upload {
		common.ErrorStrResp(c, "upload is not enabled", 403)
		return
	}

	uploadDir := flags.UploadDir
	if uploadDir == "" {
		uploadDir = "uploads"
	}

	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		common.ErrorResp(c, err, 500)
		return
	}

	maxSize := flags.MaxUploadSize
	if maxSize <= 0 {
		maxSize = 100 * 1024 * 1024
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSize)

	file, err := c.FormFile("file")
	if err != nil {
		common.ErrorResp(c, err, 400)
		return
	}

	if file.Size > maxSize {
		common.ErrorStrResp(c, fmt.Sprintf("file size exceeds maximum allowed size of %d bytes", maxSize), 413)
		return
	}

	filename := file.Filename
	savePath := filepath.Join(uploadDir, filename)
	savePath = generateUniquePath(savePath)

	if err := c.SaveUploadedFile(file, savePath); err != nil {
		common.ErrorResp(c, err, 500)
		return
	}

	common.SuccessResp(c, gin.H{
		"name":     filepath.Base(savePath),
		"size":     file.Size,
		"path":     savePath,
		"mimetype": file.Header.Get("Content-Type"),
	})
}

func generateUniquePath(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}

	dir := filepath.Dir(path)
	ext := filepath.Ext(path)
	name := strings.TrimSuffix(filepath.Base(path), ext)

	for i := 1; ; i++ {
		newName := fmt.Sprintf("%s_%d%s", name, i, ext)
		newPath := filepath.Join(dir, newName)
		if _, err := os.Stat(newPath); os.IsNotExist(err) {
			return newPath
		}
	}
}
