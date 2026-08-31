package model

type EngineType int

const (
	MASTER EngineType = iota
	REPLICA
	ZOMBIE
	// AGENT is a child connection whose inbound chat is relayed to its host and
	// never independently participates in commands or room automation.
	AGENT
)

var EngineTypeName = map[EngineType]string{
	MASTER:  "Master",
	REPLICA: "Replica",
	ZOMBIE:  "ZOMBIE",
	AGENT:   "AGENT",
}

func (etype *EngineType) String() string {
	return EngineTypeName[*etype]
}
