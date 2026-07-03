package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

type NewsSource struct {
	Title         string `json:"title"`
	URL           string `json:"url"`
	Domain        string `json:"domain"`
	PublishedAt   string `json:"publishedAt"`
	SourceCountry string `json:"sourceCountry"`
	IsPrimary     bool   `json:"isPrimary"`
}

type NewsCandidate struct {
	ID                    string       `json:"id"`
	Fingerprint           string       `json:"fingerprint"`
	Status                string       `json:"status"`
	Headline              string       `json:"headline"`
	ProposedTitle         string       `json:"proposedTitle"`
	PollType              string       `json:"pollType"`
	ClassificationID      string       `json:"classificationId"`
	CallName              string       `json:"callName"`
	Region                string       `json:"region"`
	OutcomeA              string       `json:"outcomeA"`
	OutcomeB              string       `json:"outcomeB"`
	ChoiceLabels          []string     `json:"choiceLabels"`
	ResolutionSource      string       `json:"resolutionSource"`
	ResolutionEvidenceURL string       `json:"resolutionEvidenceUrl"`
	ResolutionNotes       string       `json:"resolutionNotes"`
	StartsAt              string       `json:"startsAt"`
	EndsAt                string       `json:"endsAt"`
	Confidence            int64        `json:"confidence"`
	Rationale             string       `json:"rationale"`
	SafetyFlags           []string     `json:"safetyFlags"`
	Sources               []NewsSource `json:"sources"`
	Model                 string       `json:"model"`
	PollID                string       `json:"pollId"`
	ReviewNotes           string       `json:"reviewNotes"`
	ReviewedAt            string       `json:"reviewedAt"`
	ReviewedBy            string       `json:"reviewedBy"`
	CreatedAt             string       `json:"createdAt"`
	UpdatedAt             string       `json:"updatedAt"`
}

type NewsCandidateInput struct {
	Fingerprint           string
	Headline              string
	ProposedTitle         string
	PollType              string
	ClassificationID      string
	CallName              string
	Region                string
	OutcomeA              string
	OutcomeB              string
	ChoiceLabels          []string
	ResolutionSource      string
	ResolutionEvidenceURL string
	ResolutionNotes       string
	StartsAt              string
	EndsAt                string
	Confidence            int64
	Rationale             string
	SafetyFlags           []string
	Sources               []NewsSource
	Model                 string
}

type NewsCandidateUpdateInput struct {
	ProposedTitle         string   `json:"proposedTitle"`
	PollType              string   `json:"pollType"`
	ClassificationID      string   `json:"classificationId"`
	CallName              string   `json:"callName"`
	Region                string   `json:"region"`
	OutcomeA              string   `json:"outcomeA"`
	OutcomeB              string   `json:"outcomeB"`
	ChoiceLabels          []string `json:"choiceLabels"`
	ResolutionSource      string   `json:"resolutionSource"`
	ResolutionEvidenceURL string   `json:"resolutionEvidenceUrl"`
	ResolutionNotes       string   `json:"resolutionNotes"`
	StartsAt              string   `json:"startsAt"`
	EndsAt                string   `json:"endsAt"`
	Confidence            int64    `json:"confidence"`
	Rationale             string   `json:"rationale"`
	SafetyFlags           []string `json:"safetyFlags"`
}

