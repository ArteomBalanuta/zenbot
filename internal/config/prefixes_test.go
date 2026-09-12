package config

import (
	"github.com/BurntSushi/toml"
	"reflect"
	"testing"
)

func TestConfigRejectsInvalidExplicitPrefixes(t *testing.T) {
	for _, input := range []string{`cmdPrefixes = []`, `cmdPrefixes = [""]`, `cmdPrefixes = ["a b"]`, `cmdPrefixes = [".", "\n"]`} {
		var cfg Config
		if _, err := toml.Decode(input, &cfg); err == nil {
			t.Fatalf("accepted invalid config %s", input)
		}
	}
}

func TestConfigPrefixListOverridesLegacyAndDeduplicates(t *testing.T) {
	for _, test := range []struct {
		input string
		want  []string
	}{
		{`cmdPrefix = "!"`, []string{"!"}},
		{"cmdPrefix = \"!\"\ncmdPrefixes = [\".\",\"*\",\".\"]", []string{".", "*"}},
		{`cmdPrefixes = ["..", "."]`, []string{"..", "."}},
		{``, []string{"*"}},
	} {
		var cfg Config
		if _, err := toml.Decode(test.input, &cfg); err != nil {
			t.Fatal(err)
		}
		prefixes, err := cfg.CommandPrefixes()
		if err != nil || !reflect.DeepEqual(prefixes, test.want) {
			t.Fatalf("%s: prefixes=%v err=%v", test.input, prefixes, err)
		}
	}
}
