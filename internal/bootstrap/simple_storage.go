package bootstrap

import (
	"context"
	"fmt"
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
	var err error

	switch storageType {
	case "local":
		storage, addition = buildLocalStorage()
	case "s3":
		storage, addition, err = buildS3Storage()
		if err != nil {
			log.Errorf("invalid S3 storage config: %+v", err)
			return
		}
	case "webdav":
		storage, addition, err = buildWebDAVStorage()
		if err != nil {
			log.Errorf("invalid WebDAV storage config: %+v", err)
			return
		}
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

func requireEnv(key string) (string, error) {
	value := os.Getenv(key)
	if value == "" {
		return "", fmt.Errorf("required environment variable %s is not set", key)
	}
	return value, nil
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

func buildS3Storage() (model.Storage, driver.Additional, error) {
	accessKeyID, err := requireEnv("S3_ACCESS_KEY_ID")
	if err != nil {
		return model.Storage{}, nil, err
	}
	secretAccessKey, err := requireEnv("S3_SECRET_ACCESS_KEY")
	if err != nil {
		return model.Storage{}, nil, err
	}
	region, err := requireEnv("S3_REGION")
	if err != nil {
		return model.Storage{}, nil, err
	}
	bucket, err := requireEnv("S3_BUCKET")
	if err != nil {
		return model.Storage{}, nil, err
	}
	endpoint := getEnv("S3_ENDPOINT", "")

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
		AccessKeyID:       accessKeyID,
		SecretAccessKey:   secretAccessKey,
		Region:            region,
		Bucket:            bucket,
		Endpoint:          endpoint,
		SignURLExpire:     4,
		ListObjectVersion: "v1",
	}

	return storage, addition, nil
}

func buildWebDAVStorage() (model.Storage, driver.Additional, error) {
	url, err := requireEnv("WEBDAV_URL")
	if err != nil {
		return model.Storage{}, nil, err
	}
	username := getEnv("WEBDAV_USERNAME", "")
	password := getEnv("WEBDAV_PASSWORD", "")
	root := getEnv("WEBDAV_ROOT", "/")

	storage := model.Storage{
		MountPath:       "/",
		Order:           1,
		Driver:          "WebDav",
		CacheExpiration: 30,
		Status:          op.WORK,
	}

	addition := &webdav.Addition{
		Vendor:   "other",
		Address:  url,
		Username: username,
		Password: password,
		RootPath: driver.RootPath{
			RootFolderPath: root,
		},
	}

	return storage, addition, nil
}
