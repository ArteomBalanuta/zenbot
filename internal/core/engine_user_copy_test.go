package core

import (
	"reflect"
	"sync"
	"testing"

	"zenbot/internal/model"
)

var stateFlairFixtures = []struct {
	name string
	json string
}{
	{"object", `{"nick":"alice","trip":"trip","flair":{"label":"original","nested":[{"value":"original"},["original"]]}}`},
	{"array", `{"nick":"alice","trip":"trip","flair":[{"value":"original"},["original"]]}`},
}

func TestStateOwnsNestedFlairInputs(t *testing.T) {
	for _, boundary := range []struct {
		name string
		add  func(*EngineImpl, *model.User)
		read func(*EngineImpl) *model.User
	}{
		{"AddActiveUser", (*EngineImpl).AddActiveUser, activeFlairUser},
		{"ReplaceActiveUsers", func(e *EngineImpl, u *model.User) { e.ReplaceActiveUsers([]*model.User{u}) }, activeFlairUser},
		{"AddAfkUser", func(e *EngineImpl, u *model.User) { e.AddAfkUser(u, "away") }, afkFlairUser},
	} {
		for _, fixture := range stateFlairFixtures {
			t.Run(boundary.name+"/"+fixture.name, func(t *testing.T) {
				input := parseFlairUser(t, fixture.json)
				want := parseFlairUser(t, fixture.json)
				engine := &EngineImpl{}
				boundary.add(engine, input)
				mutateNestedFlair(input.Flair)
				if got := boundary.read(engine); !reflect.DeepEqual(got, want) {
					t.Fatalf("mutating input flair changed owned user: got %#v, want %#v", got, want)
				}
			})
		}
	}
}

func TestStateReturnsDetachedNestedFlair(t *testing.T) {
	for _, boundary := range []struct {
		name string
		read func(*EngineImpl) *model.User
	}{
		{"GetActiveUserByName", activeFlairUser},
		{"GetActiveUsers", func(e *EngineImpl) *model.User {
			for user := range *e.GetActiveUsers() {
				return user
			}
			return nil
		}},
		{"GetAfkUsers", afkFlairUser},
	} {
		for _, fixture := range stateFlairFixtures {
			t.Run(boundary.name+"/"+fixture.name, func(t *testing.T) {
				owned := parseFlairUser(t, fixture.json)
				want := parseFlairUser(t, fixture.json)
				engine := &EngineImpl{
					ActiveUsers: map[*model.User]struct{}{owned: {}},
					AfkUsers:    map[*model.User]string{owned: "away"},
				}
				mutateNestedFlair(boundary.read(engine).Flair)
				if got := boundary.read(engine); !reflect.DeepEqual(got, want) {
					t.Fatalf("mutating snapshot flair changed owned user: got %#v, want %#v", got, want)
				}
			})
		}
	}
}

func TestStatePreservesScalarFlair(t *testing.T) {
	for _, flair := range []interface{}{nil, false, true, "", "badge", float64(0), float64(1.5)} {
		engine := &EngineImpl{}
		user := &model.User{Name: "alice", Trip: "trip", Flair: flair}
		engine.AddActiveUser(user)
		engine.AddAfkUser(user, "away")
		for _, got := range []*model.User{activeFlairUser(engine), afkFlairUser(engine)} {
			if !reflect.DeepEqual(got.Flair, flair) {
				t.Fatalf("flair = %#v, want %#v", got.Flair, flair)
			}
		}
	}
}

func TestStateConcurrentDetachedFlair(t *testing.T) {
	engine := &EngineImpl{}
	input := parseFlairUser(t, stateFlairFixtures[0].json)
	want := parseFlairUser(t, stateFlairFixtures[0].json)
	engine.AddActiveUser(input)
	engine.AddAfkUser(input, "away")
	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		for i := 0; i < 100; i++ {
			engine.ReplaceActiveUsers([]*model.User{input})
			engine.AddAfkUser(input, "away")
		}
	}()
	go func() {
		defer workers.Done()
		for i := 0; i < 100; i++ {
			mutateNestedFlair(activeFlairUser(engine).Flair)
			mutateNestedFlair(afkFlairUser(engine).Flair)
		}
	}()
	workers.Wait()
	if !reflect.DeepEqual(activeFlairUser(engine), want) || !reflect.DeepEqual(afkFlairUser(engine), want) {
		t.Fatal("concurrent snapshot mutation changed owned flair")
	}
}

func activeFlairUser(e *EngineImpl) *model.User { return e.GetActiveUserByName("alice") }

func afkFlairUser(e *EngineImpl) *model.User {
	for user := range *e.GetAfkUsers() {
		return user
	}
	return nil
}

func parseFlairUser(t *testing.T, payload string) *model.User {
	t.Helper()
	user, err := model.GetUser(payload)
	if err != nil {
		t.Fatal(err)
	}
	return user
}

func mutateNestedFlair(flair interface{}) {
	switch value := flair.(type) {
	case map[string]interface{}:
		for key, nested := range value {
			if _, scalar := nested.(string); scalar {
				value[key] = "changed"
			} else {
				mutateNestedFlair(nested)
			}
		}
	case []interface{}:
		for i, nested := range value {
			if _, scalar := nested.(string); scalar {
				value[i] = "changed"
			} else {
				mutateNestedFlair(nested)
			}
		}
	}
}
