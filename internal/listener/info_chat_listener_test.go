package listener

import (
	"reflect"
	"strings"
	"testing"
)

func Test_SliceUpTo(t *testing.T) {
	s := "merc was banished to ?purgatory"
	actual := SliceUpTo(s, strings.Index(s, " was banished"))
	expected := "merc"
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("SliceUpTo() = %+v; want %+v", actual, expected)
	}
}
func Test_SliceDownTo(t *testing.T) {
	s := "merc was banished to ?purgatory"
	actual := SliceDownTo(s, strings.Index(s, "?"))
	expected := "purgatory"
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("SliceDownTo() = %+v; want %+v", actual, expected)
	}
}
