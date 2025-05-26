package contracts

type EngineInterface interface {
	EnqueueMessageForSending(msg string)
	Start()
	Stop()
}
