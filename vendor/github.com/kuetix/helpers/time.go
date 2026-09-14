package helpers

import "time"

// NowAsString returns the current time as a string in RFC3339 format
func NowAsString() string {
	return time.Now().Format(time.RFC3339)
}

// TimeToString converts a time.Time to a string in RFC3339 format
func TimeToString(t time.Time) string {
	return t.Format(time.RFC3339)
}
