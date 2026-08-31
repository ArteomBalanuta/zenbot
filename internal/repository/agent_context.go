package repository

import "context"

// PublicRoomMessage is untrusted public-room evidence for agent prompt context.
type PublicRoomMessage struct {
	Name, Trip, Hash, Message, Channel string
	CreatedOnMillis                    int64
}

// AgentConversationRepository reads bounded public room context.
type AgentConversationRepository interface {
	RecentPublicRoomMessages(context.Context, string, int) ([]PublicRoomMessage, error)
}

// AgentUserMessageHistoryRepository reads bounded public history for a named
// user in the already trusted current room.
type AgentUserMessageHistoryRepository interface {
	RecentPublicRoomMessagesForNick(context.Context, string, string, int) ([]PublicRoomMessage, error)
}
