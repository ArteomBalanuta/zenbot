package contract

import (
	"encoding/json"
	"fmt"
	"strings"
)

const ManifestVersion = "1.1"

// Example binds a natural-language request to valid structured tool arguments.
type Example struct {
	Prompt    string          `json:"prompt"`
	Arguments json.RawMessage `json:"arguments"`
}

// RoutingMetadata gives the model semantic selection guidance without changing
// the local execution contract.
type RoutingMetadata struct {
	Aliases  []string  `json:"aliases"`
	Targets  []string  `json:"targets"`
	UseWhen  []string  `json:"useWhen"`
	Examples []Example `json:"examples"`
}

// DescriptorOption adds optional metadata while preserving existing descriptor
// construction call sites.
type DescriptorOption func(*Descriptor) error

// WithMaxModelResultBytes bounds the complete provider-visible observation
// envelope. The floor leaves room for typed identity and truncation metadata.
func WithMaxModelResultBytes(maxBytes int) DescriptorOption {
	return func(descriptor *Descriptor) error {
		if descriptor == nil {
			return &ContractError{"nil descriptor"}
		}
		if maxBytes < 512 {
			return &ContractError{"model result limit must be at least 512 bytes"}
		}
		descriptor.maxModelResultBytes = maxBytes
		return nil
	}
}

// WithPrimaryIntent assigns the stable semantic operation owned by this
// provider. A caller-visible manifest may expose only one provider per intent.
func WithPrimaryIntent(intent string) DescriptorOption {
	intent = strings.TrimSpace(intent)
	return func(descriptor *Descriptor) error {
		if descriptor == nil {
			return &ContractError{"nil descriptor"}
		}
		if !nameRE.MatchString(intent) {
			return &ContractError{"invalid primary intent"}
		}
		descriptor.primaryIntent = intent
		return nil
	}
}

// WithInternalFallback marks a provider as an executor-owned implementation
// detail. It remains callable by trusted code but is never offered to a model.
func WithInternalFallback() DescriptorOption {
	return func(descriptor *Descriptor) error {
		if descriptor == nil {
			return &ContractError{"nil descriptor"}
		}
		descriptor.internalFallback = true
		return nil
	}
}

// WithRouting attaches validated semantic routing metadata to a descriptor.
func WithRouting(metadata RoutingMetadata) DescriptorOption {
	captured := cloneRoutingMetadata(metadata)
	return func(descriptor *Descriptor) error {
		if descriptor == nil {
			return &ContractError{"nil descriptor"}
		}
		if err := validateRoutingMetadata(captured, descriptor.parameters); err != nil {
			return err
		}
		descriptor.routing = cloneRoutingMetadata(captured)
		return nil
	}
}

func validateRoutingMetadata(metadata RoutingMetadata, parameters json.RawMessage) error {
	for label, values := range map[string][]string{
		"alias":    metadata.Aliases,
		"target":   metadata.Targets,
		"use-when": metadata.UseWhen,
	} {
		for _, value := range values {
			if strings.TrimSpace(value) == "" {
				return &ContractError{fmt.Sprintf("blank routing %s", label)}
			}
		}
	}
	for _, example := range metadata.Examples {
		if strings.TrimSpace(example.Prompt) == "" {
			return &ContractError{"blank routing example prompt"}
		}
		if !json.Valid(example.Arguments) {
			return &ContractError{"invalid routing example arguments"}
		}
		if err := ValidateArguments(parameters, example.Arguments); err != nil {
			return &ContractError{fmt.Sprintf("routing example does not satisfy parameters: %v", err)}
		}
	}
	return nil
}

func cloneRoutingMetadata(metadata RoutingMetadata) RoutingMetadata {
	result := RoutingMetadata{
		Aliases: append([]string(nil), metadata.Aliases...),
		Targets: append([]string(nil), metadata.Targets...),
		UseWhen: append([]string(nil), metadata.UseWhen...),
	}
	result.Examples = make([]Example, len(metadata.Examples))
	for index, example := range metadata.Examples {
		result.Examples[index] = Example{
			Prompt:    example.Prompt,
			Arguments: clone(example.Arguments),
		}
	}
	return result
}

// Manifest is the caller-filtered, versioned inventory used to construct one
// provider request and to inspect the same local execution contracts.
type Manifest struct {
	Version string          `json:"version"`
	Tools   []ManifestEntry `json:"tools"`
}

type Resources struct {
	Reads  []string `json:"reads"`
	Writes []string `json:"writes"`
}

// ManifestEntry is the JSON-safe representation of one executable tool.
type ManifestEntry struct {
	Name                    string          `json:"name"`
	PrimaryIntent           string          `json:"primaryIntent"`
	Actionable              bool            `json:"actionable"`
	Label                   string          `json:"label"`
	Description             string          `json:"description"`
	Category                string          `json:"category"`
	Routing                 RoutingMetadata `json:"routing"`
	Parameters              json.RawMessage `json:"parameters"`
	ResultSchema            json.RawMessage `json:"resultSchema"`
	Access                  Access          `json:"access"`
	Effect                  Effect          `json:"effect"`
	ResultMode              ResultMode      `json:"resultMode"`
	RequiredCapabilities    []string        `json:"capabilities"`
	RequiredSuccessfulTools []string        `json:"prerequisites"`
	Idempotent              bool            `json:"idempotent"`
	TimeoutMs               int64           `json:"timeoutMs"`
	MaxModelResultBytes     int             `json:"maxModelResultBytes"`
	Resources               Resources       `json:"resources"`
	WhenNotUse              []string        `json:"whenNotUse"`
}

