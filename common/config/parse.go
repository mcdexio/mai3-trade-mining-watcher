package config

import "time"

// ParseTimeConfig parses config to time with format.
func ParseTimeConfig(t string) (time.Time, error) {
	return time.Parse(time.RFC3339, t)
}
