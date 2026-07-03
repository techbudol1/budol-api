package httpapi

import (
	"testing"
	"time"

	"budol/server/internal/store"
)

func TestValidateNewsCandidateUpdateNormalizesMultipleChoice(t *testing.T) {
	input := validCandidateUpdate()
	input.PollType = "multiple_choice_yes_no"
	input.OutcomeA = "Ignored"
	input.OutcomeB = "Ignored"
	input.ChoiceLabels = []string{"Alpha", " Beta ", "alpha", "Gamma"}

	normalized, err := validateNewsCandidateUpdate(input)
	if err != nil {
		t.Fatalf("validateNewsCandidateUpdate returned error: %v", err)
	}
	if len(normalized.ChoiceLabels) != 3 {
		t.Fatalf("choices = %v, want three distinct choices", normalized.ChoiceLabels)
	}
	if normalized.OutcomeA != "Yes" || normalized.OutcomeB != "No" {
		t.Fatalf("outcomes = %q/%q, want Yes/No", normalized.OutcomeA, normalized.OutcomeB)
	}
}

func TestValidateNewsCandidateUpdateRejectsIncompleteCandidate(t *testing.T) {
	input := validCandidateUpdate()
	input.ResolutionNotes = "too short"
	if _, err := validateNewsCandidateUpdate(input); err == nil {
		t.Fatal("expected incomplete resolution rules to be rejected")
	}
}

func validCandidateUpdate() store.NewsCandidateUpdateInput {
	return store.NewsCandidateUpdateInput{
		ProposedTitle:         "Will the named authority issue its decision?",
		PollType:              "yes_no",
		ClassificationID:      "policy",
		CallName:              "decision market",
		Region:                "National",
		OutcomeA:              "Yes",
		OutcomeB:              "No",
		ResolutionSource:      "Named authority",
		ResolutionEvidenceURL: "https://example.test/source",
		ResolutionNotes:       "Resolve Yes only if the named authority publishes the decision before the deadline; otherwise resolve No.",
		EndsAt:                time.Now().Add(30 * 24 * time.Hour).UTC().Format(time.RFC3339),
		Confidence:            70,
		Rationale:             "The supplied report identifies a future decision.",
		SafetyFlags:           []string{"Verify source"},
	}
}
