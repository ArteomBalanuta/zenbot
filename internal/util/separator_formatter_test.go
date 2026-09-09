package util

import "testing"

func TestSeparatorFormatterPreservesSaturnNullAndLastElementSemantics(t *testing.T) {
	values := []string{"a", "b", "c"}
	if got := AddSeparator(values, ','); got[0] != "a," || got[1] != "b," || got[2] != "c" {
		t.Fatalf("separator=%v", got)
	}
	if got := FirstNonEmpty([]string{"", "first", "last"}); got != "first" {
		t.Fatalf("first=%q", got)
	}
	if got := LastNonEmpty([]string{"first", "", "last"}); got != "last" {
		t.Fatalf("last=%q", got)
	}
}
