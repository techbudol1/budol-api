package httpapi

import "testing"

func TestHashPilotIdentifierIsStableAndOpaque(t *testing.T) {
	raw := "6af91b65-f9cb-4b97-b883-e5a4c70c8842"
	first, err := hashPilotIdentifier(raw)
	if err != nil {
		t.Fatalf("hash identifier: %v", err)
	}
	second, err := hashPilotIdentifier(raw)
	if err != nil {
		t.Fatalf("hash identifier again: %v", err)
	}
	if first != second {
		t.Fatal("expected stable identifier hash")
	}
	if first == raw || len(first) != 64 {
		t.Fatalf("expected opaque sha256 identifier, got %q", first)
	}
}

func TestHashPilotIdentifierRejectsInvalidValues(t *testing.T) {
	for _, value := range []string{"short", "contains spaces and is long", "<script>alert(1)</script>"} {
		if _, err := hashPilotIdentifier(value); err == nil {
			t.Fatalf("expected %q to be rejected", value)
		}
	}
}

func TestValidatedPilotFeedback(t *testing.T) {
	input, err := validatedPilotFeedback(pilotFeedbackRequest{
		AnonymousID: "6af91b65-f9cb-4b97-b883-e5a4c70c8842",
		Category:    "Usability",
		Area:        "Trading",
		DeviceClass: "Desktop",
		Rating:      4,
		Message:     "The private claim flow was easy to understand.",
	})
	if err != nil {
		t.Fatalf("validate feedback: %v", err)
	}
	if input.Category != "usability" || input.Area != "trading" || input.DeviceClass != "desktop" {
		t.Fatalf("unexpected normalized input: %#v", input)
	}
}
