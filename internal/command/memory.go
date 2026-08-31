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
	reply(&c.commandBase, formatMemoryReport(stats))
	return model.SUCCESSFUL, nil
}

// formatMemoryReport preserves Saturn's labels while adapting them to Go runtime metrics:
// Alloc, HeapIdle, HeapSys, and Sys respectively.
func formatMemoryReport(stats runtime.MemStats) string {
	return fmt.Sprintf("JVM Used Memory : %d MB \\nJVM Free Memory : %d MB \\nJVM Total Memory: %d MB \\nJVM Max Memory  : %d MB \\n",
		stats.Alloc/memoryMiB,
		stats.HeapIdle/memoryMiB,
		stats.HeapSys/memoryMiB,
		stats.Sys/memoryMiB,
	)
}
