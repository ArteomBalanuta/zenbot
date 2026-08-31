package command

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"time"
	"zenbot/internal/model"
)

const saturnVersion = "1.0.29"

const saturnApe = `
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣀⢠⣄⣤⣀⡀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢀⣼⣟⠻⣉⠈⢧⣂⣝⣳⣤⡀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢠⡾⠁⣹⡯⠉⢀⡸⠿⠋⠉⠛⠻⢦⡀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣰⠟⠀⢸⣟⣳⠤⢤⡄⠀⠀⢀⣀⣀⡀⠙⢷⡀⠀⠀⠀⠀⠀⠀⠀⠀⠀
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢀⣾⠋⠀⢰⠾⣻⣿⠛⢒⣤⡶⣿⣻⣛⣟⣿⣙⣺⣷⠀⠀⠀⠀⠀⠀⠀⠀⠀
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣀⡴⣺⣷⠇⠀⠈⢾⣏⠉⣴⡋⠉⠶⠿⣿⣿⣿⣿⢿⡿⢿⣄⡀⠀⠀⠀⠀⠀⠀⠀
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢀⡤⣞⡵⠋⠩⢭⣁⣀⡀⠀⣿⠀⠹⢧⣾⠤⠬⠁⣽⣛⣛⢹⣷⣬⡇⠉⠳⣄⠀⠀⠀⠀⠀
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢀⡴⡫⠞⠉⠀⠀⠀⠀⠀⠉⠙⠳⣾⣿⡀⠀⠀⠀⠀⠀⠛⠛⠋⠙⠛⣿⠀⠀⠀⢹⢦⠀⠀⠀⠀
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣴⠏⠉⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠺⣿⣟⣶⠀⢸⢁⣀⡤⠶⠖⠚⠒⣾⠀⠀⠀⠈⢻⣧⠀⠀⠀
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣠⡾⠃⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠙⣿⣿⣷⣌⠳⣄⠀⢀⣀⣠⠟⠁⠀⠀⠀⠀⢸⣿⠀⠀⠀
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣀⡤⠾⠏⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣏⢻⠈⠛⢷⣦⣳⣄⣴⠋⠀⣠⡄⠀⠀⠀⠈⣿⡆⠀⠀
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢀⣀⣠⠤⣤⡶⢶⡿⠁⠀⠀⠀⠀⠀⠀⠀⠀⢠⡄⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣿⢾⠀⠀⠀⠈⠀⠀⠀⠀⢰⡏⠀⠀⠀⠀⠀⣿⡇⠀⠀
⠀⠀⠀⠀⠀⠀⠀⠀⢀⡴⠞⠋⠉⢀⣽⣯⠁⠛⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣼⠇⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⡿⢸⠀⠀⠀⠰⡇⠀⠀⠀⢸⠇⠀⠀⠀⠀⠀⡏⡇⠀⠀
⠀⠀⠀⠀⠀⠀⢀⣴⠋⠀⠀⠀⠀⢠⣿⠀⠀⠀⠀⠀⠀⠀⠀⠰⠿⠀⢰⣦⣿⣾⡀⠀⠀⠀⠀⠀⠀⠀⠀⢠⡇⢸⠀⠀⠀⠀⠁⠀⠀⠀⡾⠀⠀⠀⠀⠀⢀⡗⣇⠀⠀
⠀⠀⠀⠀⠀⠀⣼⠁⠀⠀⠀⠀⠀⠘⠏⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠛⠹⢿⣿⠀⡇⠀⠀⠀⠀⠀⠀⠀⠀⢸⠀⣸⠀⠀⠀⠀⠀⠀⢀⣼⠇⠀⠀⠀⠀⠀⢺⡇⣿⠀⠀
⠀⠀⠀⠀⠀⢠⡇⣤⠀⠀⠀⠀⢀⡖⢒⣦⣄⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢿⡆⢿⠀⠀⠀⠀⠀⠀⠀⠀⣾⠀⡟⢀⡴⠃⠀⣠⢾⣯⣷⡆⠀⠀⠀⠀⠀⠈⠀⠹⡆⠀
⠀⠀⠀⠀⠀⢸⠽⠃⠀⠀⠀⠀⠈⢷⡄⠉⠓⠲⠄⠀⠀⠀⠀⠀⠀⠀⠀⠀⢸⡇⢸⡆⠀⠀⠀⠀⠀⠀⠀⢻⣰⡟⠋⠀⢀⡼⠁⣼⣅⠈⢿⣷⡀⠀⣸⡇⠀⠀⣷⢷⠀
⠀⠀⠀⠀⠀⣾⠘⠂⠀⠀⠀⠀⠀⠀⠻⡄⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢸⡇⠀⢿⣆⠀⠀⣠⣶⠄⠀⠈⣿⣇⣠⠴⠋⠀⢀⣿⣧⠀⠀⠘⠻⠞⠛⠀⠀⠀⢿⣼⡄
⠀⠀⠀⠀⠀⢸⢰⠀⠀⠀⠀⠀⠀⠀⠀⠹⣦⡀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢠⣯⡆⠀⠀⠹⣷⢾⠟⠁⠀⠀⢶⣺⣿⠉⠀⠀⠀⢹⣿⢿⠀⠀⠀⠀⠀⠀⠀⠀⠀⣿⣿⠀
⠀⠀⠀⠀⠀⢸⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢿⣧⣄⠠⣤⣤⠀⠀⠀⠀⠀⣾⣿⣷⠀⠀⠀⠀⠀⠀⠀⠀⠀⠈⡇⣿⠀⠀⠀⠀⠘⢿⡞⡆⠀⠀⠀⠀⠀⠀⠀⢀⣿⣿⡆
⠀⠀⠀⠀⠀⢸⠃⢀⠀⠀⠀⠀⠀⠀⠀⠀⢸⣿⢳⣄⣬⣽⣿⡃⠴⣞⢦⣸⠀⠻⢤⣤⣤⣤⠀⠀⠀⠀⠀⠀⡏⢹⠀⠀⠀⠀⠀⠈⣿⣳⡄⠀⠀⠀⠀⠀⠀⢸⠋⡏⣧
⠀⠀⠀⠀⠀⣼⡴⠋⠀⠀⠀⠀⠀⠀⠀⠀⢸⣯⠀⠈⠻⠿⢭⣝⠛⠛⠛⣿⠆⡀⠀⠹⢿⢿⠆⠀⠀⠀⠀⠀⣷⣾⠀⠀⠀⠀⠀⠀⢸⣿⠁⠀⠀⠀⠀⠀⠀⠀⠀⡄⢻
⠀⠀⠀⠀⢰⡿⠋⠀⠀⠀⠀⠀⠀⠀⠀⣰⡿⠏⠀⠀⠀⠀⠀⠈⢷⡀⠀⠹⡿⢻⠇⠀⠀⠀⠀⠀⠀⠀⠀⢸⢿⡇⠀⠀⠀⠀⠀⠀⠀⣿⡆⠀⠀⠀⠀⠀⠀⠀⠀⡇⣿
⠀⠀⠀⠀⣾⠀⠀⠀⠀⠀⠀⣀⣠⣤⣾⠟⠁⠀⠀⠀⠀⠀⠀⠀⠀⣻⠀⠀⣿⢹⣄⠀⠀⠀⠀⠀⠀⠀⠀⠀⣼⠃⠀⠀⠀⠀⠀⠀⠀⠈⣧⣀⠀⠀⠀⠀⠀⠀⢠⣿⡏
⠀⠀⠀⣸⡿⠀⠀⠀⠀⠈⠉⠉⣹⡟⠁⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⡏⠀⠀⢹⣟⠛⠀⠀⠀⠀⠀⠀⠀⠀⠀⣿⠀⠀⠀⠀⠀⠀⠀⠀⠀⠸⣇⠀⠀⠀⠀⠀⢀⣿⣿⠃
⠀⠀⢠⡟⠁⠀⠀⠀⠀⠀⣰⣾⠏⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠈⣇⠀⠀⠸⣷⠀⠀⠀⠀⠀⠀⠀⠀⠀⢠⡏⠀⠀⠀⠀⠀⠀⠀⠀⠀⢠⡟⠀⣴⣤⣷⣠⣾⣿⠇⠀
⠀⠀⢻⣷⠇⠀⠀⠀⠀⡼⢵⠏⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣿⡄⠀⠀⠹⣷⢀⡀⠀⠀⠀⠀⠀⠀⢸⡇⠀⠀⠀⠀⠀⠀⠀⠀⣠⡾⣷⠀⠉⠛⠛⠟⠋⣿⠀⠀
⠀⠀⢈⡏⠀⠀⠀⠀⣀⣵⣾⠂⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠈⠙⠓⠲⠶⣿⡞⠁⠀⠀⠀⠀⠀⣀⣿⠀⠀⠀⠀⠀⠀⠀⠀⠿⠿⠶⢾⣧⣄⣠⣀⣤⣾⣿⠀⠀
⢀⣰⡟⠛⡓⢟⣧⣾⢧⣬⣿⣦⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣿⠁⠀⠀⠀⠀⢀⣰⣿⡗⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠸⣽⣅⣿⣼⠿⠛⠁⠀⠀
⠸⢷⣿⡯⠵⠛⠁⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣿⡄⠀⠀⠀⠀⠾⠟⣿⠃⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠈⠁⠀⠀⠀⠀⠀⠀⠀
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢰⣿⣿⡇⢾⣿⢼⣷⣳⡟⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠉⠙⠓⠚⠙⠛⠉⠉⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀`

type versionCommand struct{ commandBase }

func (c *versionCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	if _, err := c.engine.SendChatMessage(c.message.Name, saturnVersion, c.message.IsWhisper); err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}

type apeCommand struct{ commandBase }

func (c *apeCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	if _, err := c.engine.SendChatMessage(c.message.Name, " "+saturnApe, c.message.IsWhisper); err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}

type coinCommand struct{ commandBase }

func (c *coinCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	n, err := rand.Int(rand.Reader, big.NewInt(2))
	if err != nil {
		return model.FAILED, err
	}
	state := "tail"
	if n.Int64() == 0 {
		state = "head"
	}
	if _, err := c.engine.SendChatMessage(c.message.Name, state, false); err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}

type pingUtilityCommand struct{ commandBase }

func (c *pingUtilityCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	elapsed := time.Duration(0)
	if b := bundle(c.engine); b != nil && b.Ping != nil {
		var err error
		elapsed, err = b.Ping.Ping(ctx)
		if err != nil {
			// Saturn logs ping I/O failures and still reports zero latency.
			elapsed = 0
		}
	}
	if _, err := c.engine.SendChatMessage(c.message.Name, fmt.Sprintf("response time: %d milliseconds", elapsed.Milliseconds()), false); err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}
