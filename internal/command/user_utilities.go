package command

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	release "zenbot"
	"zenbot/internal/model"
)

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
	if err := observeAndReply(ctx, &c.commandBase, release.Version(), c.message.IsWhisper); err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}

type apeCommand struct{ commandBase }

func (c *apeCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	if err := observeAndReply(ctx, &c.commandBase, " "+saturnApe, c.message.IsWhisper); err != nil {
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
	if err := observeAndReply(ctx, &c.commandBase, state, false); err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}

type pingUtilityCommand struct{ commandBase }

func (c *pingUtilityCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	b := bundle(c.engine)
	if b == nil || b.Ping == nil {
		return model.FAILED, fmt.Errorf("ping service unavailable")
	}
	elapsed, err := b.Ping.Ping(ctx)
	if err != nil {
		return model.FAILED, err
	}
	text := fmt.Sprintf("response time: %d milliseconds", elapsed.Milliseconds())
	if err := observeAndReply(ctx, &c.commandBase, text, false); err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}
