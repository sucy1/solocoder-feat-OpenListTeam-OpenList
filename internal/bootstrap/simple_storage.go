package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/OpenListTeam/OpenList/v4/cmd/flags"
	"github.com/OpenListTeam/OpenList/v4/drivers/local"
	"github.com/OpenListTeam/OpenList/v4/drivers/s3"
	"github.com/OpenListTeam/OpenList/v4/drivers/webdav"
	"github.com/OpenListTeam/OpenList/v4/internal/driver"
	"github.com/OpenListTeam/OpenList/v4/internal/model"
	"github.com/OpenListTeam/OpenList/v4/internal/op"
	"github.com/OpenListTeam/OpenList/v4/pkg/utils"
	log "github.com/sirupsen/logrus"
)

func InitSimpleStorage() {
	if flags.StorageType == "" {
		return
	}

	storageType := strings.ToLower(flags.StorageType)
	var storage model.Storage
	var addition driver.Additional

	switch storageType {
	case "local":
		storage, addition = buildLocalStorage()
	case "s3":
		storage, addition = buildS3Storage()
	case "webdav":
		storage, addition = buildWebDAVStorage()
	default:
		log.Warnf("unsupported storage type: %s, skip simple storage init", flags.StorageType)
		return
	}

	additionStr, err := utils.Json.MarshalToString(addition)
	if err != nil {
		log.Errorf("failed to marshal storage addition: %+v", err)
		return
	}
	storage.Addition = additionStr
	storage.Modified = time.Now()

	err = op.LoadStorage(context.Background(), storage)
	if err != nil {
		log.Errorf("failed to load simple storage: %+v", err)
	} else {
		log.Infof("success load simple storage: [%s], driver: [%s]",
			storage.MountPath, storage.Driver)
	}
}

func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

func buildLocalStorage() (model.Storage, driver.Additional) {
	root := getEnv("LOCAL_ROOT", ".")
	absRoot, err := filepath.Abs(root)
	if err == nil {
		root = absRoot
	}

	storage := model.Storage{
		MountPath:       "/",
		Order:           1,
		Driver:          "Local",
		CacheExpiration: 30,
		Status:          op.WORK,
	}

	addition := &local.Addition{
		RootPath: driver.RootPath{
			RootFolderPath: root,
		},
		Thumbnail:        true,
		ThumbConcurrency: "16",
		VideoThumbPos:    "20%",
		ShowHidden:       true,
		MkdirPerm:        "777",
		RecycleBinPath:   "delete permanently",
	}

	return storage, addition
}

func buildS3Storage() (model.Storage, driver.Additional) {
	storage := model.Storage{
		MountPath:       "/",
		Order:           1,
		Driver:          "S3",
		CacheExpiration: 30,
		Status:          op.WORK,
	}

	addition := &s3.Addition{
		RootPath: driver.RootPath{
			RootFolderPath: "/",
		},
		AccessKeyID:       getEnv("S3_ACCESS_KEY_ID", ""),
		SecretAccessKey:   getEnv("S3_SECRET_ACCESS_KEY", ""),
		Region:            getEnv("S3_REGION", ""),
		Bucket:            getEnv("S3_BUCKET", ""),
		Endpoint:          getEnv("S3_ENDPOINT", ""),
		SignURLExpire:     4,
		ListObjectVersion: "v1",
	}

	return storage, addition
}

func buildWebDAVStorage() (model.Storage, driver.Additional) {
	storage := model.Storage{
		MountPath:       "/",
		Order:           1,
		Driver:          "WebDav",
		CacheExpiration: 30,
		Status:          op.WORK,
	}

	addition := &webdav.Addition{
		Vendor:   "other",
		Address:  getEnv("WEBDAV_URL", ""),
		Username: getEnv("WEBDAV_USERNAME", ""),
		Password: getEnv("WEBDAV_PASSWORD", ""),
		RootPath: driver.RootPath{
			RootFolderPath: getEnv("WEBDAV_ROOT", "/"),
		},
	}

	return storage, addition
}
