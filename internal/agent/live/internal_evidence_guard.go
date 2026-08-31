package live

import "strings"

const internalToolEvidenceMarker = "[Internal tool evidence from "

func containsInternalToolEvidence(content string) bool {
	return strings.Contains(content, internalToolEvidenceMarker)
}