func (s *MemgraphUserStore) CreateNewsCandidateIfMissing(ctx context.Context, input NewsCandidateInput) (NewsCandidate, bool, error) {
	fingerprint := strings.TrimSpace(input.Fingerprint)
	if fingerprint == "" {
		return NewsCandidate{}, false, errors.New("news candidate fingerprint is required")
	}
	sourcesJSON, err := json.Marshal(input.Sources)
	if err != nil {
		return NewsCandidate{}, false, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	params := map[string]any{
		"id":                    uuid.NewString(),
		"fingerprint":           fingerprint,
		"status":                "pending_review",
		"headline":              strings.TrimSpace(input.Headline),
		"proposedTitle":         strings.TrimSpace(input.ProposedTitle),
		"pollType":              strings.TrimSpace(input.PollType),
		"classificationId":      strings.TrimSpace(input.ClassificationID),
		"callName":              strings.TrimSpace(input.CallName),
		"region":                strings.TrimSpace(input.Region),
		"outcomeA":              strings.TrimSpace(input.OutcomeA),
		"outcomeB":              strings.TrimSpace(input.OutcomeB),
		"choiceLabels":          input.ChoiceLabels,
		"resolutionSource":      strings.TrimSpace(input.ResolutionSource),
		"resolutionEvidenceUrl": strings.TrimSpace(input.ResolutionEvidenceURL),
		"resolutionNotes":       strings.TrimSpace(input.ResolutionNotes),
		"startsAt":              strings.TrimSpace(input.StartsAt),
		"endsAt":                strings.TrimSpace(input.EndsAt),
		"confidence":            input.Confidence,
		"rationale":             strings.TrimSpace(input.Rationale),
		"safetyFlags":           input.SafetyFlags,
		"sourcesJSON":           string(sourcesJSON),
		"model":                 strings.TrimSpace(input.Model),
		"now":                   now,
	}

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MERGE (n:NewsCandidate {fingerprint: $fingerprint})
ON CREATE SET
  n.id = $id,
  n.status = $status,
  n.headline = $headline,
  n.proposedTitle = $proposedTitle,
  n.pollType = $pollType,
  n.classificationId = $classificationId,
  n.callName = $callName,
  n.region = $region,
  n.outcomeA = $outcomeA,
  n.outcomeB = $outcomeB,
  n.choiceLabels = $choiceLabels,
  n.resolutionSource = $resolutionSource,
  n.resolutionEvidenceUrl = $resolutionEvidenceUrl,
  n.resolutionNotes = $resolutionNotes,
  n.startsAt = $startsAt,
  n.endsAt = $endsAt,
  n.confidence = $confidence,
  n.rationale = $rationale,
  n.safetyFlags = $safetyFlags,
  n.sourcesJSON = $sourcesJSON,
  n.model = $model,
  n.pollId = "",
  n.reviewNotes = "",
  n.reviewedAt = "",
  n.reviewedBy = "",
  n.createdAt = $now,
  n.updatedAt = $now
RETURN `+newsCandidateReturnCypher()+`, n.id = $id AS created
`, params)
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			record := rows.Record()
			return struct {
				candidate NewsCandidate
				created   bool
			}{newsCandidateFromRecord(record), boolValue(record, "created")}, nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("failed to create news candidate")
	})
	if err != nil {
		return NewsCandidate{}, false, err
	}
	typed := result.(struct {
		candidate NewsCandidate
		created   bool
	})
	return typed.candidate, typed.created, nil
}

func (s *MemgraphUserStore) ListNewsCandidates(ctx context.Context, status string, limit int64) ([]NewsCandidate, error) {
	if limit <= 0 || limit > 250 {
		limit = 100
	}
	status = strings.ToLower(strings.TrimSpace(status))
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (n:NewsCandidate)
WHERE $status = "" OR $status = "all" OR n.status = $status
RETURN `+newsCandidateReturnCypher()+`
ORDER BY n.createdAt DESC
LIMIT $limit
`, map[string]any{"status": status, "limit": limit})
		if err != nil {
			return nil, err
		}
		candidates := []NewsCandidate{}
		for rows.Next(ctx) {
			candidates = append(candidates, newsCandidateFromRecord(rows.Record()))
		}
		return candidates, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]NewsCandidate), nil
}

func (s *MemgraphUserStore) GetNewsCandidate(ctx context.Context, id string) (NewsCandidate, bool, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (n:NewsCandidate {id: $id})
RETURN `+newsCandidateReturnCypher()+`
LIMIT 1
`, map[string]any{"id": strings.TrimSpace(id)})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return newsCandidateFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, nil
	})
	if err != nil {
		return NewsCandidate{}, false, err
	}
	if result == nil {
		return NewsCandidate{}, false, nil
	}
	return result.(NewsCandidate), true, nil
}

func (s *MemgraphUserStore) UpdatePendingNewsCandidate(ctx context.Context, id string, input NewsCandidateUpdateInput) (NewsCandidate, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (n:NewsCandidate {id: $id})
WHERE n.status = "pending_review"
SET
  n.proposedTitle = $proposedTitle,
  n.pollType = $pollType,
  n.classificationId = $classificationId,
  n.callName = $callName,
  n.region = $region,
  n.outcomeA = $outcomeA,
  n.outcomeB = $outcomeB,
  n.choiceLabels = $choiceLabels,
  n.resolutionSource = $resolutionSource,
  n.resolutionEvidenceUrl = $resolutionEvidenceUrl,
  n.resolutionNotes = $resolutionNotes,
  n.startsAt = $startsAt,
  n.endsAt = $endsAt,
  n.confidence = $confidence,
  n.rationale = $rationale,
  n.safetyFlags = $safetyFlags,
  n.updatedAt = $now
RETURN `+newsCandidateReturnCypher()+`
`, map[string]any{
			"id":                    strings.TrimSpace(id),
			"proposedTitle":         strings.TrimSpace(input.ProposedTitle),
			"pollType":              strings.TrimSpace(input.PollType),
			"classificationId":      strings.TrimSpace(input.ClassificationID),
			"callName":              strings.TrimSpace(input.CallName),
			"region":                strings.TrimSpace(input.Region),
			"outcomeA":              strings.TrimSpace(input.OutcomeA),
			"outcomeB":              strings.TrimSpace(input.OutcomeB),
			"choiceLabels":          input.ChoiceLabels,
			"resolutionSource":      strings.TrimSpace(input.ResolutionSource),
			"resolutionEvidenceUrl": strings.TrimSpace(input.ResolutionEvidenceURL),
			"resolutionNotes":       strings.TrimSpace(input.ResolutionNotes),
			"startsAt":              strings.TrimSpace(input.StartsAt),
			"endsAt":                strings.TrimSpace(input.EndsAt),
			"confidence":            input.Confidence,
			"rationale":             strings.TrimSpace(input.Rationale),
			"safetyFlags":           input.SafetyFlags,
			"now":                   now,
		})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return newsCandidateFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("pending news candidate not found")
	})
	if err != nil {
		return NewsCandidate{}, err
	}
	return result.(NewsCandidate), nil
}

