package common

// CommandAvailability lets a concrete owner narrow its static capability
// interfaces to dependencies actually installed at composition time.
type CommandAvailability interface {
	CommandAvailable(canonical string) bool
}
