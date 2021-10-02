package validator

import (
	"fmt"
	"time"
)

func formatTime(timestamp int64) string {
	return fmt.Sprintf("%v (%v)", timestamp, time.Unix(timestamp, 0).UTC().Format("2006-01-02 15:04:05"))
}
