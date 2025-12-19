package repository

type DummyImpl struct {
}

func (r *DummyImpl) LogMessage(trip, name, hash, message, channel string) (int64, error) {
	return -1, nil
}

func (r *DummyImpl) LogPresence(trip, name, hash, eventType, channel string) (int64, error) {
	return -1, nil
}

type Repository interface {
	LogMessage(trip, name, hash, message, channel string) (int64, error)
	LogPresence(trip, name, hash, eventType, channel string) (int64, error)
}
