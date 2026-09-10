package util

import (
	"errors"
	"testing"
)

func TestNormalizeNickTargetMatchesSaturnTrimAndOneMarkerRules(t *testing.T) {
	cases := []struct {
		name string
		raw  *string
		want string
	}{
		{name: "trim and marker", raw: ptr("  @alice  "), want: "alice"},
		{name: "remove exactly one", raw: ptr("@@alice"), want: "@alice"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeNickTarget(tc.raw)
			if err != nil || got != tc.want {
				t.Fatalf("NormalizeNickTarget() = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestNormalizeNickTargetRejectsNullBlankAndBareMarker(t *testing.T) {
	for _, raw := range []*string{nil, ptr("  "), ptr(" @ ")} {
		got, err := NormalizeNickTarget(raw)
		if got != "" || !errors.Is(err, ErrBlankNickTarget) || err.Error() != "Nick target cannot be blank" {
			t.Fatalf("NormalizeNickTarget(%v) = %q, %v", raw, got, err)
		}
	}
}

func TestCanonicalNickUsesLocaleIndependentLowercase(t *testing.T) {
	got, err := CanonicalNick(ptr(" @ÄLICE "))
	if err != nil || got != "älice" {
		t.Fatalf("CanonicalNick() = %q, %v; want älice", got, err)
	}
}

func TestSameNickIsNullSafeAndCanonical(t *testing.T) {
	if !SameNick(ptr("@Alice"), ptr(" alice ")) {
		t.Fatal("equivalent nick targets did not compare equal")
	}
	if SameNick(ptr("alice"), ptr("bob")) || SameNick(nil, ptr("alice")) || SameNick(ptr("alice"), nil) || SameNick(ptr(" @ "), ptr("@")) {
		t.Fatal("invalid or different nick targets compared equal")
	}
}
