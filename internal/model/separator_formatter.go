package model

// AddSeparator mutates values in place, appending separator to each non-nil
// value before the last non-nil value at an index greater than zero.
//
// The pointer-shaped input preserves Java's null elements and reference
// identity for the stop condition. Replacement still follows Java's
// value-based indexOf behavior, so duplicate string values replace the first
// matching slot.
func AddSeparator(values []*string, separator rune) []*string {
	if len(values) <= 1 {
		return values
	}

	last := separatorLastPointer(values)
	for _, value := range values {
		if value == last {
			break
		}
		if value == nil {
			continue
		}

		formatted := *value + string(separator)
		for i, candidate := range values {
			if candidate != nil && *candidate == *value {
				values[i] = &formatted
				break
			}
		}
	}
	return values
}

// GetFirst returns the first non-nil value. The boolean distinguishes an
// absent value from a present empty string.
func GetFirst(values []*string) (string, bool) {
	for _, value := range values {
		if value != nil {
			return *value, true
		}
	}
	return "", false
}

// GetLast returns the last non-nil value, ignoring index zero to match the
// Saturn loop's index-0 exclusion. The boolean distinguishes absence from an
// empty string.
func GetLast(values []*string) (string, bool) {
	last := separatorLastPointer(values)
	if last == nil {
		return "", false
	}
	return *last, true
}

func separatorLastPointer(values []*string) *string {
	for i := len(values) - 1; i > 0; i-- {
		if values[i] != nil {
			return values[i]
		}
	}
	return nil
}
