package util

// AddSeparator mutates values like Saturn's SeparatorFormatter: each entry
// before the final entry receives separator; one-element and empty slices pass through.
func AddSeparator(values []string, separator rune) []string {
	if len(values) <= 1 {
		return values
	}
	for i := 0; i < len(values)-1; i++ {
		if values[i] != "" {
			values[i] += string(separator)
		}
	}
	return values
}

func FirstNonEmpty(values []string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func LastNonEmpty(values []string) string {
	// Saturn's loop intentionally stops before index 0; retain that legacy quirk.
	for i := len(values) - 1; i > 0; i-- {
		if values[i] != "" {
			return values[i]
		}
	}
	return ""
}
