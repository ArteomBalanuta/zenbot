package core

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf16"

	"zenbot/internal/common"
)

// SupportReplicaRelay uses only the existing managed replica inventory.
type SupportReplicaRelay struct {
	manager    *ReplicaManager
	adminTrips map[string]struct{}
}

func NewSupportReplicaRelay(manager *ReplicaManager, adminTrips []string) *SupportReplicaRelay {
	trips := make(map[string]struct{}, len(adminTrips))
	for _, trip := range adminTrips {
		trips[trip] = struct{}{}
	}
	return &SupportReplicaRelay{manager: manager, adminTrips: trips}
}

func (r *SupportReplicaRelay) RelayToSupport(ctx context.Context, request common.SupportRelayRequest) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if r == nil || r.manager == nil {
		return fmt.Errorf("support replica relay is not configured")
	}
	support, ok := r.manager.ManagedEngines()["support"]
	if !ok || support == nil {
		return fmt.Errorf("managed support replica is unavailable")
	}
	_, admin := r.adminTrips[request.Trip]
	message := strings.Join(request.Arguments, " ") + " "
	if !admin {
		message = strings.Map(func(char rune) rune {
			if char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == ' ' {
				return char
			}
			return -1
		}, message)
	}
	if request.Anonymous {
		message = "anon_from_hc: " + escapeJava(message)
	} else {
		message = request.Author + ": " + escapeJava(message)
	}
	_, err := support.SendChatMessage("", message, false)
	return err
}

func escapeJava(value string) string {
	var out strings.Builder
	for _, char := range value {
		switch char {
		case '\\':
			out.WriteString(`\\`)
		case '"':
			out.WriteString(`\"`)
		case '\b':
			out.WriteString(`\b`)
		case '\f':
			out.WriteString(`\f`)
		case '\n':
			out.WriteString(`\n`)
		case '\r':
			out.WriteString(`\r`)
		case '\t':
			out.WriteString(`\t`)
		default:
			if char < 0x20 || char > 0x7f {
				if char > 0xffff {
					for _, unit := range utf16.Encode([]rune{char}) {
						fmt.Fprintf(&out, `\u%04X`, unit)
					}
				} else {
					fmt.Fprintf(&out, `\u%04X`, char)
				}
			} else {
				out.WriteRune(char)
			}
		}
	}
	return out.String()
}