func (s *MemgraphUserStore) ReviewNewsCandidate(ctx context.Context, id string, status string, reviewedBy string, reviewNotes string, pollID string) (NewsCandidate, error) {
	status = strings.ToLower(strings.TrimSpace(status))
	if status != "approved" && status != "rejected" {
		return NewsCandidate{}, errors.New("news candidate status must be approved or rejected")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (n:NewsCandidate {id: $id})
WHERE n.status = "pending_review"
SET
  n.status = $status,
  n.reviewedBy = $reviewedBy,
  n.reviewNotes = $reviewNotes,
  n.pollId = $pollId,
  n.reviewedAt = $now,
  n.updatedAt = $now
RETURN `+newsCandidateReturnCypher()+`
`, map[string]any{
			"id":          strings.TrimSpace(id),
			"status":      status,
			"reviewedBy":  strings.TrimSpace(reviewedBy),
			"reviewNotes": strings.TrimSpace(reviewNotes),
			"pollId":      strings.TrimSpace(pollID),
			"now":         now,
		})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return newsCandidateFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("pending news candidate not found")
	})
	if err != nil {
		return NewsCandidate{}, err
	}
	return result.(NewsCandidate), nil
}

func newsCandidateReturnCypher() string {
	return `
  n.id AS id,
  n.fingerprint AS fingerprint,
  coalesce(n.status, "pending_review") AS status,
  coalesce(n.headline, "") AS headline,
  coalesce(n.proposedTitle, "") AS proposedTitle,
  coalesce(n.pollType, "yes_no") AS pollType,
  coalesce(n.classificationId, "policy") AS classificationId,
  coalesce(n.callName, "market") AS callName,
  coalesce(n.region, "National") AS region,
  coalesce(n.outcomeA, "Yes") AS outcomeA,
  coalesce(n.outcomeB, "No") AS outcomeB,
  coalesce(n.choiceLabels, []) AS choiceLabels,
  coalesce(n.resolutionSource, "") AS resolutionSource,
  coalesce(n.resolutionEvidenceUrl, "") AS resolutionEvidenceUrl,
  coalesce(n.resolutionNotes, "") AS resolutionNotes,
  coalesce(n.startsAt, "") AS startsAt,
  coalesce(n.endsAt, "") AS endsAt,
  coalesce(n.confidence, 0) AS confidence,
  coalesce(n.rationale, "") AS rationale,
  coalesce(n.safetyFlags, []) AS safetyFlags,
  coalesce(n.sourcesJSON, "[]") AS sourcesJSON,
  coalesce(n.model, "") AS model,
  coalesce(n.pollId, "") AS pollId,
  coalesce(n.reviewNotes, "") AS reviewNotes,
  coalesce(n.reviewedAt, "") AS reviewedAt,
  coalesce(n.reviewedBy, "") AS reviewedBy,
  n.createdAt AS createdAt,
  n.updatedAt AS updatedAt
`
}

func newsCandidateFromRecord(record *neo4j.Record) NewsCandidate {
	sources := []NewsSource{}
	_ = json.Unmarshal([]byte(stringValue(record, "sourcesJSON")), &sources)
	return NewsCandidate{
		ID:                    stringValue(record, "id"),
		Fingerprint:           stringValue(record, "fingerprint"),
		Status:                stringValue(record, "status"),
		Headline:              stringValue(record, "headline"),
		ProposedTitle:         stringValue(record, "proposedTitle"),
		PollType:              stringValue(record, "pollType"),
		ClassificationID:      stringValue(record, "classificationId"),
		CallName:              stringValue(record, "callName"),
		Region:                stringValue(record, "region"),
		OutcomeA:              stringValue(record, "outcomeA"),
		OutcomeB:              stringValue(record, "outcomeB"),
		ChoiceLabels:          stringSliceValue(record, "choiceLabels"),
		ResolutionSource:      stringValue(record, "resolutionSource"),
		ResolutionEvidenceURL: stringValue(record, "resolutionEvidenceUrl"),
		ResolutionNotes:       stringValue(record, "resolutionNotes"),
		StartsAt:              stringValue(record, "startsAt"),
		EndsAt:                stringValue(record, "endsAt"),
		Confidence:            intValue(record, "confidence"),
		Rationale:             stringValue(record, "rationale"),
		SafetyFlags:           stringSliceValue(record, "safetyFlags"),
		Sources:               sources,
		Model:                 stringValue(record, "model"),
		PollID:                stringValue(record, "pollId"),
		ReviewNotes:           stringValue(record, "reviewNotes"),
		ReviewedAt:            stringValue(record, "reviewedAt"),
		ReviewedBy:            stringValue(record, "reviewedBy"),
		CreatedAt:             stringValue(record, "createdAt"),
		UpdatedAt:             stringValue(record, "updatedAt"),
	}
}
