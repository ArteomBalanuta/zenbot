package command

import (
	"context"
	"errors"
	"zenbot/internal/common"
	"zenbot/internal/model"
)

// VibeSubmitter uses the same process-owned runtime as direct agent requests.
type VibeSubmitter interface {
	SubmitVibe(context.Context, *model.ChatMessage) error
}

type vibeCommand struct {
	commandBase
	submitter VibeSubmitter
}

func (c *vibeCommand) NewInstance(e common.Engine, m *model.ChatMessage) common.SaturnCommand {
	base := c.commandBase
	base.engine, base.message, base.aliases = e, m, c.Aliases()
	return &vibeCommand{commandBase: base, submitter: c.submitter}
}

func (c *vibeCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	if commandBody(c.message) != "" {
		return model.FAILED, replyContext(ctx, &c.commandBase, "Use "+c.engine.GetPrefix()+"vibe without arguments.")
	}
	if c.message.IsWhisper || c.message.Whisper || c.message.Type == "whisper" {
		return model.FAILED, replyContext(ctx, &c.commandBase, "Use vibe in the room so I can read its recent public conversation.")
	}
	if c.submitter == nil {
		return model.FAILED, replyContext(ctx, &c.commandBase, "Vibe checks are unavailable while the room's LLM agent is disabled.")
	}
	if err := c.submitter.SubmitVibe(ctx, c.message); err != nil {
		if ctx.Err() != nil {
			return model.FAILED, err
		}
		return model.FAILED, errors.Join(err, replyContext(ctx, &c.commandBase, "Couldn't start a vibe check right now; the agent may be busy or unavailable. Try again shortly."))
	}
	return model.SUCCESSFUL, nil
}
