package config

import (
	"fmt"
	"strconv"
	"strings"
)

// ValueReader provides explicit/runtime > environment > file/database lookup.
type ValueReader struct {
	Prefix      string
	Runtime     map[string]string
	Environment map[string]string
	File        map[string]string
}

func (r ValueReader) lookup(key string) (string, bool) {
	key = strings.TrimSpace(key)
	for _, m := range []map[string]string{r.Runtime, r.Environment, r.File} {
		if v, ok := m[key]; ok {
			return v, true
		}
		if v, ok := m[strings.ToUpper(key)]; ok {
			return v, true
		}
	}
	return "", false
}
func (r ValueReader) String(key, fallback string) string {
	if v, ok := r.lookup(key); ok {
		return v
	}
	return fallback
}
func (r ValueReader) Bool(key string, fallback bool) (bool, error) {
	v, ok := r.lookup(key)
	if !ok || strings.TrimSpace(v) == "" {
		return fallback, nil
	}
	x, err := strconv.ParseBool(strings.TrimSpace(v))
	if err != nil {
		return false, fmt.Errorf("%s must be true or false: %w", key, err)
	}
	return x, nil
}
func (r ValueReader) Int(key string, fallback int) (int, error) {
	v, ok := r.lookup(key)
	if !ok || strings.TrimSpace(v) == "" {
		return fallback, nil
	}
	x, err := strconv.ParseInt(strings.TrimSpace(v), 10, 0)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	return int(x), nil
}
