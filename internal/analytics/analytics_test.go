package analytics

import (
	"os"
	"testing"
)

func TestDefaultAPIKey(t *testing.T) {
	t.Parallel()
	key := defaultAPIKey()
	if key == "" {
		t.Fatal("defaultAPIKey() returned empty string")
	}
	if len(key) < 10 {
		t.Errorf("defaultAPIKey() returned suspiciously short key: %q", key)
	}
}

func TestDistinctID(t *testing.T) {
	t.Parallel()
	id1 := distinctID()
	id2 := distinctID()
	if id1 == "" {
		t.Fatal("distinctID() returned empty string")
	}
	if id1 != id2 {
		t.Errorf("distinctID() not consistent: %q vs %q", id1, id2)
	}
	// SHA-256 truncated to 8 bytes = 16 hex chars
	if len(id1) != 16 {
		t.Errorf("distinctID() length = %d, want 16 hex chars", len(id1))
	}
}

func TestInitOptOut(t *testing.T) {
	// Not parallel: modifies package-level state and env vars
	// Save and restore state
	origClient := client
	origDisabled := disabled
	t.Cleanup(func() {
		client = origClient
		disabled = origDisabled
		os.Unsetenv(envOptOut)
	})

	client = nil
	disabled = false

	os.Setenv(envOptOut, "1")
	Init()

	if !disabled {
		t.Error("expected disabled=true after Init with opt-out env set")
	}
	if client != nil {
		t.Error("expected client to remain nil when opted out")
	}
}

func TestInitOptOutTrue(t *testing.T) {
	// Not parallel: modifies package-level state and env vars
	origClient := client
	origDisabled := disabled
	t.Cleanup(func() {
		client = origClient
		disabled = origDisabled
		os.Unsetenv(envOptOut)
	})

	client = nil
	disabled = false

	os.Setenv(envOptOut, "true")
	Init()

	if !disabled {
		t.Error("expected disabled=true after Init with opt-out=true")
	}
}

func TestTrackWhenDisabled(t *testing.T) {
	// Not parallel: modifies package-level state
	origClient := client
	origDisabled := disabled
	t.Cleanup(func() {
		client = origClient
		disabled = origDisabled
	})

	client = nil
	disabled = true

	// Should not panic
	Track("test_event", map[string]interface{}{"key": "value"})
	Track("test_event", nil)
}

func TestCloseWhenNil(t *testing.T) {
	// Not parallel: modifies package-level state
	origClient := client
	origDisabled := disabled
	t.Cleanup(func() {
		client = origClient
		disabled = origDisabled
	})

	client = nil
	disabled = false

	// Should not panic
	Close()
}
