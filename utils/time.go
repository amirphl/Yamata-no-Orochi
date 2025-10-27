// Package utils provides utility functions for the application.
package utils

import (
	"time"
)

// UTCNow returns the current time in UTC
func UTCNow() time.Time {
	return time.Now().UTC()
}

// UTCNowPtr returns a pointer to the current time in UTC
func UTCNowPtr() *time.Time {
	now := UTCNow()
	return &now
}

// UTCNowAdd returns the current UTC time plus the given duration
func UTCNowAdd(d time.Duration) time.Time {
	return UTCNow().Add(d)
}

// UTCNowAddPtr returns a pointer to the current UTC time plus the given duration
func UTCNowAddPtr(d time.Duration) *time.Time {
	now := UTCNowAdd(d)
	return &now
}

// UTCNowUnix returns the current UTC time as Unix timestamp
func UTCNowUnix() int64 {
	return UTCNow().Unix()
}

// UTCNowUnixNano returns the current UTC time as Unix nanosecond timestamp
func UTCNowUnixNano() int64 {
	return UTCNow().UnixNano()
}

// UTCNowFormat returns the current UTC time formatted according to the given layout
func UTCNowFormat(layout string) string {
	return UTCNow().Format(layout)
}

// UTCNowRFC3339 returns the current UTC time in RFC3339 format
func UTCNowRFC3339() string {
	return UTCNow().Format(time.RFC3339)
}

// UTCNowRFC3339Nano returns the current UTC time in RFC3339Nano format
func UTCNowRFC3339Nano() string {
	return UTCNow().Format(time.RFC3339Nano)
}

// IsExpired checks if the given time is in the past (expired)
func IsExpired(t time.Time) bool {
	return UTCNow().After(t)
}

// IsExpiredPtr checks if the given time pointer is in the past (expired)
func IsExpiredPtr(t *time.Time) bool {
	if t == nil {
		return false
	}
	return IsExpired(*t)
}

// IsValid checks if the given time is in the future (valid)
func IsValid(t time.Time) bool {
	return UTCNow().Before(t)
}

// IsValidPtr checks if the given time pointer is in the future (valid)
func IsValidPtr(t *time.Time) bool {
	if t == nil {
		return false
	}
	return IsValid(*t)
}

// TimeToUTC converts a time to UTC if it's not already
func TimeToUTC(t time.Time) time.Time {
	return t.UTC()
}

// TimeToUTCPtr converts a time pointer to UTC if it's not already
func TimeToUTCPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	utc := TimeToUTC(*t)
	return &utc
}

func TehranNow() (time.Time, error) {
	loc, err := time.LoadLocation("Asia/Tehran")
	if err != nil {
		return time.Time{}, err
	}
	return time.Now().In(loc), nil
}
