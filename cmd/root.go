package cmd

import (
	"fmt"
	"os"

	"github.com/OpenListTeam/OpenList/v4/cmd/flags"
	_ "github.com/OpenListTeam/OpenList/v4/drivers"
	_ "github.com/OpenListTeam/OpenList/v4/internal/archive"
	_ "github.com/OpenListTeam/OpenList/v4/internal/offline_download"
	"github.com/spf13/cobra"
)

var RootCmd = &cobra.Command{
	Use:   "openlist",
	Short: "A file list program that supports multiple storage.",
	Long: `A file list program that supports multiple storage,
built with love by OpenListTeam.
Complete documentation is available at https://doc.oplist.org/`,
}

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	RootCmd.PersistentFlags().StringVar(&flags.DataDir, "data", "data", "data directory (relative paths are resolved against the current working directory)")
	RootCmd.PersistentFlags().StringVar(&flags.ConfigPath, "config", "", "path to config.json (relative to current working directory; defaults to [data directory]/config.json, where [data directory] is set by --data)")
	RootCmd.PersistentFlags().BoolVar(&flags.Debug, "debug", false, "start with debug mode")
	RootCmd.PersistentFlags().BoolVar(&flags.NoPrefix, "no-prefix", false, "disable env prefix")
	RootCmd.PersistentFlags().BoolVar(&flags.Dev, "dev", false, "start with dev mode")
	RootCmd.PersistentFlags().BoolVar(&flags.ForceBinDir, "force-bin-dir", false, "Force to use the directory where the binary file is located as data directory")
	RootCmd.PersistentFlags().BoolVar(&flags.LogStd, "log-std", false, "Force to log to std")
	RootCmd.PersistentFlags().StringVar(&flags.StorageType, "storage-type", "local", "storage type: local, s3, webdav")
	RootCmd.PersistentFlags().BoolVar(&flags.Upload, "upload", false, "enable file upload")
	RootCmd.PersistentFlags().StringVar(&flags.UploadDir, "upload-dir", "uploads", "upload directory (default: uploads)")
	RootCmd.PersistentFlags().Int64Var(&flags.MaxUploadSize, "max-upload-size", 100*1024*1024, "max upload size in bytes (default: 100MB)")
	RootCmd.PersistentFlags().StringVar(&flags.PasswdFile, "passwd-file", "", "path to htpasswd file for directory protection")
}
