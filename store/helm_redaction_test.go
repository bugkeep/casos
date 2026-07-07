package store

import (
	"errors"
	"strings"
	"testing"
)

func TestRedactURLForErrorRedactsCredentials(t *testing.T) {
	raw := "https://user:pass@example.com/charts/app"

	got := redactURLForError(raw)

	if strings.Contains(got, "user:pass") {
		t.Fatalf("expected credentials to be redacted, got %q", got)
	}
	if got != "https://REDACTED@example.com/charts/app" {
		t.Fatalf("unexpected redacted URL %q", got)
	}
}

func TestRedactURLForErrorRedactsSchemeLessOCIRef(t *testing.T) {
	raw := "user:pass@registry.example.com/myrepo/chart"

	got := redactURLForError(raw)

	if strings.Contains(got, "user:pass") {
		t.Fatalf("expected scheme-less credentials to be redacted, got %q", got)
	}
	if got != "REDACTED@registry.example.com/myrepo/chart" {
		t.Fatalf("unexpected redacted OCI ref %q", got)
	}
}

func TestRedactURLForErrorDoesNotCorruptDigestReference(t *testing.T) {
	raw := "oci://registry.example.com:5000/myrepo@sha256:abc123"

	got := redactURLForError(raw)

	if got != raw {
		t.Fatalf("expected digest reference to remain unchanged, got %q", got)
	}
}

func TestSanitizeErrorMessageRedactsNestedVariantsAndPreservesCause(t *testing.T) {
	original := errors.New(`pull failed for "https://user:pass@example.com/repo/index.yaml" and "user:pass@example.com/repo:1.2.3"`)

	sanitized := sanitizeErrorMessage(
		original,
		"https://user:pass@example.com/repo",
		"https://user:pass@example.com/repo/index.yaml",
		"user:pass@example.com/repo:1.2.3",
	)

	if errors.Is(sanitized, original) == false {
		t.Fatalf("expected sanitized error to unwrap to original error")
	}
	if strings.Contains(sanitized.Error(), "user:pass") {
		t.Fatalf("expected sanitized message to remove credentials, got %q", sanitized.Error())
	}
	if !strings.Contains(sanitized.Error(), "https://REDACTED@example.com/repo/index.yaml") {
		t.Fatalf("expected sanitized message to keep redacted index URL, got %q", sanitized.Error())
	}
	if !strings.Contains(sanitized.Error(), "REDACTED@example.com/repo:1.2.3") {
		t.Fatalf("expected sanitized message to keep redacted OCI ref, got %q", sanitized.Error())
	}
}
