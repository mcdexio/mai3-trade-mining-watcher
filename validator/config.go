package validator

import "time"

type Config struct {
	RoundInterval time.Duration `arg:"env:ROUND_INTERVAL"`
	MainDB        string        `arg:"env:MAIN_DB"`
	BackupDB      string        `arg:"env:BACKUP_DB"`
}
