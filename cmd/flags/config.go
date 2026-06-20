package flags

import "time"

var (
	DataDir        string
	ConfigPath     string
	Debug          bool
	NoPrefix       bool
	Dev            bool
	ForceBinDir    bool
	LogStd         bool
	StorageType    string
	Upload         bool
	UploadDir      string
	MaxUploadSize  int64
	PasswdFile     string
	ArchiveTimeout time.Duration
)
