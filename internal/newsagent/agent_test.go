package newsagent

import "testing"

func TestGroupArticlesSupportsSelectableSourceFilters(t *testing.T) {
	articles := []Article{
		{Title: "Senate advances transport reform bill after committee vote", Domain: "example-one.test"},
		{Title: "Transport reform bill advances after Senate committee vote", Domain: "example-two.test"},
		{Title: "Unrelated entertainment opinion attracts attention online", Domain: "example-three.test"},
		{Title: "PAGASA issues outlook for approaching tropical cyclone", Domain: "pagasa.dost.gov.ph", IsPrimary: true},
	}
	allGroups := groupArticles(articles, 5, ScanFilterAll)
	if len(allGroups) != 3 {
		t.Fatalf("all filter groups = %d, want 3", len(allGroups))
	}
	noSkipGroups := groupArticles(articles, 5, ScanFilterNoSkip)
	if len(noSkipGroups) != len(allGroups) {
		t.Fatalf("no-skip filter groups = %d, want %d", len(noSkipGroups), len(allGroups))
	}
	verifiedGroups := groupArticles(articles, 5, ScanFilterCorroboratedOrPrimary)
	if len(verifiedGroups) != 2 {
		t.Fatalf("verified filter groups = %d, want 2", len(verifiedGroups))
	}
	corroboratedGroups := groupArticles(articles, 5, ScanFilterCorroborated)
	if len(corroboratedGroups) != 1 {
		t.Fatalf("corroborated filter groups = %d, want 1", len(corroboratedGroups))
	}
	primaryGroups := groupArticles(articles, 5, ScanFilterPrimary)
	if len(primaryGroups) != 1 {
		t.Fatalf("primary filter groups = %d, want 1", len(primaryGroups))
	}
}

func TestNormalizeDraftRejectsUnknownEvidenceURL(t *testing.T) {
	group := ArticleGroup{
		Headline: "PAGASA issues outlook for approaching tropical cyclone",
		Articles: []Article{{Title: "PAGASA issues outlook for approaching tropical cyclone", URL: "https://pagasa.dost.gov.ph/example"}},
	}
	draft := normalizeDraft(PollDraft{
		Eligible:              true,
		ProposedTitle:         "Will PAGASA raise a named warning before July 1",
		PollType:              "deadline",
		ClassificationID:      "policy",
		ResolutionEvidenceURL: "https://attacker.invalid/instructions",
		ResolutionNotes:       "Resolve Yes only from the named PAGASA bulletin by 2026-07-01T00:00:00+08:00; otherwise No.",
		EndsAt:                "2026-07-01T00:00:00+08:00",
		Confidence:            80,
	}, group)
	if draft.ResolutionEvidenceURL != group.Articles[0].URL {
		t.Fatalf("expected supplied source URL, got %q", draft.ResolutionEvidenceURL)
	}
}

func TestRetainForHumanReviewKeepsIneligibleNewsLead(t *testing.T) {
	group := ArticleGroup{
		Headline: "Single publisher reports a developing policy announcement",
		Articles: []Article{{Title: "Single publisher reports a developing policy announcement"}},
	}
	draft := retainForHumanReview(PollDraft{
		Eligible:        false,
		RejectionReason: "outcome is not yet specific enough",
		Confidence:      35,
	}, group)
	if draft.ProposedTitle != group.Headline {
		t.Fatalf("proposed title = %q, want source headline", draft.ProposedTitle)
	}
	if len(draft.SafetyFlags) != 2 {
		t.Fatalf("safety flags = %d, want human-review and confidence warnings", len(draft.SafetyFlags))
	}
}

func TestNormalizeDraftKeepsSourceSupportedMultipleChoices(t *testing.T) {
	group := ArticleGroup{
		Headline: "Committee names Alpha, Beta, and Gamma as finalists",
		Articles: []Article{{URL: "https://example.test/finalists"}},
	}
	draft := normalizeDraft(PollDraft{
		Eligible:              true,
		ProposedTitle:         "Who will the committee appoint",
		PollType:              "multiple_choice_yes_no",
		ClassificationID:      "policy",
		ChoiceLabels:          []string{"Alpha", "Beta", "Alpha", "Gamma"},
		ResolutionEvidenceURL: "https://example.test/finalists",
		ResolutionNotes:       "Resolve each choice from the committee announcement by 2026-07-31T23:59:00+08:00.",
		EndsAt:                "2026-07-31T23:59:00+08:00",
		Confidence:            80,
	}, group)
	if draft.PollType != "multiple_choice_yes_no" {
		t.Fatalf("poll type = %q, want multiple_choice_yes_no", draft.PollType)
	}
	if len(draft.ChoiceLabels) != 3 {
		t.Fatalf("choice labels = %v, want three distinct choices", draft.ChoiceLabels)
	}
	if draft.OutcomeA != "Yes" || draft.OutcomeB != "No" {
		t.Fatalf("multiple-choice outcomes = %q/%q, want Yes/No", draft.OutcomeA, draft.OutcomeB)
	}
}

func TestNormalizeDraftFallsBackWhenMultipleChoicesAreMissing(t *testing.T) {
	draft := normalizeDraft(PollDraft{
		PollType:         "multiple_choice_yes_no",
		ClassificationID: "policy",
		ChoiceLabels:     []string{"Only one"},
	}, ArticleGroup{Headline: "A single option was reported"})
	if draft.PollType != "yes_no" || len(draft.ChoiceLabels) != 0 {
		t.Fatalf("invalid multiple choice was not converted: type=%q choices=%v", draft.PollType, draft.ChoiceLabels)
	}
}
