package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// UserIdentity is Saturn's normalized, source-qualified identity value.
type UserIdentity struct{ value string }
type AgentUserIdentity = UserIdentity

func NewUserIdentity(value string) (UserIdentity, error) {
	if strings.TrimSpace(value) == "" {
		return UserIdentity{}, errors.New("identity value must not be blank")
	}
	return UserIdentity{value: value}, nil
}
func (u UserIdentity) Value() string { return u.value }
func (u UserIdentity) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Value string `json:"value"`
	}{u.value})
}
func (u *UserIdentity) UnmarshalJSON(data []byte) error {
	var w struct {
		Value *string `json:"value"`
	}
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	if w.Value == nil {
		return errors.New("identity value is required")
	}
	v, err := NewUserIdentity(*w.Value)
	if err != nil {
		return err
	}
	*u = v
	return nil
}
func FromContext(c *Context) (UserIdentity, error) {
	if c == nil {
		return UserIdentity{}, errors.New("context must not be nil")
	}
	return FromValues(valueOrEmpty(c.trip), valueOrEmpty(c.hash), c.nick)
}
func FromValues(trip, hash, nick string) (UserIdentity, error) {
	source, value := "nick", nick
	if strings.TrimSpace(trip) != "" {
		source, value = "trip", trip
	} else if strings.TrimSpace(hash) != "" {
		source, value = "hash", hash
	}
	value = strings.ToLower(javaStrip(value))
	if value == "" {
		return UserIdentity{}, errors.New("nick must not be blank")
	}
	return NewUserIdentity(fmt.Sprintf("%s:%s", source, value))
}

func javaStrip(s string) string {
	return strings.TrimFunc(s, func(r rune) bool {
		if r >= 0x1c && r <= 0x1f {
			return true
		}
		if !unicode.IsSpace(r) {
			return false
		}
		switch r {
		case '\u00a0', '\u2007', '\u202f':
			return false
		default:
			return true
		}
	})
}
