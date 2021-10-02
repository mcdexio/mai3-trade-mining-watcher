package validator

import "time"

type Config struct {
	RoundInterval time.Duration `arg:"env:ROUND_INTERVAL" default:"1m"`
	DatabaseURLs  []string      `arg:"env:DATABASE_URLS"`
}
