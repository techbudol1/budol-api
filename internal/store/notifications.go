package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

type Notification struct {
	ID        string `json:"id"`
	UserID    string `json:"userId"`
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	Detail    string `json:"detail"`
	Link      string `json:"link"`
	ReadAt    string `json:"readAt"`
	CreatedAt string `json:"createdAt"`
}

type WatchlistItem struct {
	PollID    string `json:"pollId"`
	Slug      string `json:"slug"`
	Title     string `json:"title"`
	Category  string `json:"category"`
	Region    string `json:"region"`
	AddedAt   string `json:"addedAt"`
	UpdatedAt string `json:"updatedAt"`
}

type MarketAlertInput struct {
	PriceEnabled      bool
	PriceDirection    string
	PriceThreshold    int64
	ClosingEnabled    bool
	ResolutionEnabled bool
}

type MarketAlert struct {
	PollID            string `json:"pollId"`
	Slug              string `json:"slug"`
	Title             string `json:"title"`
	Enabled           bool   `json:"enabled"`
	PriceEnabled      bool   `json:"priceEnabled"`
	PriceDirection    string `json:"priceDirection"`
	PriceThreshold    int64  `json:"priceThreshold"`
	ClosingEnabled    bool   `json:"closingEnabled"`
	ResolutionEnabled bool   `json:"resolutionEnabled"`
	CreatedAt         string `json:"createdAt"`
	UpdatedAt         string `json:"updatedAt"`
}

type AdminActivity struct {
	ID         string `json:"id"`
	Actor      string `json:"actor"`
	Action     string `json:"action"`
	TargetType string `json:"targetType"`
	TargetID   string `json:"targetId"`
	Title      string `json:"title"`
	Detail     string `json:"detail"`
	IPAddress  string `json:"ipAddress"`
	UserAgent  string `json:"userAgent"`
	CreatedAt  string `json:"createdAt"`
}

type AdminActivityInput struct {
	Actor      string
	Action     string
	TargetType string
	TargetID   string
	Title      string
	Detail     string
	IPAddress  string
	UserAgent  string
}

func (s *MemgraphUserStore) ListNotifications(ctx context.Context, userID string) ([]Notification, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:User {id: $userID})-[:HAS_NOTIFICATION]->(n:Notification)
RETURN
  n.id AS id,
  n.userId AS userId,
  coalesce(n.kind, "") AS kind,
  coalesce(n.title, "") AS title,
  coalesce(n.detail, "") AS detail,
  coalesce(n.link, "") AS link,
  coalesce(n.readAt, "") AS readAt,
  n.createdAt AS createdAt
ORDER BY n.createdAt DESC
LIMIT 80
`, map[string]any{"userID": userID})
		if err != nil {
			return nil, err
		}
		notifications := []Notification{}
		for rows.Next(ctx) {
			notifications = append(notifications, notificationFromRecord(rows.Record()))
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return notifications, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]Notification), nil
}

func (s *MemgraphUserStore) CreateNotification(ctx context.Context, userID string, kind string, title string, detail string, link string) (Notification, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	params := map[string]any{
		"id":     uuid.NewString(),
		"userID": userID,
		"kind":   normalizeEventValue(kind, "general"),
		"title":  trimEventText(title, 120),
		"detail": trimEventText(detail, 320),
		"link":   trimEventText(link, 180),
		"now":    now,
	}
	if params["title"] == "" {
		params["title"] = "BudolPH update"
	}

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (u:User {id: $userID})
CREATE (n:Notification {
  id: $id,
  userId: $userID,
  kind: $kind,
  title: $title,
  detail: $detail,
  link: $link,
  readAt: "",
  createdAt: $now,
  updatedAt: $now
})
MERGE (u)-[:HAS_NOTIFICATION]->(n)
RETURN
  n.id AS id,
  n.userId AS userId,
  n.kind AS kind,
  n.title AS title,
  n.detail AS detail,
  n.link AS link,
  n.readAt AS readAt,
  n.createdAt AS createdAt
`, params)
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return notificationFromRecord(rows.Record()), nil
		}
		return nil, rows.Err()
	})
	if err != nil {
		return Notification{}, err
	}
	if result == nil {
		return Notification{}, nil
	}
	return result.(Notification), nil
}

func (s *MemgraphUserStore) MarkNotificationRead(ctx context.Context, userID string, id string) (Notification, bool, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:User {id: $userID})-[:HAS_NOTIFICATION]->(n:Notification {id: $id})
SET n.readAt = CASE WHEN coalesce(n.readAt, "") = "" THEN $now ELSE n.readAt END,
  n.updatedAt = $now
