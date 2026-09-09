package tool

import (
	"testing"

	"zenbot/internal/agent/api"
)

func TestRegistryDefinitionsHideToolsWithoutRequiredCapability(t *testing.T) {
	registry := NewRegistry([]Tool{DatabaseSchema{}}, []string{databaseSchemaName})
	anonymous, err := api.NewContext("room", "nick", "", "", false, []string{})
	if err != nil {
		t.Fatal(err)
	}
	if got := registry.Definitions(anonymous); len(got) != 0 {
		t.Fatalf("anonymous definitions=%#v", got)
	}
	authorized, err := api.NewContextWithCapabilities("room", "nick", "", "", false, []string{}, []api.Capability{api.DynamicSQL}, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := registry.Definitions(authorized); len(got) != 1 || got[0].Name != databaseSchemaName {
		t.Fatalf("authorized definitions=%#v", got)
	}
}
