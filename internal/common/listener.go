package common

import "strings"

type Listener interface {
	Notify(jsonMessage string)
}

/* dummy listener for our zombie engine */
type DummyListener struct{}

func (l *DummyListener) Notify(jsonMessage string) {}
func NewDummyListener() *DummyListener {
	return &DummyListener{}
}

func ParseCommandText(text, prefix string) string {
	afterPrefix := text[len(prefix):]
	fields := strings.Fields(afterPrefix)
	return fields[0]
}