RETURN
  n.id AS id,
  n.userId AS userId,
  coalesce(n.kind, "") AS kind,
  coalesce(n.title, "") AS title,
  coalesce(n.detail, "") AS detail,
  coalesce(n.link, "") AS link,
  coalesce(n.readAt, "") AS readAt,
  n.createdAt AS createdAt
`, map[string]any{"userID": userID, "id": id, "now": now})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return notificationFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, nil
	})
	if err != nil {
		return Notification{}, false, err
	}
	if result == nil {
		return Notification{}, false, nil
	}
	return result.(Notification), true, nil
}

func (s *MemgraphUserStore) MarkAllNotificationsRead(ctx context.Context, userID string) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:User {id: $userID})-[:HAS_NOTIFICATION]->(n:Notification)
WHERE coalesce(n.readAt, "") = ""
SET n.readAt = $now,
  n.updatedAt = $now
RETURN count(n) AS count
`, map[string]any{"userID": userID, "now": now})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return intValue(rows.Record(), "count"), nil
		}
		return int64(0), rows.Err()
	})
	if err != nil {
		return 0, err
	}
	return result.(int64), nil
}

func (s *MemgraphUserStore) ListWatchlist(ctx context.Context, userID string) ([]WatchlistItem, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:User {id: $userID})-[w:WATCHES]->(p:Poll)
RETURN
  p.id AS pollId,
  p.slug AS slug,
  p.title AS title,
  coalesce(p.category, "") AS category,
  coalesce(p.region, "") AS region,
  w.createdAt AS addedAt,
  coalesce(w.updatedAt, w.createdAt) AS updatedAt
ORDER BY w.createdAt DESC
`, map[string]any{"userID": userID})
		if err != nil {
			return nil, err
		}
		items := []WatchlistItem{}
		for rows.Next(ctx) {
			items = append(items, watchlistItemFromRecord(rows.Record()))
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return items, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]WatchlistItem), nil
}

func (s *MemgraphUserStore) AddWatchlist(ctx context.Context, userID string, slug string) ([]WatchlistItem, bool, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (u:User {id: $userID})
MATCH (p:Poll {slug: $slug})
MERGE (u)-[w:WATCHES]->(p)
ON CREATE SET w.createdAt = $now, w.wasCreated = true
ON MATCH SET w.wasCreated = false
SET w.updatedAt = $now
RETURN coalesce(w.wasCreated, false) AS created
`, map[string]any{"userID": userID, "slug": slugify(slug), "now": now})
		if err != nil {
			return nil, err
		}
		created := false
		if rows.Next(ctx) {
			created = boolValue(rows.Record(), "created")
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return created, nil
	})
	if err != nil {
		return nil, false, err
	}
	items, err := s.ListWatchlist(ctx, userID)
	if err != nil {
		return nil, false, err
	}
	created, _ := result.(bool)
	return items, created, nil
}

func (s *MemgraphUserStore) RemoveWatchlist(ctx context.Context, userID string, slug string) ([]WatchlistItem, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		_, err := tx.Run(ctx, `
MATCH (:User {id: $userID})-[w:WATCHES]->(:Poll {slug: $slug})
DELETE w
`, map[string]any{"userID": userID, "slug": slugify(slug)})
		return nil, err
	})
	if err != nil {
		return nil, err
	}
	return s.ListWatchlist(ctx, userID)
}

func (s *MemgraphUserStore) GetMarketAlert(ctx context.Context, userID string, slug string) (MarketAlert, bool, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:User {id: $userID})-[a:HAS_MARKET_ALERT]->(p:Poll {slug: $slug})
RETURN
  p.id AS pollId,
  p.slug AS slug,
  p.title AS title,
  true AS enabled,
  coalesce(a.priceEnabled, false) AS priceEnabled,
  coalesce(a.priceDirection, "above") AS priceDirection,
  coalesce(a.priceThreshold, 50) AS priceThreshold,
  coalesce(a.closingEnabled, false) AS closingEnabled,
  coalesce(a.resolutionEnabled, false) AS resolutionEnabled,
  a.createdAt AS createdAt,
  coalesce(a.updatedAt, a.createdAt) AS updatedAt
