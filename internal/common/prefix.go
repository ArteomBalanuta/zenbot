package common

// PrefixController applies a validated live command prefix to the host and its
// current managed concrete replicas.
type PrefixController interface {
	UpdatePrefix(string) (previous string, err error)
}
