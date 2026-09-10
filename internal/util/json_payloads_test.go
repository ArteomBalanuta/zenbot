package util

import (
	"encoding/json"
	"testing"
)

func TestCommandPayloadHasExactSaturnSpacing(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{name: "command only", got: Command("ban"), want: `{ "cmd": "ban"}`},
		{name: "one value", got: CommandWithValue("ban", "nick", "mer\"c\\"), want: `{ "cmd": "ban", "nick": "mer\"c\\"}`},
		{name: "two values", got: CommandWithValues("kick", "nick", "me\\", "to", "x\"y"), want: `{ "cmd": "kick", "nick": "me\\", "to": "x\"y" }`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Fatalf("payload = %q, want %q", tc.got, tc.want)
			}
			var decoded map[string]any
			if err := json.Unmarshal([]byte(tc.got), &decoded); err != nil {
				t.Fatalf("payload is not JSON: %v", err)
			}
		})
	}
}

func TestCommandPayloadEscapesKeysValuesAndControlsExactly(t *testing.T) {
	got := CommandWithValue("q\"\\", "k\n\r\t\b\f\x00\x1f", "v\n\r\t\b\f\x00\x1f/é")
	want := `{ "cmd": "q\"\\", "k\n\r\t\b\f\u0000\u001F": "v\n\r\t\b\f\u0000\u001F/é"}`
	if got != want {
		t.Fatalf("payload = %q, want %q", got, want)
	}
}

func TestCommandPayloadPreservesEmptyStringsAndTrailingBackslash(t *testing.T) {
	if got, want := CommandWithValues("", "", "", "", "\\"), `{ "cmd": "", "": "", "": "\\" }`; got != want {
		t.Fatalf("payload = %q, want %q", got, want)
	}
}
