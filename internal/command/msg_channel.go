package command

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"

	"zenbot/internal/common"
	"zenbot/internal/listener/snapshot"
	"zenbot/internal/model"
)

// msgChannelCommand is the concrete local msgchannel/msgroom command.
type msgChannelCommand struct{ commandBase }

func (c *msgChannelCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	target, message := splitCommandToken(commandBody(c.message))
	message = strings.TrimSpace(message)
	if target == "" || message == "" {
		reply(&c.commandBase, " Example: "+c.engine.GetPrefix()+"msgroom your-room your message")
		return model.FAILED, nil
	}
	room := strings.TrimSpace(strings.ReplaceAll(target, "?", ""))
	if room == "" {
		reply(&c.commandBase, "Room name cannot be blank.")
		return model.FAILED, nil
	}
	body := msgChannelBody(c.engine.GetChannel(), message)
	if room != c.engine.GetChannel() {
		submitter, ok := c.engine.(common.CredentialedRoomSnapshotSubmitter)
		if !ok {
			return model.FAILED, fmt.Errorf("room snapshot submitter is not configured")
		}
		workflowID, err := msgChannelWorkflowID()
		if err != nil {
			return model.FAILED, err
		}
		if err := submitter.SubmitCredentialedRoomSnapshot(snapshot.RoomSnapshotRequest{
			Context:       ctx,
			WorkflowID:    workflowID,
			Author:        c.message.Name,
			Whisper:       c.message.IsWhisper || c.message.Whisper || c.message.Type == "whisper",
			SourceChannel: c.engine.GetChannel(),
			TargetChannel: room,
			ReplyMessage:  "Unable to complete room operation.",
			RemoteMessage: body,
			Operation:     snapshot.NewRemoteMessageOperation(body),
		}); err != nil {
			return model.FAILED, err
		}
		return model.SUCCESSFUL, nil
	}
	if _, err := c.engine.SendChatMessage(c.message.Name+" ", body, c.message.IsWhisper); err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}

func msgChannelBody(channel, message string) string {
	body := "anonymous mail from: ?" + channel + " message: " + message
	if strings.Contains(message, "![](") {
		return message + "\\n anonymous mail from: ?" + channel
	}
	return body
}

var msgChannelWorkflowID = func() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("generate msgchannel workflow ID: %w", err)
	}
	return fmt.Sprintf("%x", bytes), nil
}
