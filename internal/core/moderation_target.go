package core

import (
	"fmt"
	"strings"

	"zenbot/internal/model"
)

// ModerationTarget is an immutable snapshot of listener-resolved user identity.
// Its raw hash is retained for the source-compatible unmute protocol.
type ModerationTarget struct {
	Name string
	Trip string
	Hash string
}

// NewModerationTarget copies authoritative listener-resolved identity. It does
// not normalize any identity field, because downstream source semantics require
// exact values.
func NewModerationTarget(user model.User) (ModerationTarget, error) {
	if strings.TrimSpace(user.Name) == "" {
		return ModerationTarget{}, fmt.Errorf("moderation target name is blank")
	}
	return ModerationTarget{Name: user.Name, Trip: user.Trip, Hash: user.Hash}, nil
}