`, map[string]any{"userID": strings.TrimSpace(userID), "slug": slugify(slug)})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return marketAlertFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, nil
	})
	if err != nil {
		return MarketAlert{}, false, err
	}
	if result == nil {
		return MarketAlert{}, false, nil
	}
	return result.(MarketAlert), true, nil
}

func (s *MemgraphUserStore) UpsertMarketAlert(ctx context.Context, userID string, slug string, input MarketAlertInput) (MarketAlert, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (u:User {id: $userID})
MATCH (p:Poll {slug: $slug})
MERGE (u)-[a:HAS_MARKET_ALERT]->(p)
ON CREATE SET a.createdAt = $now
SET
  a.priceEnabled = $priceEnabled,
  a.priceDirection = $priceDirection,
  a.priceThreshold = $priceThreshold,
  a.closingEnabled = $closingEnabled,
  a.resolutionEnabled = $resolutionEnabled,
  a.closingAlertSentAt = CASE WHEN $closingEnabled THEN coalesce(a.closingAlertSentAt, "") ELSE "" END,
  a.resolutionAlertStatus = CASE WHEN $resolutionEnabled THEN coalesce(a.resolutionAlertStatus, "") ELSE "" END,
  a.updatedAt = $now
RETURN
  p.id AS pollId,
  p.slug AS slug,
  p.title AS title,
  true AS enabled,
  a.priceEnabled AS priceEnabled,
  a.priceDirection AS priceDirection,
  a.priceThreshold AS priceThreshold,
  a.closingEnabled AS closingEnabled,
  a.resolutionEnabled AS resolutionEnabled,
  a.createdAt AS createdAt,
  a.updatedAt AS updatedAt
`, map[string]any{
			"userID":            strings.TrimSpace(userID),
			"slug":              slugify(slug),
			"priceEnabled":      input.PriceEnabled,
			"priceDirection":    input.PriceDirection,
			"priceThreshold":    input.PriceThreshold,
			"closingEnabled":    input.ClosingEnabled,
			"resolutionEnabled": input.ResolutionEnabled,
			"now":               now,
		})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return marketAlertFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("user or market not found")
	})
	if err != nil {
		return MarketAlert{}, err
	}
	return result.(MarketAlert), nil
}

func (s *MemgraphUserStore) DeleteMarketAlert(ctx context.Context, userID string, slug string) error {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		_, err := tx.Run(ctx, `
MATCH (:User {id: $userID})-[a:HAS_MARKET_ALERT]->(:Poll {slug: $slug})
DELETE a
`, map[string]any{"userID": strings.TrimSpace(userID), "slug": slugify(slug)})
		return nil, err
	})
	return err
}

func (s *MemgraphUserStore) NotifyMarketPriceAlerts(ctx context.Context, pollID string, exceptUserID string, oldYesPercent int64, newYesPercent int64) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (u:User)-[a:HAS_MARKET_ALERT]->(p:Poll {id: $pollID})
WHERE u.id <> $exceptUserID
  AND coalesce(a.priceEnabled, false) = true
  AND (
    (a.priceDirection = "above" AND $oldYesPercent < a.priceThreshold AND $newYesPercent >= a.priceThreshold)
    OR
    (a.priceDirection = "below" AND $oldYesPercent > a.priceThreshold AND $newYesPercent <= a.priceThreshold)
  )
CREATE (n:Notification {
  id: $idPrefix + u.id,
  userId: u.id,
  kind: "market_price_alert",
  title: "Market price alert",
  detail: p.title + ": Yes reached " + toString($newYesPercent) + "c.",
  link: "/markets/" + p.slug,
  readAt: "",
  createdAt: $now,
  updatedAt: $now
})
MERGE (u)-[:HAS_NOTIFICATION]->(n)
SET a.lastPriceAlertAt = $now,
    a.updatedAt = $now
