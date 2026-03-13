package managedsections

import (
	"strings"
	"testing"
)

func TestUpsertIsIdempotent(t *testing.T) {
	content := "# Agents\n"

	first, err := Upsert(content, "@acme/browser-skill", "- browser skill\n")
	if err != nil {
		t.Fatalf("Upsert returned error: %v", err)
	}

	second, err := Upsert(first, "@acme/browser-skill", "- browser skill\n")
	if err != nil {
		t.Fatalf("second Upsert returned error: %v", err)
	}

	if first != second {
		t.Fatalf("expected idempotent content, got:\n%s\n---\n%s", first, second)
	}
}

func TestRemoveDeletesManagedBlockOnly(t *testing.T) {
	content := "# Agents\n\n<!-- ocpm:begin @acme/browser-skill -->\n- browser skill\n<!-- ocpm:end @acme/browser-skill -->\n\nUser content.\n"

	updated, removed, err := Remove(content, "@acme/browser-skill")
	if err != nil {
		t.Fatalf("Remove returned error: %v", err)
	}
	if !removed {
		t.Fatalf("expected managed block to be removed")
	}
	if strings.Contains(updated, "browser skill") {
		t.Fatalf("expected managed content to be removed, got %q", updated)
	}
	if !strings.Contains(updated, "User content.") {
		t.Fatalf("expected user content to remain, got %q", updated)
	}
}

func TestValidateRejectsNestedMarkers(t *testing.T) {
	content := "<!-- ocpm:begin one -->\n<!-- ocpm:begin two -->\n<!-- ocpm:end two -->\n<!-- ocpm:end one -->\n"
	if err := Validate(content); err == nil {
		t.Fatalf("expected nested marker validation error")
	}
}
