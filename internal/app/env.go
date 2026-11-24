package app

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// EnvString returns the value of key, or def when key is unset or empty.
func EnvString(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// EnvInt returns the value of key as an int, or def when key is unset or empty.
func EnvInt(key string, def int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %q is not an integer", key, v)
	}
	return n, nil
}

// EnvFloat returns the value of key as a float64, or def when key is unset or empty.
func EnvFloat(key string, def float64) (float64, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: %q is not a number", key, v)
	}
	return f, nil
}

// EnvDuration returns the value of key as a time.Duration (for example "2s" or "500ms"),
// or def when key is unset or empty.
func EnvDuration(key string, def time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %q is not a duration such as 2s or 500ms", key, v)
	}
	return d, nil
}
