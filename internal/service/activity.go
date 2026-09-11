package service

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf16"

	"zenbot/internal/repository"
)

// ActivityService renders the bounded activity query with Saturn's
// SQLServiceImpl/TableGenerator output contract.
type ActivityService struct {
	Repo repository.ActivityRepository
}

func (s *ActivityService) Stats(ctx context.Context, trip string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if s == nil || s.Repo == nil {
		return "", fmt.Errorf("activity repository unavailable")
	}
	stats, err := s.Repo.ActivityStats(ctx, trip)
	if err != nil {
		return "", fmt.Errorf("read activity stats: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if len(stats) == 0 {
		return "No activity found.", nil
	}
	rows := make([][]string, 0, len(stats))
	for _, stat := range stats {
		rows = append(rows, []string{stat.Trip, stat.DayOfWeek, stat.Hour, stat.ProbabilityPercentage})
	}
	return "\\n```Text\\n" + generateSaturnTable([]string{"TRIP", "DAY_OF_WEEK", "HOUR", "PROBABILITY_PERCENTAGE"}, rows) + "\\n ```", nil
}

func generateSaturnTable(headers []string, rows [][]string) string {
	widths := make([]int, len(headers))
	for i, header := range headers {
		widths[i] = javaStringLength(header)
	}
	for _, row := range rows {
		for i, cell := range row {
			if width := javaStringLength(cell); width > widths[i] {
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
		out.WriteByte('+')
		for _, width := range widths {
			out.WriteString(strings.Repeat("-", width+4))
			out.WriteByte('+')
		}
		return out.String()
	}
	cell := func(value string, index int) string {
		length := javaStringLength(value)
		adjusted := length
		if adjusted%2 != 0 {
			adjusted++
		}
		padding := 2 + (widths[index]-adjusted)/2
		return strings.Repeat(" ", padding) + value + map[bool]string{true: " ", false: ""}[length%2 != 0] + strings.Repeat(" ", padding)
	}
	row := func(values []string) string {
		var out strings.Builder
		out.WriteByte('|')
		for i, value := range values {
			out.WriteString(cell(value, i))
			out.WriteByte('|')
		}
		return out.String()
	}

	var out strings.Builder
	out.WriteString("\n\n")
	out.WriteString(border())
	out.WriteByte('\n')
	out.WriteString(row(headers))
	out.WriteByte('\n')
	out.WriteString(border())
	for _, values := range rows {
		out.WriteByte('\n')
		out.WriteString(row(values))
	}
	out.WriteByte('\n')
	out.WriteString(border())
	out.WriteString("\n\n")
	return out.String()
}

// Java's TableGenerator uses String.length(), which counts UTF-16 code units.
func javaStringLength(value string) int { return len(utf16.Encode([]rune(value))) }