func NewManifestEntry(descriptor Descriptor) ManifestEntry {
	return ManifestEntry{
		Name:                    descriptor.Name(),
		PrimaryIntent:           descriptor.PrimaryIntent(),
		Actionable:              true,
		Label:                   descriptor.Label(),
		Description:             descriptor.Description(),
		Category:                descriptor.Category(),
		Routing:                 descriptor.Routing(),
		Parameters:              descriptor.Parameters(),
		ResultSchema:            descriptor.ResultSchema(),
		Access:                  descriptor.Access(),
		Effect:                  descriptor.Effect(),
		ResultMode:              descriptor.ResultMode(),
		RequiredCapabilities:    descriptor.RequiredCapabilities(),
		RequiredSuccessfulTools: descriptor.RequiredSuccessfulTools(),
		Idempotent:              descriptor.Idempotent(),
		TimeoutMs:               descriptor.Timeout().Milliseconds(),
		MaxModelResultBytes:     descriptor.MaxModelResultBytes(),
		Resources: Resources{
			Reads:  descriptor.ResourceReads(),
			Writes: descriptor.ResourceWrites(),
		},
		WhenNotUse: append([]string(nil), descriptor.whenNotUse...),
	}
}

// Definition projects rich local metadata into the compact OpenAI function
// shape. Local schema and result validation remain authoritative.
func (entry ManifestEntry) Definition() Definition {
	metadata := []string{
		entry.Description,
		"Primary intent: " + entry.PrimaryIntent,
		"Label: " + entry.Label,
		"Category: " + entry.Category,
		"Access: " + string(entry.Access),
		"Effect: " + string(entry.Effect),
		"Result mode: " + string(entry.ResultMode),
		fmt.Sprintf("Idempotent: %t", entry.Idempotent),
	}
	if len(entry.Routing.Aliases) > 0 {
		metadata = append(metadata, "Aliases: "+strings.Join(entry.Routing.Aliases, ", "))
	}
	if len(entry.Routing.Targets) > 0 {
		metadata = append(metadata, "Targets: "+strings.Join(entry.Routing.Targets, ", "))
	}
	if len(entry.Routing.UseWhen) > 0 {
		metadata = append(metadata, "Use when: "+strings.Join(entry.Routing.UseWhen, " "))
	}
	for _, example := range entry.Routing.Examples {
		metadata = append(metadata, "Example: "+example.Prompt+" => "+string(CanonicalJSON(example.Arguments)))
	}
	if len(entry.RequiredCapabilities) > 0 {
		metadata = append(metadata, "Required capabilities: "+strings.Join(entry.RequiredCapabilities, ", "))
	}
	if len(entry.RequiredSuccessfulTools) > 0 {
		metadata = append(metadata, "Required successful tools: "+strings.Join(entry.RequiredSuccessfulTools, ", "))
	}
	if len(entry.Resources.Reads) > 0 {
		metadata = append(metadata, "Reads: "+strings.Join(entry.Resources.Reads, ", "))
	}
	if len(entry.Resources.Writes) > 0 {
		metadata = append(metadata, "Writes: "+strings.Join(entry.Resources.Writes, ", "))
	}
	metadata = append(metadata, "When not to use: "+strings.Join(entry.WhenNotUse, " "))
	return Definition{
		Name:        entry.Name,
		Description: strings.Join(metadata, "\n"),
		Parameters:  clone(entry.Parameters),
	}
}

// ProviderDefinition keeps the model-facing contract compact while preserving
// the complete inspectable metadata in ManifestEntry.
func (entry ManifestEntry) ProviderDefinition() Definition {
	// Access is already caller-filtered and aliases are not callable function
	// names, so neither belongs in the scarce provider-facing description.
	header := []string{"I=" + entry.PrimaryIntent, "L=" + entry.Label}
	if len(entry.Routing.Targets) > 0 {
		header = append(header, "Targets="+strings.Join(entry.Routing.Targets, ","))
	}
	parts := []string{strings.Join(header, ";")}
	if len(entry.Routing.UseWhen) == 0 {
		parts = append(parts, entry.Description)
	}
	if len(entry.Routing.UseWhen) > 0 {
		parts = append(parts, "Use: "+strings.Join(entry.Routing.UseWhen, " "))
	}
	if len(entry.WhenNotUse) > 0 {
		parts = append(parts, "Avoid: "+strings.Join(entry.WhenNotUse, " "))
	}
	if len(entry.Routing.Examples) > 0 {
		parts = append(parts, "Args="+string(CanonicalJSON(entry.Routing.Examples[0].Arguments)))
	}
	policy := string(entry.Effect) + "/" + string(entry.ResultMode)
	if entry.Idempotent {
		policy += "/IDEMPOTENT"
	} else {
		policy += "/SEQUENTIAL"
	}
	parts = append(parts, "P="+policy)
	return Definition{Name: entry.Name, Description: strings.Join(parts, ";"), Parameters: clone(entry.Parameters)}
}

func (manifest Manifest) Definitions() []Definition {
	definitions := make([]Definition, len(manifest.Tools))
	for index, entry := range manifest.Tools {
		definitions[index] = entry.Definition()
	}
	return definitions
}

func (manifest Manifest) ProviderDefinitions() []Definition {
	definitions := make([]Definition, len(manifest.Tools))
	for index, entry := range manifest.Tools {
		definitions[index] = entry.ProviderDefinition()
	}
	return definitions
}
