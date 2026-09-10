package turn

import "strings"

// IsInternalToolEvidence identifies private persisted tool context.
func IsInternalToolEvidence(content string) bool {
	return strings.HasPrefix(content, "[Internal tool evidence from ")
}

// InternalToolEvidenceName returns the tool name from a valid evidence marker.
func InternalToolEvidenceName(content string) string {
	const prefix = "[Internal tool evidence from "
	if !strings.HasPrefix(content, prefix) {
		return ""
	}
	end := strings.Index(content[len(prefix):], "]\n")
	if end <= 0 {
		return ""
	}
	return content[len(prefix) : len(prefix)+end]
}
