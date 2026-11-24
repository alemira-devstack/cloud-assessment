package app

import (
	"testing"
	"time"
)

func TestEnvDefaults(t *testing.T) {
	t.Setenv("TEST_VALUE", "")

	if got := EnvString("TEST_VALUE", "jobs"); got != "jobs" {
		t.Errorf("EnvString = %q, want %q", got, "jobs")
	}
	if got, err := EnvInt("TEST_VALUE", 1); err != nil || got != 1 {
		t.Errorf("EnvInt = %d, %v, want 1, nil", got, err)
	}
	if got, err := EnvFloat("TEST_VALUE", 0); err != nil || got != 0 {
		t.Errorf("EnvFloat = %v, %v, want 0, nil", got, err)
	}
	if got, err := EnvDuration("TEST_VALUE", 2*time.Second); err != nil || got != 2*time.Second {
		t.Errorf("EnvDuration = %s, %v, want 2s, nil", got, err)
	}
}

func TestEnvParsing(t *testing.T) {
	t.Setenv("TEST_INT", "4")
	t.Setenv("TEST_FLOAT", "2.5")
	t.Setenv("TEST_DURATION", "500ms")

	if got, err := EnvInt("TEST_INT", 1); err != nil || got != 4 {
		t.Errorf("EnvInt = %d, %v, want 4, nil", got, err)
	}
	if got, err := EnvFloat("TEST_FLOAT", 0); err != nil || got != 2.5 {
		t.Errorf("EnvFloat = %v, %v, want 2.5, nil", got, err)
	}
	if got, err := EnvDuration("TEST_DURATION", 0); err != nil || got != 500*time.Millisecond {
		t.Errorf("EnvDuration = %s, %v, want 500ms, nil", got, err)
	}
}

func TestEnvInvalid(t *testing.T) {
	t.Setenv("TEST_BAD", "two")

	if _, err := EnvInt("TEST_BAD", 1); err == nil {
		t.Error("EnvInt: want an error")
	}
	if _, err := EnvFloat("TEST_BAD", 0); err == nil {
		t.Error("EnvFloat: want an error")
	}
	if _, err := EnvDuration("TEST_BAD", 0); err == nil {
		t.Error("EnvDuration: want an error")
	}

	t.Setenv("TEST_NO_UNIT", "2")
	if _, err := EnvDuration("TEST_NO_UNIT", 0); err == nil {
		t.Error("EnvDuration: want an error for a number without a unit")
	}
}
