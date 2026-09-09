package common

import (
	"context"

	"zenbot/internal/model"
)

// MessageRecordAuditor preserves message visibility at the persistence edge.
type MessageRecordAuditor interface {
	LogMessageRecord(context.Context, model.MessageRecord) (int64, error)
}

// AfkRenameController updates AFK identities when the room renames a user.
type AfkRenameController interface {
	RenameAfkUser(before, after string)
}

// BotIdentityMatcher identifies host and managed-replica users.
type BotIdentityMatcher interface {
	IsManagedBotName(string) bool
}
