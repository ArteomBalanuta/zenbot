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

type nukeCommand struct{ commandBase }

func (c *nukeCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	arguments := args(c.message)
	if len(arguments) == 0 {
		if err := replyContext(ctx, &c.commandBase, "\\n Example: "+c.engine.GetPrefix()+"nuke hotlinks"); err != nil {
			return model.FAILED, err
		}
		return model.FAILED, nil
	}
	target := strings.ReplaceAll(arguments[0], "@", "")
	if target == "" {
		if err := replyContext(ctx, &c.commandBase, "\\n Example: "+c.engine.GetPrefix()+"nuke hotlinks"); err != nil {
			return model.FAILED, err
		}
		return model.FAILED, nil
	}
	submitter, ok := c.engine.(common.CredentialedRoomSnapshotSubmitter)
	if !ok {
		return model.FAILED, fmt.Errorf("credentialed room snapshot submitter is not configured")
	}
	workflowID, err := nukeWorkflowID()
	if err != nil {
		return model.FAILED, err
	}
	if err := submitter.SubmitCredentialedRoomSnapshot(snapshot.RoomSnapshotRequest{
		Context:       ctx,
		WorkflowID:    workflowID,
		Author:        c.message.Name,
		Whisper:       c.message.IsWhisper || c.message.Whisper || c.message.Type == "whisper",
		SourceChannel: c.message.Channel,
		TargetChannel: target,
		ReplyMessage:  "Unable to complete room operation.",
		Operation:     snapshot.NewNukeRoomOperation(),
	}); err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}

func nukeWorkflowID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("generate nuke workflow ID: %w", err)
	}
	return fmt.Sprintf("nuke-%x", bytes), nil
}
