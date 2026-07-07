package httpapi

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/techbudol1/budol-api/internal/newsagent"
	"github.com/techbudol1/budol-api/internal/store"

	"github.com/gofiber/fiber/v2"
)

func (s Server) adminNewsAgentStatus(c *fiber.Ctx) error {
	if s.newsAgent == nil {
		return c.JSON(fiber.Map{
			"agent": fiber.Map{
				"configured":    false,
				"enabled":       false,
				"framework":     "CloudWeGo Eino",
				"autoPublishes": false,
			},
		})
	}
	return c.JSON(fiber.Map{"agent": s.newsAgent.Status()})
}

func (s Server) adminRunNewsAgent(c *fiber.Ctx) error {
	if s.newsAgent == nil || !s.newsAgent.Configured() {
		return fiber.NewError(fiber.StatusServiceUnavailable, "news agent requires OPENAI_API_KEY and NEWS_AGENT_ENABLED=true")
	}
	var request struct {
		Filter string `json:"filter"`
	}
	if len(c.Body()) > 0 {
		if err := c.BodyParser(&request); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
		}
	}
	if !newsagent.ValidScanFilter(request.Filter) {
		return fiber.NewError(fiber.StatusBadRequest, "unsupported news scan filter")
	}
	result, err := s.newsAgent.ScanWithFilter(c.Context(), request.Filter)
	if err != nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, err.Error())
	}
	_, _ = s.store.CreateAdminActivity(c.Context(), store.AdminActivityInput{
		Actor:      s.adminActor(c),
		Action:     "run_news_agent",
		TargetType: "news_agent",
		Title:      "News Scout scan completed",
		Detail:     result.String(),
		IPAddress:  c.IP(),
		UserAgent:  c.Get("User-Agent"),
	})
	return c.JSON(fiber.Map{"result": result})
}

func (s Server) adminNewsCandidates(c *fiber.Ctx) error {
	candidates, err := s.store.ListNewsCandidates(c.Context(), c.Query("status", "all"), int64(c.QueryInt("limit", 100)))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load news candidates")
	}
	return c.JSON(fiber.Map{"candidates": candidates})
}

func (s Server) adminUpdateNewsCandidate(c *fiber.Ctx) error {
	var input store.NewsCandidateUpdateInput
	if err := c.BodyParser(&input); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	input, err := validateNewsCandidateUpdate(input)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	candidate, err := s.store.UpdatePendingNewsCandidate(c.Context(), c.Params("id"), input)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	_, _ = s.store.CreateAdminActivity(c.Context(), store.AdminActivityInput{
		Actor:      s.adminActor(c),
		Action:     "update_news_candidate",
		TargetType: "news_candidate",
		TargetID:   candidate.ID,
		Title:      "News candidate edited",
		Detail:     candidate.ProposedTitle,
		IPAddress:  c.IP(),
		UserAgent:  c.Get("User-Agent"),
	})
	return c.JSON(fiber.Map{"candidate": candidate})
}

