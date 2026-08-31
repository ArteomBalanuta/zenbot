package h2

import (
	"context"
	"reflect"
	"testing"
)

func TestConfigExposesAutoPortOptIn(t *testing.T) {
	field, ok := reflect.TypeOf(Config{}).FieldByName("AutoPort")
	if !ok {
		t.Fatal("Config must expose an explicit AutoPort opt-in")
	}
	if field.Type.Kind() != reflect.Bool {
		t.Fatalf("Config.AutoPort type = %s, want bool", field.Type)
	}
}

func TestDefaultPortZeroRemainsLegacy5435(t *testing.T) {
	s := &processServer{cfg: Config{Java: "h2-test-java-not-installed", H2Jar: "unused"}}
	if err := s.Start(context.Background()); err == nil {
		t.Fatal("Start unexpectedly succeeded with nonexistent Java runtime")
	}
	if s.cfg.Port != 5435 {
		t.Fatalf("legacy zero port = %d, want 5435", s.cfg.Port)
	}
}
