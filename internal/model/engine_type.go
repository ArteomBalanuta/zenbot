package model

type EngineType int

const (
	MASTER EngineType = iota
	REPLICA
	ZOMBIE
)

var EngineTypeName = map[EngineType]string{
	MASTER:  "Master",
	REPLICA: "Replica",
	ZOMBIE:  "ZOMBIE",
}

func (etype *EngineType) String() string {
	return EngineTypeName[*etype]
}