func (s Server) adminApproveNewsCandidate(c *fiber.Ctx) error {
	var request NewsCandidateReviewRequest
	if len(c.Body()) > 0 {
		if err := c.BodyParser(&request); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
		}
	}
	candidate, ok, err := s.store.GetNewsCandidate(c.Context(), c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load news candidate")
	}
	if !ok {
		return fiber.NewError(fiber.StatusNotFound, "news candidate not found")
	}
	if candidate.Status != "pending_review" {
		return fiber.NewError(fiber.StatusConflict, "news candidate has already been reviewed")
	}
	if _, err := validateNewsCandidateUpdate(newsCandidateUpdateInput(candidate)); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "edit candidate before approval: "+err.Error())
	}
	pollInput := store.PollInput{
		Slug:                  "news-" + candidate.ID + "-" + firstNonEmpty(candidate.ProposedTitle, candidate.Headline),
		Title:                 firstNonEmpty(candidate.ProposedTitle, candidate.Headline),
		PollType:              candidate.PollType,
		ClassificationID:      candidate.ClassificationID,
		CallName:              candidate.CallName,
		Region:                candidate.Region,
		Status:                "draft",
		Visibility:            "internal",
		OutcomeA:              candidate.OutcomeA,
		OutcomeB:              candidate.OutcomeB,
		ChoiceLabels:          candidate.ChoiceLabels,
		YesPercent:            50,
		Volume:                "P0",
		Change:                "+0.0%",
		Color:                 "blue",
		SortOrder:             100,
		Liquidity:             5000,
		AuditReason:           "Created from human-approved News Scout candidate " + candidate.ID + "; publication still requires a separate admin action.",
		ResolutionSource:      candidate.ResolutionSource,
		ResolutionEvidenceURL: candidate.ResolutionEvidenceURL,
		ResolutionNotes:       candidate.ResolutionNotes,
		StartsAt:              candidate.StartsAt,
		EndsAt:                candidate.EndsAt,
	}
	var polls []store.Poll
	if candidate.PollType == "multiple_choice_yes_no" {
		polls, err = s.createMultiChoicePolls(c, pollInput)
	} else {
		var poll store.Poll
		poll, err = s.store.UpsertPoll(c.Context(), pollInput)
		polls = []store.Poll{poll}
	}
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	poll := polls[0]
	reviewed, err := s.store.ReviewNewsCandidate(c.Context(), candidate.ID, "approved", s.adminActor(c), request.Notes, poll.ID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "poll draft was created but candidate review could not be recorded")
	}
	_, _ = s.store.CreateAdminActivity(c.Context(), store.AdminActivityInput{
		Actor:      s.adminActor(c),
		Action:     "approve_news_candidate",
		TargetType: "news_candidate",
		TargetID:   candidate.ID,
		Title:      "News candidate approved to internal draft",
		Detail:     fmt.Sprintf("%d internal draft market(s) created; all remain unpublished.", len(polls)),
		IPAddress:  c.IP(),
		UserAgent:  c.Get("User-Agent"),
	})
	return c.JSON(fiber.Map{"candidate": reviewed, "poll": poll, "polls": polls})
}

func validateNewsCandidateUpdate(input store.NewsCandidateUpdateInput) (store.NewsCandidateUpdateInput, error) {
	input.ProposedTitle = strings.TrimSpace(input.ProposedTitle)
	if len(input.ProposedTitle) < 12 {
		return input, fmt.Errorf("poll question must be at least 12 characters")
	}
	if !strings.HasSuffix(input.ProposedTitle, "?") {
		input.ProposedTitle += "?"
	}
	input.PollType = strings.ToLower(strings.TrimSpace(input.PollType))
	if !validNewsPollType(input.PollType) {
		return input, fmt.Errorf("unsupported poll type")
	}
	input.ClassificationID = strings.ToLower(strings.TrimSpace(input.ClassificationID))
	switch input.ClassificationID {
	case "elections", "congress", "lgu", "policy":
	default:
		return input, fmt.Errorf("unsupported classification")
	}
	input.CallName = strings.TrimSpace(input.CallName)
	input.Region = strings.TrimSpace(input.Region)
	if input.CallName == "" || input.Region == "" {
		return input, fmt.Errorf("call name and region are required")
	}
	input.ChoiceLabels = normalizedChoiceLabels(input.ChoiceLabels)
	if input.PollType == "multiple_choice_yes_no" {
		if len(input.ChoiceLabels) < 2 || len(input.ChoiceLabels) > 8 {
			return input, fmt.Errorf("multiple-choice polls require 2 to 8 distinct choices")
		}
		input.OutcomeA = "Yes"
		input.OutcomeB = "No"
	} else {
		input.ChoiceLabels = nil
		input.OutcomeA = strings.TrimSpace(input.OutcomeA)
		input.OutcomeB = strings.TrimSpace(input.OutcomeB)
		if input.OutcomeA == "" || input.OutcomeB == "" || strings.EqualFold(input.OutcomeA, input.OutcomeB) {
			return input, fmt.Errorf("two distinct outcomes are required")
		}
	}
	input.ResolutionSource = strings.TrimSpace(input.ResolutionSource)
	input.ResolutionNotes = strings.TrimSpace(input.ResolutionNotes)
	if input.ResolutionSource == "" || len(input.ResolutionNotes) < 20 {
		return input, fmt.Errorf("resolution source and detailed resolution rules are required")
	}
	input.ResolutionEvidenceURL = strings.TrimSpace(input.ResolutionEvidenceURL)
	if input.ResolutionEvidenceURL != "" {
		evidenceURL, err := url.Parse(input.ResolutionEvidenceURL)
		if err != nil || evidenceURL.Scheme != "https" || evidenceURL.Host == "" {
			return input, fmt.Errorf("resolution evidence URL must be a valid HTTPS URL")
		}
	}
	input.StartsAt = strings.TrimSpace(input.StartsAt)
	if input.StartsAt != "" {
		if _, err := time.Parse(time.RFC3339, input.StartsAt); err != nil {
			return input, fmt.Errorf("start time must use RFC3339 format")
		}
	}
	input.EndsAt = strings.TrimSpace(input.EndsAt)
	deadline, err := time.Parse(time.RFC3339, input.EndsAt)
	if err != nil {
		return input, fmt.Errorf("trading deadline must use RFC3339 format")
	}
	now := time.Now().UTC()
	if !deadline.After(now) || deadline.After(now.AddDate(1, 0, 0)) {
		return input, fmt.Errorf("trading deadline must be in the future and no more than one year away")
	}
	if input.Confidence < 0 || input.Confidence > 100 {
		return input, fmt.Errorf("confidence must be between 0 and 100")
	}
	input.Rationale = strings.TrimSpace(input.Rationale)
	if len(input.Rationale) < 10 {
		return input, fmt.Errorf("a short candidate summary is required")
	}
	input.SafetyFlags = normalizedTextList(input.SafetyFlags, 12)
	return input, nil
}