RETURN count(n) AS count
`, map[string]any{
			"pollID":        strings.TrimSpace(pollID),
			"exceptUserID":  strings.TrimSpace(exceptUserID),
			"oldYesPercent": oldYesPercent,
			"newYesPercent": newYesPercent,
			"idPrefix":      uuid.NewString() + "-",
			"now":           now,
		})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return intValue(rows.Record(), "count"), nil
		}
		return int64(0), rows.Err()
	})
	if err != nil {
		return 0, err
	}
	return result.(int64), nil
}

func (s *MemgraphUserStore) NotifyMarketResolutionAlerts(ctx context.Context, pollSlug string, status string) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (u:User)-[a:HAS_MARKET_ALERT]->(p:Poll {slug: $slug})
WHERE coalesce(a.resolutionEnabled, false) = true
  AND coalesce(a.resolutionAlertStatus, "") <> $status
CREATE (n:Notification {
  id: $idPrefix + u.id,
  userId: u.id,
  kind: "market_resolution_alert",
  title: "Market " + $status,
  detail: p.title + " has been " + $status + ".",
  link: "/markets/" + p.slug,
  readAt: "",
  createdAt: $now,
  updatedAt: $now
})
MERGE (u)-[:HAS_NOTIFICATION]->(n)
SET a.resolutionAlertStatus = $status,
    a.updatedAt = $now
RETURN count(n) AS count
`, map[string]any{
			"slug":     slugify(pollSlug),
			"status":   normalizeEventValue(status, "resolved"),
			"idPrefix": uuid.NewString() + "-",
			"now":      now,
		})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return intValue(rows.Record(), "count"), nil
		}
		return int64(0), rows.Err()
	})
	if err != nil {
		return 0, err
	}
	return result.(int64), nil
}

func (s *MemgraphUserStore) ProcessDueMarketClosingAlerts(ctx context.Context, now time.Time) (int64, error) {
	nowText := now.UTC().Format(time.RFC3339)
	deadline := now.UTC().Add(24 * time.Hour).Format(time.RFC3339)
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (u:User)-[a:HAS_MARKET_ALERT]->(p:Poll)
WHERE coalesce(a.closingEnabled, false) = true
  AND coalesce(a.closingAlertSentAt, "") = ""
  AND p.status = "published"
  AND coalesce(p.endsAt, "") > $now
  AND p.endsAt <= $deadline
CREATE (n:Notification {
  id: $idPrefix + u.id + "-" + p.id,
  userId: u.id,
  kind: "market_closing_alert",
  title: "Market closing soon",
  detail: p.title + " closes within 24 hours.",
  link: "/markets/" + p.slug,
  readAt: "",
  createdAt: $now,
  updatedAt: $now
})
MERGE (u)-[:HAS_NOTIFICATION]->(n)
SET a.closingAlertSentAt = $now,
    a.updatedAt = $now
RETURN count(n) AS count
`, map[string]any{
			"now":      nowText,
			"deadline": deadline,
			"idPrefix": uuid.NewString() + "-",
		})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return intValue(rows.Record(), "count"), nil
		}
		return int64(0), rows.Err()
	})
	if err != nil {
		return 0, err
	}
	return result.(int64), nil
}

func (s *MemgraphUserStore) NotifyWatchers(ctx context.Context, pollSlug string, exceptUserID string, kind string, title string, detail string, link string) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (u:User)-[:WATCHES]->(p:Poll {slug: $slug})
WHERE u.id <> $exceptUserID
CREATE (n:Notification {
  id: $idPrefix + u.id,
  userId: u.id,
  kind: $kind,
  title: $title,
  detail: $detail,
  link: $link,
  readAt: "",
  createdAt: $now,
  updatedAt: $now
})
MERGE (u)-[:HAS_NOTIFICATION]->(n)
RETURN count(n) AS count
`, map[string]any{
			"slug":         slugify(pollSlug),
			"idPrefix":     uuid.NewString() + "-",
			"exceptUserID": exceptUserID,
			"kind":         normalizeEventValue(kind, "watchlist"),
			"title":        trimEventText(title, 120),
			"detail":       trimEventText(detail, 320),
			"link":         trimEventText(link, 180),
			"now":          now,
		})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return intValue(rows.Record(), "count"), nil
		}
		return int64(0), rows.Err()
	})
	if err != nil {
		return 0, err
	}
	return result.(int64), nil
}

