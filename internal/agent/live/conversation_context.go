package live

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"zenbot/internal/agent/runtime"
	"zenbot/internal/repository"
)

// ConversationContextProvider loads serialized untrusted room evidence.
type ConversationContextProvider interface {
	Load(context.Context, runtime.Invocation) (string, error)
}

// NoConversationContext preserves existing behavior where no provider is wired.
type NoConversationContext struct{}

func (NoConversationContext) Load(context.Context, runtime.Invocation) (string, error) {
	return "", nil
}

// RepositoryConversationContextProvider loads bounded public-room messages.
type RepositoryConversationContextProvider struct {
	Repository   repository.AgentConversationRepository
	MessageLimit int
}

func NewRepositoryConversationContextProvider(repository repository.AgentConversationRepository, messageLimit int) (RepositoryConversationContextProvider, error) {
	if repository == nil {
		return RepositoryConversationContextProvider{}, fmt.Errorf("agent conversation context repository is required")
	}
	if messageLimit <= 0 {
		return RepositoryConversationContextProvider{}, fmt.Errorf("agent conversation context message limit must be positive")
	}
	return RepositoryConversationContextProvider{Repository: repository, MessageLimit: messageLimit}, nil
}

type publicRoomMessageJSON struct {
	Name      string `json:"name"`
	Trip      string `json:"trip"`
	Hash      string `json:"hash"`
	Message   string `json:"message"`
	CreatedOn int64  `json:"createdOn"`
	Channel   string `json:"channel"`
}

type publicRoomMessagesJSON struct {
	Rows []publicRoomMessageJSON `json:"rows"`
}

func (p RepositoryConversationContextProvider) Load(ctx context.Context, inv runtime.Invocation) (string, error) {
	if inv.Context().Whisper() {
		return "", nil
	}
	rows, err := p.Repository.RecentPublicRoomMessages(ctx, inv.Context().Room(), p.MessageLimit)
	if err != nil {
		return "", err
	}
	out := make([]publicRoomMessageJSON, 0, len(rows))
	for _, row := range rows {
		out = append(out, publicRoomMessageJSON{Name: row.Name, Trip: row.Trip, Hash: row.Hash, Message: row.Message, CreatedOn: row.CreatedOnMillis, Channel: row.Channel})
	}
	if nick, current := inv.Context().Nick(), inv.CurrentMessageText(); strings.TrimSpace(nick) != "" && current != "" {
		for i := len(out) - 1; i >= 0; i-- {
			if out[i].Name == nick && out[i].Message == current {
				out = append(out[:i], out[i+1:]...)
				break
			}
		}
	}
	encoded, err := json.Marshal(publicRoomMessagesJSON{Rows: out})
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func loadRecentContext(ctx context.Context, provider ConversationContextProvider, inv runtime.Invocation) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if provider == nil || inv.Context().Whisper() {
		return "", nil
	}
	recent, err := provider.Load(ctx, inv)
	if err == nil {
		return recent, nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "", err
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", ctxErr
	}
	log.Printf("agent conversation context load failed requestID=%s: %v", inv.RequestID(), err)
	return "", nil
}
