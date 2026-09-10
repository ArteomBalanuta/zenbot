package util

import (
	"errors"
	"strings"
)

var ErrBlankNickTarget = errors.New("Nick target cannot be blank")

// NormalizeNickTarget mirrors Saturn: trim, remove exactly one ASCII @, trim,
// and reject null/blank/bare-marker targets.
func NormalizeNickTarget(raw *string) (string, error) {
	if raw == nil {
		return "", ErrBlankNickTarget
	}
	value := strings.TrimSpace(*raw)
	if strings.HasPrefix(value, "@") {
		value = strings.TrimSpace(value[1:])
	}
	if value == "" {
		return "", ErrBlankNickTarget
	}
	return value, nil
}

func CanonicalNick(raw *string) (string, error) {
	value, err := NormalizeNickTarget(raw)
	if err != nil {
		return "", err
	}
	return strings.ToLower(value), nil
}

func SameNick(left, right *string) bool {
	if left == nil || right == nil {
		return false
	}
	leftCanonical, leftErr := CanonicalNick(left)
	rightCanonical, rightErr := CanonicalNick(right)
	return leftErr == nil && rightErr == nil && leftCanonical == rightCanonical
}
