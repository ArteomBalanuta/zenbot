package core

type MessageListener interface {
	Notify(jsonMessage string)
}

/* dummy listener for our zombie engine */
type DummyListener struct{}

func (l *DummyListener) Notify(jsonMessage string) {}
func NewDummyListener() *DummyListener {
	return &DummyListener{}
}
