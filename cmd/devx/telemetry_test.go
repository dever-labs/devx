package main

import "testing"

func TestTelemetryFromStateDefaultsOff(t *testing.T) {
	defer chdirTemp(t, "")()
	if telemetryFromState() {
		t.Fatal("telemetry must be off when no state exists")
	}
	if err := writeState(state{Profile: "local", Runtime: "docker", Telemetry: true}); err != nil {
		t.Fatal(err)
	}
	if !telemetryFromState() {
		t.Fatal("telemetry should follow persisted state")
	}
}
