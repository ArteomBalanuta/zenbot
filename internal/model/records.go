package model

import "strings"

type Afk struct {
	Trip      string `json:"trip"`
	Name      string `json:"nick"`
	Reason    string `json:"reason"`
	CreatedOn int64  `json:"createdOn"`
}
type BanRecord struct {
	ID                       int64
	Trip, Name, Hash, Reason string
	CreatedOn                int64
}
type Mail struct {
	ID                               int64
	Owner, Receiver, Message, Status string
	CreatedOn                        int64
	IsWhisper                        bool
}
type MessageAuditEvent struct {
	Trip, Name, Hash, Message, Channel, Visibility string
	CreatedOnMillis                                int64
}
type Weather struct {
	Location, Description string
	Temperature           float64
	Time                  WeatherTime
}
type WeatherTime struct{ Sunrise, Sunset string }
type TimeResponse struct{ Location, Time, Zone string }
type MessageRecord struct {
	Trip, Name, Hash, Message string
	CreatedOnMillis           int64
	Channel, Visibility       string
}
type PresenceRecord struct {
	Trip, Name, Hash, EventType string
	CreatedOnMillis             int64
	Channel                     string
}
type CommandAuditRecord struct {
	Trip, CommandName, Arguments, Status string
	CreatedOnMillis                      int64
	Channel                              string
}

func IdentityKey(trip, hash, nick string) string {
	if strings.TrimSpace(trip) != "" {
		return "trip:" + strings.TrimSpace(trip)
	}
	if strings.TrimSpace(hash) != "" {
		return "hash:" + strings.TrimSpace(hash)
	}
	return "nick:" + strings.ToLower(strings.TrimSpace(nick))
}
