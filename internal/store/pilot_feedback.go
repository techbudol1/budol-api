package store

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

type PilotEventInput struct {
	AnonymousID string
	Event       string
	DeviceClass string
}

type PilotFeedbackInput struct {
	AnonymousID string
	Category    string
	Area        string
	DeviceClass string
	Rating      int64
	Message     string
}

type PilotFeedback struct {
	ID          string `json:"id"`
	Category    string `json:"category"`
	Area        string `json:"area"`
	DeviceClass string `json:"deviceClass"`
	Rating      int64  `json:"rating"`
	Message     string `json:"message"`
	Status      string `json:"status"`
	AdminNote   string `json:"adminNote"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

type PilotFunnel struct {
	Started       int64 `json:"started"`
	Connected     int64 `json:"connected"`
	PublicTrade   int64 `json:"publicTrade"`
	PrivateTrade  int64 `json:"privateTrade"`
	PositionSold  int64 `json:"positionSold"`
	PublicClaim   int64 `json:"publicClaim"`
	PrivateClaim  int64 `json:"privateClaim"`
	Traded        int64 `json:"traded"`
	PrivacyUsed   int64 `json:"privacyUsed"`
	FeedbackCount int64 `json:"feedbackCount"`
	NewFeedback   int64 `json:"newFeedback"`
}

type PilotOverview struct {
	Funnel   PilotFunnel     `json:"funnel"`
	Feedback []PilotFeedback `json:"feedback"`
}

func (s *MemgraphUserStore) RecordPilotEvent(ctx context.Context, input PilotEventInput) error {
	now := time.Now().UTC().Format(time.RFC3339)
	params := map[string]any{
		"anonymousID": strings.TrimSpace(input.AnonymousID),
		"deviceClass": normalizeEventValue(input.DeviceClass, "unknown"),
		"event":       normalizeEventValue(input.Event, "pilot_started"),
		"now":         now,
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MERGE (p:PilotParticipant {anonymousId: $anonymousID})
ON CREATE SET p.firstSeenAt = $now
SET p.lastSeenAt = $now,
    p.deviceClass = $deviceClass
MERGE (p)-[:COMPLETED]->(e:PilotEvent {anonymousId: $anonymousID, event: $event})
ON CREATE SET e.createdAt = $now
SET e.updatedAt = $now
RETURN e.event AS event
`, params)
		if err != nil {
			return nil, err
		}
		_, err = rows.Collect(ctx)
		return nil, err
	})
	return err
}

func (s *MemgraphUserStore) CreatePilotFeedback(ctx context.Context, input PilotFeedbackInput) (PilotFeedback, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	params := map[string]any{
		"id":          uuid.NewString(),
		"anonymousID": strings.TrimSpace(input.AnonymousID),
		"category":    normalizeEventValue(input.Category, "other"),
		"area":        normalizeEventValue(input.Area, "other"),
		"deviceClass": normalizeEventValue(input.DeviceClass, "unknown"),
		"rating":      input.Rating,
		"message":     trimEventText(input.Message, 1500),
		"now":         now,
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MERGE (p:PilotParticipant {anonymousId: $anonymousID})
ON CREATE SET p.firstSeenAt = $now
SET p.lastSeenAt = $now,
    p.deviceClass = $deviceClass
CREATE (f:PilotFeedback {
  id: $id,
  category: $category,
  area: $area,
  deviceClass: $deviceClass,
  rating: $rating,
  message: $message,
  status: "new",
  adminNote: "",
  createdAt: $now,
  updatedAt: $now
})
MERGE (p)-[:SUBMITTED_FEEDBACK]->(f)
MERGE (p)-[:COMPLETED]->(e:PilotEvent {anonymousId: $anonymousID, event: "feedback_submitted"})
ON CREATE SET e.createdAt = $now
SET e.updatedAt = $now
RETURN f.id AS id, f.category AS category, f.area AS area,
       f.deviceClass AS deviceClass, f.rating AS rating, f.message AS message,
       f.status AS status, f.adminNote AS adminNote,
       f.createdAt AS createdAt, f.updatedAt AS updatedAt
`, params)
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return pilotFeedbackFromRecord(rows.Record()), nil
		}
		return nil, rows.Err()
	})
	if err != nil {
		return PilotFeedback{}, err
	}
	if result == nil {
		return PilotFeedback{}, nil
	}
	return result.(PilotFeedback), nil
}

func (s *MemgraphUserStore) PilotOverview(ctx context.Context, feedbackLimit int64) (PilotOverview, error) {
	if feedbackLimit < 1 || feedbackLimit > 500 {
		feedbackLimit = 200
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		funnel, err := pilotFunnelFromTransaction(ctx, tx)
		if err != nil {
			return nil, err
		}
		rows, err := tx.Run(ctx, `
MATCH (f:PilotFeedback)
RETURN f.id AS id, coalesce(f.category, "other") AS category,
       coalesce(f.area, "other") AS area,
       coalesce(f.deviceClass, "unknown") AS deviceClass,
       coalesce(f.rating, 0) AS rating, coalesce(f.message, "") AS message,
       coalesce(f.status, "new") AS status, coalesce(f.adminNote, "") AS adminNote,
       coalesce(f.createdAt, "") AS createdAt, coalesce(f.updatedAt, "") AS updatedAt
ORDER BY f.createdAt DESC
LIMIT $limit
`, map[string]any{"limit": feedbackLimit})
		if err != nil {
			return nil, err
		}
		feedback := []PilotFeedback{}
		for rows.Next(ctx) {
			feedback = append(feedback, pilotFeedbackFromRecord(rows.Record()))
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return PilotOverview{Funnel: funnel, Feedback: feedback}, nil
	})
	if err != nil {
		return PilotOverview{}, err
	}
	return result.(PilotOverview), nil
}

func (s *MemgraphUserStore) UpdatePilotFeedback(ctx context.Context, id string, status string, adminNote string) (PilotFeedback, bool, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (f:PilotFeedback {id: $id})
SET f.status = $status,
    f.adminNote = $adminNote,
    f.updatedAt = $now
RETURN f.id AS id, coalesce(f.category, "other") AS category,
       coalesce(f.area, "other") AS area,
       coalesce(f.deviceClass, "unknown") AS deviceClass,
       coalesce(f.rating, 0) AS rating, coalesce(f.message, "") AS message,
       coalesce(f.status, "new") AS status, coalesce(f.adminNote, "") AS adminNote,
       coalesce(f.createdAt, "") AS createdAt, coalesce(f.updatedAt, "") AS updatedAt
`, map[string]any{"adminNote": trimEventText(adminNote, 1000), "id": strings.TrimSpace(id), "now": now, "status": normalizeEventValue(status, "reviewed")})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return pilotFeedbackFromRecord(rows.Record()), nil
		}
		return nil, rows.Err()
	})
	if err != nil {
		return PilotFeedback{}, false, err
	}
	if result == nil {
		return PilotFeedback{}, false, nil
	}
	return result.(PilotFeedback), true, nil
}

