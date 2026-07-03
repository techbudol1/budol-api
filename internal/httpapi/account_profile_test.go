package httpapi

import "testing"

func TestNormalizeDisplayName(t *testing.T) {
	got, err := normalizeDisplayName("  Juan   Dela-Cruz  ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "Juan Dela-Cruz" {
		t.Fatalf("unexpected display name: %q", got)
	}
}

func TestNormalizeDisplayNameAllowsUnicodeNames(t *testing.T) {
	got, err := normalizeDisplayName("José Rizal")
	if err != nil {
		t.Fatal(err)
	}
	if got != "José Rizal" {
		t.Fatalf("unexpected display name: %q", got)
	}
}

func TestNormalizeDisplayNameRejectsReservedAndInvalidNames(t *testing.T) {
	for _, value := range []string{"BudolPH Admin", "Budol Admin", "@trader", "ab"} {
		if _, err := normalizeDisplayName(value); err == nil {
			t.Fatalf("expected %q to be rejected", value)
		}
	}
}