func newsCandidateUpdateInput(candidate store.NewsCandidate) store.NewsCandidateUpdateInput {
	return store.NewsCandidateUpdateInput{
		ProposedTitle:         candidate.ProposedTitle,
		PollType:              candidate.PollType,
		ClassificationID:      candidate.ClassificationID,
		CallName:              candidate.CallName,
		Region:                candidate.Region,
		OutcomeA:              candidate.OutcomeA,
		OutcomeB:              candidate.OutcomeB,
		ChoiceLabels:          candidate.ChoiceLabels,
		ResolutionSource:      candidate.ResolutionSource,
		ResolutionEvidenceURL: candidate.ResolutionEvidenceURL,
		ResolutionNotes:       candidate.ResolutionNotes,
		StartsAt:              candidate.StartsAt,
		EndsAt:                candidate.EndsAt,
		Confidence:            candidate.Confidence,
		Rationale:             candidate.Rationale,
		SafetyFlags:           candidate.SafetyFlags,
	}
}

func validNewsPollType(value string) bool {
	switch value {
	case "yes_no", "head_to_head", "polling_leader", "threshold", "deadline", "bill_passage", "multiple_choice_yes_no", "approval_rating", "turnout", "court_decision", "appointment", "budget_release":
		return true
	default:
		return false
	}
}

func normalizedTextList(values []string, limit int) []string {
	result := make([]string, 0, min(len(values), limit))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		key := strings.ToLower(value)
		if value == "" || seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, value)
		if len(result) == limit {
			break
		}
	}
	return result
}

func (s Server) adminRejectNewsCandidate(c *fiber.Ctx) error {
	var request NewsCandidateReviewRequest
	if len(c.Body()) > 0 {
		if err := c.BodyParser(&request); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
		}
	}
	candidate, err := s.store.ReviewNewsCandidate(c.Context(), c.Params("id"), "rejected", s.adminActor(c), strings.TrimSpace(request.Notes), "")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	_, _ = s.store.CreateAdminActivity(c.Context(), store.AdminActivityInput{
		Actor:      s.adminActor(c),
		Action:     "reject_news_candidate",
		TargetType: "news_candidate",
		TargetID:   candidate.ID,
		Title:      "News candidate rejected",
		Detail:     firstNonEmpty(candidate.ReviewNotes, candidate.ProposedTitle),
		IPAddress:  c.IP(),
		UserAgent:  c.Get("User-Agent"),
	})
	return c.JSON(fiber.Map{"candidate": candidate})
}
