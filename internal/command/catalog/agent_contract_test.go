package catalog

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestAgentArgumentContractsEncodeTypedInvocations(t *testing.T) {
	tests := []struct {
		name     string
		contract AgentArgumentContract
		input    string
		want     string
		wantErr  bool
	}{
		{name: "empty", contract: emptyArguments(), input: `{}`, want: ""},
		{name: "empty rejects values", contract: emptyArguments(), input: `{"value":"x"}`, wantErr: true},
		{name: "required positional", contract: positionalArguments(requiredString("nick", "Target nick.")), input: `{"nick":"@jill"}`, want: "@jill"},
		{name: "optional positional omitted", contract: positionalArguments(optionalString("reason", "AFK reason.")), input: `{}`, want: ""},
		{name: "ordered positional", contract: positionalArguments(requiredString("nick", "Target nick."), requiredString("message", "Message body.")), input: `{"message":"hello there","nick":"jill"}`, want: "jill hello there"},
		{name: "boolean enabled", contract: booleanStateArguments("enabled", "Enable captcha.", "on", "off"), input: `{"enabled":true}`, want: "on"},
		{name: "boolean disabled", contract: booleanStateArguments("enabled", "Enable captcha.", "on", "off"), input: `{"enabled":false}`, want: "off"},
		{name: "comma list", contract: commaListArguments("trips", "Trips.", "role", "Role.", []string{"ADMIN", "MODERATOR"}), input: `{"trips":["one","two"],"role":"MODERATOR"}`, want: "one,two MODERATOR"},
		{name: "kick exact", contract: kickArguments(), input: `{"mode":"exact","targets":["@jill"]}`, want: "@jill"},
		{name: "kick multiple", contract: kickArguments(), input: `{"mode":"multiple","targets":["jill","nex"]}`, want: "-m jill nex"},
		{name: "kick contains", contract: kickArguments(), input: `{"mode":"contains","targets":["raid"]}`, want: "-c raid"},
		{name: "kick exact rejects multiple", contract: kickArguments(), input: `{"mode":"exact","targets":["jill","nex"]}`, wantErr: true},
		{name: "shadow ban exact", contract: shadowBanArguments(), input: `{"mode":"exact","target":"jill"}`, want: "jill"},
		{name: "shadow ban contains", contract: shadowBanArguments(), input: `{"mode":"contains","target":"raid"}`, want: "-c raid"},
		{name: "automove enable", contract: automoveArguments(), input: `{"operation":"enable"}`, want: "on"},
		{name: "automove disable", contract: automoveArguments(), input: `{"operation":"disable"}`, want: "off"},
		{name: "automove configure", contract: automoveArguments(), input: `{"operation":"configure","source":"purgatory","destination":"lounge"}`, want: "purgatory lounge"},
		{name: "automove requires rooms", contract: automoveArguments(), input: `{"operation":"configure"}`, wantErr: true},
		{name: "notes list", contract: notesArguments(), input: `{"operation":"list"}`, want: ""},
		{name: "notes purge", contract: notesArguments(), input: `{"operation":"purge"}`, want: "purge"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.contract.Encode(json.RawMessage(test.input))
			if test.wantErr {
				if err == nil {
					t.Fatalf("Encode(%s) succeeded with %q; want error", test.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Encode(%s): %v", test.input, err)
			}
			if got != test.want {
				t.Fatalf("Encode(%s) = %q, want %q", test.input, got, test.want)
			}
		})
	}
}

func TestAgentArgumentContractSchemaIsDefensivelyCopied(t *testing.T) {
	arguments := positionalArguments(requiredString("nick", "Target nick."))
	first := arguments.Schema()
	first[0] = '['
	if second := arguments.Schema(); len(second) == 0 || second[0] != '{' {
		t.Fatalf("Schema leaked mutable bytes: %q", second)
	}
}

func TestAgentCatalogHasCompleteSingleSourceContracts(t *testing.T) {
	if err := ValidateAgentContracts(); err != nil {
		t.Fatal(err)
	}
	var actionable int
	hidden := make([]string, 0, 5)
	for _, entry := range Entries() {
		if entry.Agent.Actionable {
			actionable++
			if len(entry.Agent.Examples) == 0 {
				t.Fatalf("actionable command %q has no examples", entry.Canonical)
			}
			for _, example := range entry.Agent.Examples {
				tail, err := entry.Agent.Arguments.Encode(example.Arguments)
				if err != nil {
					t.Fatalf("%s example %q is invalid: %v", entry.Canonical, example.Prompt, err)
				}
				if len(tail) > maxAgentCommandTailBytes {
					t.Fatalf("%s example encoded to %d bytes", entry.Canonical, len(tail))
				}
			}
		} else {
			hidden = append(hidden, entry.Canonical)
		}
	}
	sort.Strings(hidden)
	if actionable != 59 {
		t.Fatalf("actionable command count = %d, want 59", actionable)
	}
	if want := []string{"l", "mine", "whiskey", "ws", "wsa"}; !reflect.DeepEqual(hidden, want) {
		t.Fatalf("hidden commands = %v, want %v", hidden, want)
	}
}

func TestEntriesReturnDefensiveAgentMetadataCopies(t *testing.T) {
	first := Entries()
	first[0].Agent.Targets[0] = "mutated"
	first[0].Agent.Examples[0].Arguments[0] = '['
	second := Entries()
	if second[0].Agent.Targets[0] == "mutated" || strings.HasPrefix(string(second[0].Agent.Examples[0].Arguments), "[") {
		t.Fatalf("Entries leaked agent metadata: %#v", second[0].Agent)
	}
}

func TestAgentCatalogDerivedPoliciesRemainExact(t *testing.T) {
	ban, ok := AgentEntry("ban")
	if !ok || Access(ban) != AgentPermanentBan || !TargetsUser("ban") || !ban.Agent.RunCommandCompatible {
		t.Fatalf("ban policy = %#v", ban.Agent)
	}
	prefix, ok := AgentEntry("prefix")
	if !ok || Access(prefix) != AgentAdmin || TargetsUser("prefix") || prefix.Agent.RunCommandCompatible {
		t.Fatalf("prefix policy = %#v", prefix.Agent)
	}
	if _, ok := AgentEntry("l"); ok {
		t.Fatal("hidden recursive command l was exposed")
	}
}
