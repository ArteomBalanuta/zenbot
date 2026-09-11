package live

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"zenbot/internal/agent/assemble"
	"zenbot/internal/agent/tool/contract"
)

const (
	genericFailureReply  = "failed: the agent could not answer that request."
	maxFailureReplyRunes = 2000
	maxReceiptIDRunes    = 80
)

// FailureReply renders bounded execution receipts for an incomplete turn.
// It deliberately excludes causes, arguments, and tool result bodies.
func FailureReply(cause error) string {
	var incomplete *IncompleteTurnError
	if !errors.As(cause, &incomplete) || incomplete == nil || incomplete.Completion.Observations == nil {
		return genericFailureReply
	}
	receipts := incomplete.Completion.Observations.Index(0)
	if len(receipts) == 0 {
		return genericFailureReply
	}

	reply := "failed: the agent's answer is incomplete. execution receipts: "
	included := 0
	for index, receipt := range receipts {
		separator := ""
		if included > 0 {
			separator = "; "
		}
		candidate := reply + separator + renderFailureReceipt(receipt)
		remaining := len(receipts) - index - 1
		tail := "."
		if remaining > 0 {
			tail = fmt.Sprintf("; omitted receipts: %d.", remaining)
		}
		if len([]rune(candidate+tail)) > maxFailureReplyRunes {
			break
		}
		reply = candidate
		included++
	}
	if omitted := len(receipts) - included; omitted > 0 {
		reply += fmt.Sprintf("; omitted receipts: %d.", omitted)
	} else {
		reply += "."
	}
	return reply
}

func renderFailureReceipt(receipt assemble.ObservationReceipt) string {
	return fmt.Sprintf(
		"call %s tool %s: status=%s, effect=%s, deliveries=%d, actions=%d",
		boundedReceiptIdentity(receipt.CallID),
		boundedReceiptIdentity(receipt.Tool),
		failureReceiptStatus(receipt),
		failureReceiptEffect(receipt),
		receipt.DeliveryCount,
		receipt.ActionCount,
	)
}

func failureReceiptStatus(receipt assemble.ObservationReceipt) string {
	if receipt.Code == "ACTION_NOT_EXECUTED" {
		return "skipped"
	}
	if receipt.Code == "COMMAND_REJECTED" {
		return "rejected"
	}
	if receipt.Status == "success" {
		return "succeeded"
	}
	return "failed"
}

func failureReceiptEffect(receipt assemble.ObservationReceipt) string {
	switch receipt.EffectState {
	case contract.EffectCommitted:
		return "committed"
	case contract.EffectPartial:
		return "partial"
	case contract.EffectUnknown:
		return "unknown"
	case contract.EffectNotStarted:
		return "not-started"
	case contract.EffectNotCommitted:
		return "not-committed"
	default:
		return "unspecified"
	}
}

func boundedReceiptIdentity(value string) string {
	clean := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return ' '
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, value)
	clean = strings.Join(strings.Fields(clean), " ")
	if clean == "" {
		clean = "unknown"
	}
	runes := []rune(clean)
	if len(runes) > maxReceiptIDRunes {
		clean = string(runes[:maxReceiptIDRunes-3]) + "..."
	}
	return strconv.QuoteToGraphic(clean)
}
