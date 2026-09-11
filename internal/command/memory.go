package command

import (
	"context"
	"fmt"
	"runtime"

	"zenbot/internal/model"
)

const memoryMiB = 1024 * 1024

type memoryCommand struct{ commandBase }

func (c *memoryCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}

	runtime.GC()
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	text := formatMemoryReport(stats)
	if err := observeAndReply(ctx, &c.commandBase, text, c.message.IsWhisper || c.message.Whisper || c.message.Type == "whisper"); err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}

func formatMemoryReport(stats runtime.MemStats) string {
	return fmt.Sprintf("Go Alloc: %d MiB \nGo HeapIdle: %d MiB \nGo HeapSys: %d MiB \nGo Sys: %d MiB \n",
		stats.Alloc/memoryMiB,
		stats.HeapIdle/memoryMiB,
		stats.HeapSys/memoryMiB,
		stats.Sys/memoryMiB,
	)
}
