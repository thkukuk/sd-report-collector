package dashboard

import "testing"

func TestParseDescribeAgainstFixture(t *testing.T) {
	entries, err := ParseDescribe("../../test-data/report.describe")
	if err != nil {
		t.Fatalf("ParseDescribe: %v", err)
	}

	got, ok := entries["io.systemd.Manager.SystemState"]
	if !ok {
		t.Fatalf("expected io.systemd.Manager.SystemState to be present")
	}
	if got.Type != "string" {
		t.Errorf("Type = %q, want %q", got.Type, "string")
	}
	if got.Description != "Overall system state" {
		t.Errorf("Description = %q, want %q", got.Description, "Overall system state")
	}
}

func TestParseDescribeMissingFile(t *testing.T) {
	if _, err := ParseDescribe("/does/not/exist"); err == nil {
		t.Fatalf("expected an error for a missing describe file")
	}
}
