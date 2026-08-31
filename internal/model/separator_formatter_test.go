package model

import "testing"

func stringPointer(value string) *string { return &value }

func TestSeparatorFormatterSaturnFixture(t *testing.T) {
	values := []*string{nil, stringPointer("test"), stringPointer("test2"), stringPointer("test3"), stringPointer("test4"), nil}
	firstSlot := &values[0]

	first, ok := GetFirst(values)
	if !ok || first != "test" {
		t.Fatalf("GetFirst() = %q, %v; want %q, true", first, ok, "test")
	}
	last, ok := GetLast(values)
	if !ok || last != "test4" {
		t.Fatalf("GetLast() = %q, %v; want %q, true", last, ok, "test4")
	}

	actual := AddSeparator(values, ',')
	want := []*string{nil, stringPointer("test,"), stringPointer("test2,"), stringPointer("test3,"), stringPointer("test4"), nil}
	assertStringPointersEqual(t, actual, want)
	if &actual[0] != firstSlot {
		t.Fatal("AddSeparator did not return the original slice")
	}
}

func TestSeparatorFormatterGetLastExcludesIndexZero(t *testing.T) {
	value := stringPointer("only")
	for _, values := range [][]*string{{}, {value}, {value, nil}, {nil, nil}} {
		if last, ok := GetLast(values); ok || last != "" {
			t.Fatalf("GetLast(%v) = %q, %v; want empty, false", values, last, ok)
		}
	}

	values := []*string{stringPointer("first"), nil, stringPointer("last")}
	last, ok := GetLast(values)
	if !ok || last != "last" {
		t.Fatalf("GetLast() = %q, %v; want %q, true", last, ok, "last")
	}
}

func TestSeparatorFormatterAddSeparatorPreservesNullsAndShortSlices(t *testing.T) {
	for _, values := range [][]*string{{}, {nil}, {stringPointer("one")}} {
		before := append([]*string(nil), values...)
		actual := AddSeparator(values, ',')
		if len(actual) != len(before) {
			t.Fatalf("AddSeparator(%v) changed length to %d", before, len(actual))
		}
		for i := range before {
			if actual[i] != before[i] {
				t.Fatalf("AddSeparator(%v) changed short slice at index %d", before, i)
			}
		}
		if len(values) > 0 && &actual[0] != &values[0] {
			t.Fatal("AddSeparator did not preserve short-slice identity")
		}
	}

	values := []*string{nil, stringPointer("middle"), nil, stringPointer("last")}
	actual := AddSeparator(values, '|')
	if actual[0] != nil || actual[2] != nil || *actual[3] != "last" || *actual[1] != "middle|" {
		t.Fatalf("AddSeparator() = %v; want [nil middle| nil last]", actual)
	}
}

func TestSeparatorFormatterGoReferenceIdentityAdaptation(t *testing.T) {
	firstDuplicate := stringPointer("duplicate")
	lastDuplicate := stringPointer("duplicate")
	values := []*string{stringPointer("prefix"), firstDuplicate, lastDuplicate}

	actual := AddSeparator(values, ',')
	if *actual[0] != "prefix," || *actual[1] != "duplicate," || actual[2] != lastDuplicate {
		t.Fatalf("AddSeparator() = %v; want value-based index lookup before pointer-identity stop", actual)
	}
}

func assertStringPointersEqual(t *testing.T, actual, want []*string) {
	t.Helper()
	if len(actual) != len(want) {
		t.Fatalf("length = %d; want %d", len(actual), len(want))
	}
	for i := range want {
		if actual[i] == nil || want[i] == nil {
			if actual[i] != want[i] {
				t.Fatalf("index %d = %v; want %v", i, actual[i], want[i])
			}
			continue
		}
		if *actual[i] != *want[i] {
			t.Fatalf("index %d = %q; want %q", i, *actual[i], *want[i])
		}
	}
}
