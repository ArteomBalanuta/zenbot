package common

import "context"

// SupportRelayRequest is the source-shaped relay input for an existing support replica.
type SupportRelayRequest struct {
	Author    string
	Trip      string
	Arguments []string
	Anonymous bool
}

// SupportReplicaRelay sends one message through an already managed support replica.
type SupportReplicaRelay interface {
	RelayToSupport(context.Context, SupportRelayRequest) error
}