func pilotFunnelFromTransaction(ctx context.Context, tx neo4j.ManagedTransaction) (PilotFunnel, error) {
	record, err := metricRecord(ctx, tx, `
OPTIONAL MATCH (p:PilotParticipant)
OPTIONAL MATCH (p)-[:COMPLETED]->(e:PilotEvent)
WITH count(DISTINCT p.anonymousId) AS started,
     count(DISTINCT CASE WHEN e.event = "wallet_connected" THEN p.anonymousId ELSE null END) AS connected,
     count(DISTINCT CASE WHEN e.event = "public_trade_completed" THEN p.anonymousId ELSE null END) AS publicTrade,
     count(DISTINCT CASE WHEN e.event = "private_trade_completed" THEN p.anonymousId ELSE null END) AS privateTrade,
     count(DISTINCT CASE WHEN e.event = "position_sold" THEN p.anonymousId ELSE null END) AS positionSold,
     count(DISTINCT CASE WHEN e.event = "public_claim_completed" THEN p.anonymousId ELSE null END) AS publicClaim,
     count(DISTINCT CASE WHEN e.event = "private_claim_completed" THEN p.anonymousId ELSE null END) AS privateClaim,
     count(DISTINCT CASE WHEN e.event IN ["public_trade_completed", "private_trade_completed"] THEN p.anonymousId ELSE null END) AS traded,
     count(DISTINCT CASE WHEN e.event IN ["private_trade_completed", "private_claim_completed"] THEN p.anonymousId ELSE null END) AS privacyUsed
OPTIONAL MATCH (f:PilotFeedback)
RETURN started, connected, publicTrade, privateTrade, positionSold, publicClaim, privateClaim, traded, privacyUsed,
       count(f) AS feedbackCount,
       sum(CASE WHEN f.id IS NOT NULL AND coalesce(f.status, "new") = "new" THEN 1 ELSE 0 END) AS newFeedback
`, nil)
	if err != nil {
		return PilotFunnel{}, err
	}
	return PilotFunnel{
		Started:       intValue(record, "started"),
		Connected:     intValue(record, "connected"),
		PublicTrade:   intValue(record, "publicTrade"),
		PrivateTrade:  intValue(record, "privateTrade"),
		PositionSold:  intValue(record, "positionSold"),
		PublicClaim:   intValue(record, "publicClaim"),
		PrivateClaim:  intValue(record, "privateClaim"),
		Traded:        intValue(record, "traded"),
		PrivacyUsed:   intValue(record, "privacyUsed"),
		FeedbackCount: intValue(record, "feedbackCount"),
		NewFeedback:   intValue(record, "newFeedback"),
	}, nil
}

func pilotFeedbackFromRecord(record *neo4j.Record) PilotFeedback {
	return PilotFeedback{
		ID:          stringValue(record, "id"),
		Category:    stringValue(record, "category"),
		Area:        stringValue(record, "area"),
		DeviceClass: stringValue(record, "deviceClass"),
		Rating:      intValue(record, "rating"),
		Message:     stringValue(record, "message"),
		Status:      stringValue(record, "status"),
		AdminNote:   stringValue(record, "adminNote"),
		CreatedAt:   stringValue(record, "createdAt"),
		UpdatedAt:   stringValue(record, "updatedAt"),
	}
}
