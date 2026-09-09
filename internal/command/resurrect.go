package command

import (
	"context"
	"crypto/rand"
	"fmt"

	"zenbot/internal/common"
	"zenbot/internal/listener/snapshot"
	"zenbot/internal/model"
	"zenbot/internal/util"
)

type resurrectCommand struct{ commandBase }

func (c *resurrectCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	arguments := args(c.message)
	if len(arguments) != 3 {
		reply(&c.commandBase, " "+c.engine.GetPrefix()+"move <nick> <from> <to>")
		return model.FAILED, nil
	}
	nick, err := util.NormalizeNickTarget(&arguments[0])
	if err != nil {
		reply(&c.commandBase, " "+c.engine.GetPrefix()+"move <nick> <from> <to>")
		return model.FAILED, nil
	}
	mover, ok := c.engine.(common.LiveRoomMover)
	if !ok {
		return model.FAILED, fmt.Errorf("live room mover is not configured")
	}
	handled, err := mover.MoveFromServingRoom(ctx, arguments[1], common.NickTarget(nick), common.Channel(arguments[2]))
	if err != nil || handled {
		if err != nil {
			return model.FAILED, err
		}
		return model.SUCCESSFUL, nil
	}
	submitter, ok := c.engine.(common.RoomSnapshotSubmitter)
	if !ok {
		return model.FAILED, fmt.Errorf("room snapshot submitter is not configured")
	}
	workflowID, err := resurrectWorkflowID()
	if err != nil {
		return model.FAILED, err
	}
	if err := submitter.SubmitRoomSnapshot(snapshot.RoomSnapshotRequest{
		WorkflowID:         workflowID,
		Author:             c.message.Name,
		Whisper:            c.message.IsWhisper || c.message.Whisper || c.message.Type == "whisper",
		SourceChannel:      arguments[1],
		TargetChannel:      arguments[1],
		DestinationChannel: arguments[2],
		ReplyMessage:       "Unable to complete room operation.",
		Operation:          snapshot.NewKickOrResurrectOperation(nick),
	}); err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}

func resurrectWorkflowID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("generate resurrect workflow ID: %w", err)
	}
	return fmt.Sprintf("resurrect-%x", bytes), nil
}
