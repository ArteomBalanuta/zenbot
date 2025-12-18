package model

type EngineType int

const (
	MASTER EngineType = iota
	REPLICA
	DUMMY
)

var EngineTypeName = map[EngineType]string{
	MASTER:  "Master",
	REPLICA: "Replica",
	DUMMY:   "Dummy",
}

func (etype *EngineType) String() string {
	return EngineTypeName[*etype]
}
