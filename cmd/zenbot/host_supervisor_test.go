package main

import (
	"context"
	"reflect"
	"testing"
)

type supervisorMasterFake struct{ id int }

func TestHostSupervisorInitialStartBuildsStartsAndPublishes(t *testing.T) {
	var events []string
	s := NewHostSupervisor(nil, nil,
		func(context.Context) (any, error) {
			events = append(events, "build")
			return &supervisorMasterFake{id: 1}, nil
		},
		func(context.Context, any) error { events = append(events, "start"); return nil },
		func(any) { events = append(events, "rebind") },
	)
	if err := s.StartInitial(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s.Master() == nil || !reflect.DeepEqual(events, []string{"build", "start", "rebind"}) {
		t.Fatalf("master=%v events=%v", s.Master(), events)
	}
}

func TestHostSupervisorRestartBuildsFreshMasterAndRebinds(t *testing.T) {
	old := &supervisorMasterFake{id: 1}
	var events []string
	s := NewHostSupervisor(old,
		func(context.Context, any) error { events = append(events, "retire-old"); return nil },
		func(context.Context) (any, error) {
			events = append(events, "build-new")
			return &supervisorMasterFake{id: 2}, nil
		},
		func(context.Context, any) error { events = append(events, "start-new"); return nil },
		func(master any) {
			if master != nil {
				events = append(events, "rebind")
			}
		},
	)
	if err := s.Restart(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := s.Master(); got == old {
		t.Fatal("master was not replaced")
	}
	if want := []string{"retire-old", "build-new", "start-new", "rebind"}; !reflect.DeepEqual(events, want) {
		t.Fatalf("events=%v want=%v", events, want)
	}
}

func TestHostSupervisorRestartWithoutFreshBuilderFailsBeforeRetiringMaster(t *testing.T) {
	master := &supervisorMasterFake{id: 1}
	retired := false
	s := NewHostSupervisor(master, func(context.Context, any) error {
		retired = true
		return nil
	}, nil, nil, nil)

	if err := s.Restart(context.Background()); err == nil {
		t.Fatal("restart without a fresh-master builder succeeded")
	}
	if retired {
		t.Fatal("restart without a fresh-master builder retired the current master")
	}
	if got := s.Master(); got != master {
		t.Fatalf("master=%v, want original master", got)
	}
}

func TestMainProductionHostLifecycleWiresSupervisorCallbacks(t *testing.T) {
	old := &supervisorMasterFake{id: 1}
	var events []string
	supervisor := NewHostSupervisor(old,
		func(context.Context, any) error { events = append(events, "retire-old"); return nil },
		func(context.Context) (any, error) {
			events = append(events, "build-new")
			return &supervisorMasterFake{id: 2}, nil
		},
		func(context.Context, any) error { events = append(events, "start-new"); return nil },
		func(master any) {
			if master != nil {
				events = append(events, "rebind")
			}
		},
	)
	controller := newProductionHostLifecycle(context.Background(), supervisor, nil)
	defer controller.Close()

	releaseDispatch, err := controller.BeginDispatch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := controller.RequestRestart(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("lifecycle callback ran before dispatch release: %v", events)
	}
	releaseDispatch()
	controller.Wait()
	if want := []string{"retire-old", "build-new", "start-new", "rebind"}; !reflect.DeepEqual(events, want) {
		t.Fatalf("events=%v want=%v", events, want)
	}
}

func TestHostSupervisorShutdownStopsOnlyHostAndSignalTeardownIsIdempotent(t *testing.T) {
	master := &supervisorMasterFake{id: 1}
	var retire, teardown int
	s := NewHostSupervisor(master, func(context.Context, any) error { retire++; return nil }, nil, nil, nil)
	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s.Master() != nil || retire != 1 {
		t.Fatalf("master=%v retire=%d", s.Master(), retire)
	}
	s.SignalTeardown(func(context.Context) error { teardown++; return nil }, context.Background())
	s.SignalTeardown(func(context.Context) error { teardown++; return nil }, context.Background())
	if teardown != 1 {
		t.Fatalf("teardown=%d", teardown)
	}
}
