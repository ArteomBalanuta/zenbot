package util

import "strings"

// Command builds Saturn's command-only JSON fragment, including its unusual
// lack of a space before the closing brace.
func Command(cmd string) string { return `{ "cmd": "` + escapeJSONString(cmd) + `"}` }

func CommandWithValue(cmd, key, value string) string {
	return `{ "cmd": "` + escapeJSONString(cmd) + `", "` + escapeJSONString(key) + `": "` + escapeJSONString(value) + `"}`
}

func CommandWithValues(cmd, firstKey, firstValue, secondKey, secondValue string) string {
	return `{ "cmd": "` + escapeJSONString(cmd) + `", "` + escapeJSONString(firstKey) + `": "` + escapeJSONString(firstValue) + `", "` + escapeJSONString(secondKey) + `": "` + escapeJSONString(secondValue) + `" }`
}

// escapeJSONString follows the required Commons Text-compatible mapping for
// JSON strings without encoding/json's additional HTML escaping.
func escapeJSONString(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))
	for _, r := range value {
		switch r {
		case '"':
			builder.WriteString(`\"`)
		case '\\':
			builder.WriteString(`\\`)
		case '\b':
			builder.WriteString(`\b`)
		case '\t':
			builder.WriteString(`\t`)
		case '\n':
			builder.WriteString(`\n`)
		case '\f':
			builder.WriteString(`\f`)
		case '\r':
			builder.WriteString(`\r`)
		default:
			if r < 0x20 {
				builder.WriteString(`\u00`)
				const hex = "0123456789ABCDEF"
				builder.WriteByte(hex[r>>4])
				builder.WriteByte(hex[r&0xf])
			} else {
				builder.WriteRune(r)
			}
		}
	}
	return builder.String()
}
