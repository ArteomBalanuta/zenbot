package command

import (
	"context"

	"zenbot/internal/common"
	"zenbot/internal/model"
)

const moderationGuidePayload = "hack.chat moderation guide \n In case spammer or a ~~valid~~ nasty user joined: \n https://youtu.be/E_Yl9ul3Ulw"

type howToCommand struct {
	engine  common.Engine
	message *model.ChatMessage
}

func (c *howToCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	_, err := c.engine.SendAddressedMessage(c.message.Name, moderationGuidePayload, c.message.IsWhisper)
	if err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}

func (c *howToCommand) Role() model.Role { return model.USER }
func (c *howToCommand) Aliases() []string {
	return []string{"crashcourse", "howto", "moderationcrashcourse", "hcguide"}
}
func (c *howToCommand) NewInstance(e common.Engine, m *model.ChatMessage) common.SaturnCommand {
	return &howToCommand{engine: e, message: m}
}

func howToDefinition() common.CommandDefinition {
	aliases := []string{"crashcourse", "howto", "moderationcrashcourse", "hcguide"}
	return common.CommandDefinition{
		Canonical: "crashcourse", Aliases: aliases, Role: model.USER,
		New: func(e common.Engine, m *model.ChatMessage) common.SaturnCommand {
			return &howToCommand{engine: e, message: m}
		},
	}
}
