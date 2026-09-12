package factory

import (
	"reflect"
	"testing"
	"zenbot/internal/config"
	"zenbot/internal/model"
)

func TestFactoryComposesPrefixesForHostAndReplicas(t *testing.T) {
	cfg := &config.Config{Channel: "master", Name: "bot", CmdPrefix: "!", CmdPrefixes: []string{".", "*"}}
	repo := &autoMoveCompositionRepository{}
	host, err := NewEngineWithOptions(model.MASTER, cfg, repo, EngineOptions{})
	if err != nil {
		t.Fatal(err)
	}
	factory := ReplicaFactory{Config: cfg, Repository: repo}
	replica, err := factory.NewReplica(nil, "room")
	if err != nil {
		t.Fatal(err)
	}
	cfg.CmdPrefixes[0] = "mutated"
	if !reflect.DeepEqual(host.GetPrefixes(), []string{".", "*"}) || !reflect.DeepEqual(replica.GetPrefixes(), []string{".", "*"}) || host.GetPrefix() != "." || replica.GetPrefix() != "." {
		t.Fatal("factory lost or shared configured prefix list")
	}
}

func TestCompatibilityFactoryRetainsPrefixesWhenConstructionFallsBack(t *testing.T) {
	// AGENT without a host relay takes the compatibility fallback path.
	cfg := &config.Config{CmdPrefix: "!", CmdPrefixes: []string{".", "*"}}
	engine := NewEngine(model.AGENT, cfg, &autoMoveCompositionRepository{})
	source, ok := engine.(interface{ GetPrefixes() []string })
	if !ok || !reflect.DeepEqual(source.GetPrefixes(), []string{".", "*"}) || engine.GetPrefix() != "." {
		t.Fatal("compatibility factory discarded the configured prefixes")
	}
}
