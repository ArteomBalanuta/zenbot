package live

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"zenbot/internal/common"
	"zenbot/internal/util"

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
	Repository      repository.AgentConversationRepository
	MessageLimit    int
	BotNames        func() []string
	CommandPrefix   func() string
	CommandPrefixes func() []string
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
	limit := p.MessageLimit
	if inv.Mode() == runtime.VIBE {
		limit = min(limit, 60)
	}
	rows, err := p.Repository.RecentPublicRoomMessages(ctx, inv.Context().Room(), limit)
	if err != nil {
		return "", err
	}
	if inv.Mode() == runtime.VIBE {
		return p.vibeContext(inv, rows)
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
		if inv.Mode() == runtime.VIBE {
			return "", fmt.Errorf("vibe room history is unavailable")
		}
		return "", nil
	}
	recent, err := provider.Load(ctx, inv)
	if err == nil {
		if inv.Mode() == runtime.VIBE && strings.TrimSpace(recent) == "" {
			return "", fmt.Errorf("vibe room history is unavailable")
		}
		return recent, nil
	}
	if inv.Mode() == runtime.VIBE {
		return "", fmt.Errorf("load vibe room history: %w", err)
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

// vibeContext projects existing history, not a second message store. Keep the
// newest bounded sample and leave interpretation and sufficiency to the model.
func (p RepositoryConversationContextProvider) vibeContext(inv runtime.Invocation, rows []repository.PublicRoomMessage) (string, error) {
	type row struct {
		Name       string `json:"name"`
		Message    string `json:"message"`
		CreatedOn  int64  `json:"createdOn"`
		AgeMinutes int64  `json:"ageMinutes"`
		Truncated  bool   `json:"truncated,omitempty"`
	}
	var prefixes []string
	bots := map[string]bool{}
	if p.BotNames != nil {
		for _, bot := range p.BotNames() {
			key, err := util.CanonicalNick(&bot)
			if err == nil {
				bots[key] = true
			}
		}
	}
	if p.CommandPrefixes != nil {
		prefixes = p.CommandPrefixes()
	} else if p.CommandPrefix != nil {
		prefixes = []string{p.CommandPrefix()}
	}
	caller := inv.Context().Nick()
	callerKey, _ := util.CanonicalNick(&caller)
	kept := make([]row, 0)
	remaining := 16000
	for i := len(rows) - 1; i >= 0 && len(kept) < 60; i-- {
		r := rows[i]
		name, err := util.NormalizeNickTarget(&r.Name)
		key, _ := util.CanonicalNick(&r.Name)
		if err != nil || !strings.EqualFold(r.Channel, inv.Context().Room()) || strings.TrimSpace(r.Message) == "" || bots[key] {
			continue
		}
		if (key == callerKey && r.Message == inv.CurrentMessageText()) || common.MatchCommandPrefix(strings.TrimSpace(r.Message), prefixes) != "" {
			continue
		}
		chars := []rune(r.Message)
		truncated := len(chars) > 1000
		if truncated {
			chars = chars[:1000]
		}
		if len(chars) > remaining {
			break
		}
		remaining -= len(chars)
		kept = append(kept, row{Name: name, Message: string(chars), CreatedOn: r.CreatedOnMillis, AgeMinutes: max(0, (inv.CreatedOn().UnixMilli()-r.CreatedOnMillis)/60000), Truncated: truncated})
	}
	for i, j := 0, len(kept)-1; i < j; i, j = i+1, j-1 {
		kept[i], kept[j] = kept[j], kept[i]
	}
	encoded, err := json.Marshal(struct {
		Rows []row `json:"rows"`
	}{Rows: kept})
	return string(encoded), err
}
