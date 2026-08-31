package command

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type ReplicaController interface {
	AddReplica(context.Context, string) error
	RemoveReplica(context.Context, string) error
	ReplicaChannels() []string
}

func ParseReplicaChannel(args []string) (string, error) {
	if len(args) > 0 {
		args = args[1:]
	}
	if len(args) == 0 {
		return "", errors.New("replica channel is required")
	}
	ch := strings.TrimSpace(args[0])
	if ch == "" {
		return "", errors.New("replica channel is required")
	}
	return ch, nil
}
func ReplicaReply(count int) string { return fmt.Sprintf(" replicas: %d", count) }
func ReplicaStatusReply(host string, channels []string) string {
	cp := append([]string(nil), channels...)
	sort.Strings(cp)
	return fmt.Sprintf(" channel: %s replicas: %s", host, strings.Join(cp, ","))
}
func ReplicaOff(ctx context.Context, c ReplicaController, args []string) error {
	ch, err := ParseReplicaChannel(args)
	if err != nil {
		return err
	}
	return c.RemoveReplica(ctx, ch)
}

type RoomMessenger interface {
	SendChatMessage(author, message string, whisper bool) (string, error)
}

func ParseMsgChannel(args []string) (string, string, error) {
	if len(args) > 0 {
		args = args[1:]
	}
	if len(args) < 2 {
		return "", "", errors.New("target room and message are required")
	}
	room := strings.TrimPrefix(strings.TrimSpace(args[0]), "?")
	if room == "" {
		return "", "", errors.New("target room is required")
	}
	return room, strings.Join(args[1:], " "), nil
}

type ProxyRetryPolicy struct {
	ProbeTimeout time.Duration
	MaxAttempts  int
}

// WhiskeyProxyOrder bounds failover while preserving configured order.
func WhiskeyProxyOrder(ctx context.Context, proxies []string, probe func(context.Context, string) error, max int) (string, []string, error) {
	if max <= 0 || max > len(proxies) {
		max = len(proxies)
	}
	for i, p := range proxies[:max] {
		if err := probe(ctx, p); err == nil {
			return p, append([]string(nil), proxies[i+1:]...), nil
		}
	}
	return "", nil, errors.New("no healthy proxy")
}
