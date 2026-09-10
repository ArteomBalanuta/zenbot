package command

import (
	"context"
	"reflect"
	"testing"
	"zenbot/internal/common"
	"zenbot/internal/model"
)

func TestSaturnCatalogHas64ConcreteFactories(t *testing.T) {
	r := common.NewSaturnCommandRegistry()
	if err := RegisterAll(r); err != nil {
		t.Fatal(err)
	}
	defs := r.Definitions()
	if len(defs) != 64 {
		t.Fatalf("catalog size=%d, want 64", len(defs))
	}
	e := &commandEngineStub{users: map[string]*model.User{}}
	for _, d := range defs {
		c := d.New(e, &model.ChatMessage{Text: "!" + d.Canonical, Name: "alice"})
		if c == nil || reflect.ValueOf(c).IsNil() {
			t.Fatalf("nil handler for %s", d.Canonical)
		}
		if c.Aliases() == nil || c.Role() != d.Role {
			t.Fatalf("bad metadata for %s", d.Canonical)
		}
		_, _ = c.Execute(context.Background())
	}
}