func (s *MemgraphUserStore) ListAdminActivity(ctx context.Context) ([]AdminActivity, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (a:AdminActivity)
RETURN
  a.id AS id,
  coalesce(a.actor, "") AS actor,
  coalesce(a.action, "") AS action,
  coalesce(a.targetType, "") AS targetType,
  coalesce(a.targetId, "") AS targetId,
  coalesce(a.title, "") AS title,
  coalesce(a.detail, "") AS detail,
  coalesce(a.ipAddress, "") AS ipAddress,
  coalesce(a.userAgent, "") AS userAgent,
  a.createdAt AS createdAt
ORDER BY a.createdAt DESC
LIMIT 120
`, nil)
		if err != nil {
			return nil, err
		}
		activities := []AdminActivity{}
		for rows.Next(ctx) {
			activities = append(activities, adminActivityFromRecord(rows.Record()))
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return activities, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]AdminActivity), nil
}

func (s *MemgraphUserStore) CreateAdminActivity(ctx context.Context, input AdminActivityInput) (AdminActivity, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	params := map[string]any{
		"id":         uuid.NewString(),
		"actor":      trimEventText(input.Actor, 80),
		"action":     normalizeEventValue(input.Action, "admin_action"),
		"targetType": normalizeEventValue(input.TargetType, "system"),
		"targetID":   trimEventText(input.TargetID, 120),
		"title":      trimEventText(input.Title, 140),
		"detail":     trimEventText(input.Detail, 360),
		"ipAddress":  trimEventText(input.IPAddress, 80),
		"userAgent":  trimEventText(input.UserAgent, 220),
		"now":        now,
	}
	if params["actor"] == "" {
		params["actor"] = "admin"
	}
	if params["title"] == "" {
		params["title"] = "Admin action"
	}

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
CREATE (a:AdminActivity {
  id: $id,
  actor: $actor,
  action: $action,
  targetType: $targetType,
  targetId: $targetID,
  title: $title,
  detail: $detail,
  ipAddress: $ipAddress,
  userAgent: $userAgent,
  createdAt: $now,
  updatedAt: $now
})
RETURN
  a.id AS id,
  a.actor AS actor,
  a.action AS action,
  a.targetType AS targetType,
  a.targetId AS targetId,
  a.title AS title,
  a.detail AS detail,
  coalesce(a.ipAddress, "") AS ipAddress,
  coalesce(a.userAgent, "") AS userAgent,
  a.createdAt AS createdAt
`, params)
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return adminActivityFromRecord(rows.Record()), nil
		}
		return nil, rows.Err()
	})
	if err != nil {
		return AdminActivity{}, err
	}
	return result.(AdminActivity), nil
}

func notificationFromRecord(record *neo4j.Record) Notification {
	return Notification{
		ID:        stringValue(record, "id"),
		UserID:    stringValue(record, "userId"),
		Kind:      stringValue(record, "kind"),
		Title:     stringValue(record, "title"),
		Detail:    stringValue(record, "detail"),
		Link:      stringValue(record, "link"),
		ReadAt:    stringValue(record, "readAt"),
		CreatedAt: stringValue(record, "createdAt"),
	}
}

func watchlistItemFromRecord(record *neo4j.Record) WatchlistItem {
	return WatchlistItem{
		PollID:    stringValue(record, "pollId"),
		Slug:      stringValue(record, "slug"),
		Title:     stringValue(record, "title"),
		Category:  stringValue(record, "category"),
		Region:    stringValue(record, "region"),
		AddedAt:   stringValue(record, "addedAt"),
		UpdatedAt: stringValue(record, "updatedAt"),
	}
}

func marketAlertFromRecord(record *neo4j.Record) MarketAlert {
	return MarketAlert{
		PollID:            stringValue(record, "pollId"),
		Slug:              stringValue(record, "slug"),
		Title:             stringValue(record, "title"),
		Enabled:           boolValue(record, "enabled"),
		PriceEnabled:      boolValue(record, "priceEnabled"),
		PriceDirection:    stringValue(record, "priceDirection"),
		PriceThreshold:    intValue(record, "priceThreshold"),
		ClosingEnabled:    boolValue(record, "closingEnabled"),
		ResolutionEnabled: boolValue(record, "resolutionEnabled"),
		CreatedAt:         stringValue(record, "createdAt"),
		UpdatedAt:         stringValue(record, "updatedAt"),
	}
}

func adminActivityFromRecord(record *neo4j.Record) AdminActivity {
	return AdminActivity{
		ID:         stringValue(record, "id"),
		Actor:      stringValue(record, "actor"),
		Action:     stringValue(record, "action"),
		TargetType: stringValue(record, "targetType"),
		TargetID:   stringValue(record, "targetId"),
		Title:      stringValue(record, "title"),
		Detail:     stringValue(record, "detail"),
		IPAddress:  stringValue(record, "ipAddress"),
		UserAgent:  stringValue(record, "userAgent"),
		CreatedAt:  stringValue(record, "createdAt"),
	}
}

func normalizeEventValue(value string, fallback string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "_")
	if value == "" {
		return fallback
	}
	return value
}

func trimEventText(value string, maxLength int) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if maxLength > 0 && len(value) > maxLength {
		return value[:maxLength]
	}
	return value
}
