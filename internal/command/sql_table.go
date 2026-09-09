package command

import (
	"fmt"
	"strings"
	"unicode/utf16"

	"zenbot/internal/service"
)

// renderSaturnSQLTable mirrors Saturn's TableGenerator followed by
// StringEscapeUtils.escapeJava around SQLServiceImpl's exact fence wrapper.
func renderSaturnSQLTable(table service.SQLTable) string {
	widths := make([]int, len(table.Columns))
	for i, header := range table.Columns {
		widths[i] = javaUTF16Length(header)
	}
	for _, values := range table.Rows {
		for i, value := range values {
			if width := javaUTF16Length(value); width > widths[i] {
				widths[i] = width
			}
		}
	}
	for i := range widths {
		if widths[i]%2 != 0 {
			widths[i]++
		}
	}

	border := func() string {
		var out strings.Builder
		for _, width := range widths {
			if out.Len() == 0 {
				out.WriteByte('+')
			}
			out.WriteString(strings.Repeat("-", width+4))
			out.WriteByte('+')
		}
		return out.String()
	}
	row := func(values []string) string {
		var out strings.Builder
		for i, value := range values {
			padding := 2
			length := javaUTF16Length(value)
			adjusted := length
			if adjusted%2 != 0 {
				adjusted++
			}
			if adjusted < widths[i] {
				padding += (widths[i] - adjusted) / 2
			}
			if i == 0 {
				out.WriteByte('|')
			}
			out.WriteString(strings.Repeat(" ", padding))
			out.WriteString(value)
			if length%2 != 0 {
				out.WriteByte(' ')
			}
			out.WriteString(strings.Repeat(" ", padding))
			out.WriteByte('|')
		}
		return out.String()
	}

	var out strings.Builder
	out.WriteString("\n\n")
	out.WriteString(border())
	out.WriteByte('\n')
	out.WriteString(row(table.Columns))
	out.WriteByte('\n')
	out.WriteString(border())
	for _, values := range table.Rows {
		out.WriteByte('\n')
		out.WriteString(row(values))
	}
	out.WriteByte('\n')
	out.WriteString(border())
	out.WriteString("\n\n")
	return escapeSaturnJava("\n```Text\n" + out.String() + "\n ```")
}

func javaUTF16Length(value string) int { return len(utf16.Encode([]rune(value))) }

func escapeSaturnJava(value string) string {
	var out strings.Builder
	for _, unit := range utf16.Encode([]rune(value)) {
		switch unit {
		case '\b':
			out.WriteString(`\b`)
		case '\n':
			out.WriteString(`\n`)
		case '\t':
			out.WriteString(`\t`)
		case '\f':
			out.WriteString(`\f`)
		case '\r':
			out.WriteString(`\r`)
		case '\\':
			out.WriteString(`\\`)
		case '"':
			out.WriteString(`\"`)
		default:
			if unit < 0x20 || unit > 0x7f {
				out.WriteString(fmt.Sprintf(`\u%04X`, unit))
			} else {
				out.WriteRune(rune(unit))
			}
		}
	}
	return out.String()
}
