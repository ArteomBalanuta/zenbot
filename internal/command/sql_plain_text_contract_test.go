package command

import (
	"strings"
	"testing"
	"zenbot/internal/service"
)

func TestSQLDisplayPreservesCellTextWithoutProtocolEscaping(t *testing.T) {
	cell := "quote \" path C:\\new\nChișinău 🌦️\t"
	got := renderSaturnSQLTable(service.SQLTable{Columns: []string{"value"}, Rows: [][]string{{cell}}})
	if !strings.HasPrefix(got, "\n```Text\n") || !strings.Contains(got, cell) {
		t.Fatalf("cell text escaped before serialization: %q", got)
	}
}
