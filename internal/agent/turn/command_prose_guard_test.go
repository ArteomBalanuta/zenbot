package turn

import (
	"testing"

	"zenbot/internal/agent/api"
	agenttool "zenbot/internal/agent/tool"
	"zenbot/internal/agent/tool/contract"
)

func commandDefinition(t *testing.T) contract.Definition {
	t.Helper()
	descriptor, err := (agenttool.RunCommand{}).Descriptor(api.Context{})
	if err != nil {
		t.Fatal(err)
	}
	return contract.NewDefinition(descriptor)
}

func TestCommandProseGuardFindsOnlyAdvertisedSourceShapedCommands(t *testing.T) {
	guard := NewCommandProseGuard([]contract.Definition{commandDefinition(t)})
	for _, tc := range []struct {
		content string
		want    string
		found   bool
	}{
		{"Use `weather Tokyo` now", "weather", true},
		{"```\n!W Tokyo\n```", "w", true},
		{"~~~\nweather Tokyo\n~~~", "weather", true},
		{"weather Tokyo", "", false},
		{"List.of() is ordinary prose", "", false},
		{"`whois alice`", "", false},
		{"`kick alice`", "", false},
	} {
		got, found := guard.FindCommand(tc.content)
		if got != tc.want || found != tc.found {
			t.Fatalf("FindCommand(%q) = (%q, %v), want (%q, %v)", tc.content, got, found, tc.want, tc.found)
		}
	}
}

func TestCommandProseGuardFailsClosedForMalformedDefinitionAndCalls(t *testing.T) {
	guard := NewCommandProseGuard([]contract.Definition{{Name: "run_command", Parameters: []byte(`{"type":"object","properties":{"command":{"enum":["weather",7,null]}}}`)}})
	if _, found := guard.FindCommand("`weather Tokyo`"); found {
		t.Fatal("malformed advertised schema granted command recognition")
	}
	guard = NewCommandProseGuard([]contract.Definition{commandDefinition(t)})
	if !guard.MatchesRunCommand("weather", "run_command", map[string]any{"command": "weather", "arguments": "Tokyo"}) {
		t.Fatal("exact advertised command call was rejected")
	}
	for _, call := range []struct {
		name string
		args map[string]any
	}{
		{"run_command", map[string]any{"command": "Weather"}},
		{"run_command", map[string]any{"command": "weather", "arguments": 7}},
		{"run_command", map[string]any{"command": "weather", "extra": true}},
		{"run_command", map[string]any{"command": "kick"}},
		{"other", map[string]any{"command": "weather"}},
	} {
		if guard.MatchesRunCommand("weather", call.name, call.args) {
			t.Fatalf("invalid call accepted: %#v", call)
		}
	}
}
