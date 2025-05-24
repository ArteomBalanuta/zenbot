package model

type EngineInterface interface {
	EnqueueMessageForSending(msg string)
}
