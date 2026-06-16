package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

var validEngineScopes = map[string]bool{
	"transactions:write": true,
	"transactions:read":  true,
	"wallets:read":       true,
	"wallets:write":      true,
	"contracts:read":     true,
	"contracts:write":    true,
	"admin":              true,
}

func (s *MemgraphUserStore) CreateEngineApp(ctx context.Context, input EngineAppInput) (EngineApp, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return EngineApp{}, errors.New("app name is required")
	}
	environment := normalizeEngineEnvironment(input.Environment)
	now := time.Now().UTC().Format(time.RFC3339)
	params := map[string]any{
		"id":               uuid.NewString(),
		"name":             name,
		"environment":      environment,
		"allowedChains":    normalizeInt64Slice(input.AllowedChains),
		"allowedContracts": normalizeStringSlice(input.AllowedContracts),
		"rateLimit":        normalizeRateLimit(input.RateLimitPerMin),
		"now":              now,
	}

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		record, err := tx.Run(ctx, `
CREATE (a:EngineApp {
  id: $id,
  name: $name,
  environment: $environment,
  status: "active",
  allowedChains: $allowedChains,
  allowedContracts: $allowedContracts,
  rateLimitPerMinute: $rateLimit,
  createdAt: $now,
  updatedAt: $now
})
RETURN
  a.id AS id,
  a.name AS name,
  a.environment AS environment,
  a.status AS status,
  a.allowedChains AS allowedChains,
  a.allowedContracts AS allowedContracts,
  a.rateLimitPerMinute AS rateLimitPerMinute,
  a.createdAt AS createdAt,
  a.updatedAt AS updatedAt
`, params)
		if err != nil {
			return nil, err
		}
		if record.Next(ctx) {
			return engineAppFromRecord(record.Record()), nil
		}
		return nil, record.Err()
	})
	if err != nil {
		return EngineApp{}, err
	}
	return result.(EngineApp), nil
}

func (s *MemgraphUserStore) ListEngineApps(ctx context.Context) ([]EngineApp, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (a:EngineApp)
RETURN
  a.id AS id,
  a.name AS name,
  a.environment AS environment,
  coalesce(a.status, "active") AS status,
  coalesce(a.allowedChains, []) AS allowedChains,
  coalesce(a.allowedContracts, []) AS allowedContracts,
  coalesce(a.rateLimitPerMinute, 60) AS rateLimitPerMinute,
  a.createdAt AS createdAt,
  a.updatedAt AS updatedAt
ORDER BY a.createdAt DESC
`, nil)
		if err != nil {
			return nil, err
		}
		apps := []EngineApp{}
		for rows.Next(ctx) {
			apps = append(apps, engineAppFromRecord(rows.Record()))
		}
		return apps, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]EngineApp), nil
}

func (s *MemgraphUserStore) GetEngineApp(ctx context.Context, id string) (EngineApp, bool, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		record, err := tx.Run(ctx, `
MATCH (a:EngineApp {id: $id})
RETURN
  a.id AS id,
  a.name AS name,
  a.environment AS environment,
  coalesce(a.status, "active") AS status,
  coalesce(a.allowedChains, []) AS allowedChains,
  coalesce(a.allowedContracts, []) AS allowedContracts,
  coalesce(a.rateLimitPerMinute, 60) AS rateLimitPerMinute,
  a.createdAt AS createdAt,
  a.updatedAt AS updatedAt
`, map[string]any{"id": strings.TrimSpace(id)})
		if err != nil {
			return nil, err
		}
		if record.Next(ctx) {
			return engineAppFromRecord(record.Record()), nil
		}
		return nil, record.Err()
	})
	if err != nil {
		return EngineApp{}, false, err
	}
	if result == nil {
		return EngineApp{}, false, nil
	}
	return result.(EngineApp), true, nil
}

func (s *MemgraphUserStore) CreateEngineAPIKey(ctx context.Context, appID string, input EngineAPIKeyInput, prefix string, secretHash string) (EngineAPIKey, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return EngineAPIKey{}, errors.New("key name is required")
	}
	scopes, err := normalizeEngineScopes(input.Scopes)
	if err != nil {
		return EngineAPIKey{}, err
	}
	if strings.TrimSpace(prefix) == "" || strings.TrimSpace(secretHash) == "" {
		return EngineAPIKey{}, errors.New("key material is required")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	params := map[string]any{
		"id":             uuid.NewString(),
		"appID":          strings.TrimSpace(appID),
		"name":           name,
		"prefix":         strings.TrimSpace(prefix),
		"secretHash":     strings.TrimSpace(secretHash),
		"scopes":         scopes,
		"allowedOrigins": normalizeStringSlice(input.AllowedOrigins),
		"allowedIPs":     normalizeStringSlice(input.AllowedIPs),
		"expiresAt":      strings.TrimSpace(input.ExpiresAt),
		"now":            now,
	}

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		record, err := tx.Run(ctx, `
MATCH (a:EngineApp {id: $appID})
CREATE (k:EngineAPIKey {
  id: $id,
  appId: $appID,
  name: $name,
  prefix: $prefix,
  secretHash: $secretHash,
  scopes: $scopes,
  allowedOrigins: $allowedOrigins,
  allowedIPs: $allowedIPs,
  status: "active",
  expiresAt: $expiresAt,
  createdAt: $now
})
MERGE (a)-[:HAS_ENGINE_KEY]->(k)
RETURN
  k.id AS id,
  k.appId AS appId,
  a.name AS appName,
  k.name AS name,
  k.prefix AS prefix,
  k.scopes AS scopes,
  coalesce(k.allowedOrigins, []) AS allowedOrigins,
  coalesce(k.allowedIPs, []) AS allowedIPs,
  coalesce(k.status, "active") AS status,
  coalesce(k.expiresAt, "") AS expiresAt,
  coalesce(k.lastUsedAt, "") AS lastUsedAt,
  k.createdAt AS createdAt,
  coalesce(k.revokedAt, "") AS revokedAt
`, params)
		if err != nil {
			return nil, err
		}
		if record.Next(ctx) {
			return engineAPIKeyFromRecord(record.Record()), nil
		}
		if err := record.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("engine app not found")
	})
	if err != nil {
		return EngineAPIKey{}, err
	}
	return result.(EngineAPIKey), nil
}

func (s *MemgraphUserStore) ListEngineAPIKeys(ctx context.Context, appID string) ([]EngineAPIKey, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (a:EngineApp {id: $appID})-[:HAS_ENGINE_KEY]->(k:EngineAPIKey)
RETURN
  k.id AS id,
  k.appId AS appId,
  a.name AS appName,
  k.name AS name,
  k.prefix AS prefix,
  k.scopes AS scopes,
  coalesce(k.allowedOrigins, []) AS allowedOrigins,
  coalesce(k.allowedIPs, []) AS allowedIPs,
  coalesce(k.status, "active") AS status,
  coalesce(k.expiresAt, "") AS expiresAt,
  coalesce(k.lastUsedAt, "") AS lastUsedAt,
  k.createdAt AS createdAt,
  coalesce(k.revokedAt, "") AS revokedAt
ORDER BY k.createdAt DESC
`, map[string]any{"appID": strings.TrimSpace(appID)})
		if err != nil {
			return nil, err
		}
		keys := []EngineAPIKey{}
		for rows.Next(ctx) {
			keys = append(keys, engineAPIKeyFromRecord(rows.Record()))
		}
		return keys, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]EngineAPIKey), nil
}

func (s *MemgraphUserStore) VerifyEngineAPIKey(ctx context.Context, prefix string, secretHash string) (EngineAPIKeyVerification, bool, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		record, err := tx.Run(ctx, `
MATCH (a:EngineApp)-[:HAS_ENGINE_KEY]->(k:EngineAPIKey {prefix: $prefix, secretHash: $secretHash})
WHERE coalesce(a.status, "active") = "active"
  AND coalesce(k.status, "active") = "active"
  AND (coalesce(k.expiresAt, "") = "" OR k.expiresAt > $now)
SET k.lastUsedAt = $now
RETURN
  a.id AS appId,
  a.name AS appName,
  a.environment AS appEnvironment,
  coalesce(a.status, "active") AS appStatus,
  coalesce(a.allowedChains, []) AS appAllowedChains,
  coalesce(a.allowedContracts, []) AS appAllowedContracts,
  coalesce(a.rateLimitPerMinute, 60) AS appRateLimitPerMinute,
  a.createdAt AS appCreatedAt,
  a.updatedAt AS appUpdatedAt,
  k.id AS id,
  k.appId AS keyAppId,
  k.name AS name,
  k.prefix AS prefix,
  k.scopes AS scopes,
  coalesce(k.allowedOrigins, []) AS allowedOrigins,
  coalesce(k.allowedIPs, []) AS allowedIPs,
  coalesce(k.status, "active") AS status,
  coalesce(k.expiresAt, "") AS expiresAt,
  coalesce(k.lastUsedAt, "") AS lastUsedAt,
  k.createdAt AS createdAt,
  coalesce(k.revokedAt, "") AS revokedAt
`, map[string]any{"prefix": strings.TrimSpace(prefix), "secretHash": strings.TrimSpace(secretHash), "now": now})
		if err != nil {
			return nil, err
		}
		if record.Next(ctx) {
			return EngineAPIKeyVerification{
				App: engineAppFromPrefixedRecord(record.Record(), "app"),
				Key: engineAPIKeyFromRecordWithAppID(record.Record(), "keyAppId"),
			}, nil
		}
		return nil, record.Err()
	})
	if err != nil {
		return EngineAPIKeyVerification{}, false, err
	}
	if result == nil {
		return EngineAPIKeyVerification{}, false, nil
	}
	return result.(EngineAPIKeyVerification), true, nil
}

func (s *MemgraphUserStore) RevokeEngineAPIKey(ctx context.Context, id string) (EngineAPIKey, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		record, err := tx.Run(ctx, `
MATCH (a:EngineApp)-[:HAS_ENGINE_KEY]->(k:EngineAPIKey {id: $id})
SET k.status = "revoked", k.revokedAt = $now
RETURN
  k.id AS id,
  k.appId AS appId,
  a.name AS appName,
  k.name AS name,
  k.prefix AS prefix,
  k.scopes AS scopes,
  coalesce(k.allowedOrigins, []) AS allowedOrigins,
  coalesce(k.allowedIPs, []) AS allowedIPs,
  coalesce(k.status, "active") AS status,
  coalesce(k.expiresAt, "") AS expiresAt,
  coalesce(k.lastUsedAt, "") AS lastUsedAt,
  k.createdAt AS createdAt,
  coalesce(k.revokedAt, "") AS revokedAt
`, map[string]any{"id": strings.TrimSpace(id), "now": now})
		if err != nil {
			return nil, err
		}
		if record.Next(ctx) {
			return engineAPIKeyFromRecord(record.Record()), nil
		}
		if err := record.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("engine api key not found")
	})
	if err != nil {
		return EngineAPIKey{}, err
	}
	return result.(EngineAPIKey), nil
}

func (s *MemgraphUserStore) RecordEngineAPIUsage(ctx context.Context, input EngineAPIUsageInput) error {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		_, err := tx.Run(ctx, `
MATCH (a:EngineApp {id: $appID})
OPTIONAL MATCH (k:EngineAPIKey {id: $keyID})
CREATE (u:EngineAPIUsage {
  id: $id,
  appId: $appID,
  keyId: $keyID,
  method: $method,
  path: $path,
  statusCode: $statusCode,
  ipAddress: $ipAddress,
  userAgent: $userAgent,
  createdAt: $now
})
MERGE (a)-[:HAS_ENGINE_USAGE]->(u)
FOREACH (_ IN CASE WHEN k IS NULL THEN [] ELSE [1] END | MERGE (k)-[:MADE_ENGINE_REQUEST]->(u))
`, map[string]any{
			"id":         uuid.NewString(),
			"appID":      strings.TrimSpace(input.AppID),
			"keyID":      strings.TrimSpace(input.KeyID),
			"method":     strings.TrimSpace(input.Method),
			"path":       strings.TrimSpace(input.Path),
			"statusCode": int64(input.StatusCode),
			"ipAddress":  strings.TrimSpace(input.IPAddress),
			"userAgent":  strings.TrimSpace(input.UserAgent),
			"now":        time.Now().UTC().Format(time.RFC3339),
		})
		return nil, err
	})
	return err
}

func (s *MemgraphUserStore) ListEngineAPIUsage(ctx context.Context, appID string, limit int64) ([]EngineAPIUsage, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:EngineApp {id: $appID})-[:HAS_ENGINE_USAGE]->(u:EngineAPIUsage)
RETURN
  u.id AS id,
  u.appId AS appId,
  u.keyId AS keyId,
  u.method AS method,
  u.path AS path,
  coalesce(u.statusCode, 0) AS statusCode,
  coalesce(u.ipAddress, "") AS ipAddress,
  coalesce(u.userAgent, "") AS userAgent,
  u.createdAt AS createdAt
ORDER BY u.createdAt DESC
LIMIT $limit
`, map[string]any{"appID": strings.TrimSpace(appID), "limit": limit})
		if err != nil {
			return nil, err
		}
		usage := []EngineAPIUsage{}
		for rows.Next(ctx) {
			usage = append(usage, engineAPIUsageFromRecord(rows.Record()))
		}
		return usage, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]EngineAPIUsage), nil
}

func engineAppFromRecord(record *neo4j.Record) EngineApp {
	return EngineApp{
		ID:               stringValue(record, "id"),
		Name:             stringValue(record, "name"),
		Environment:      stringValue(record, "environment"),
		Status:           stringValue(record, "status"),
		AllowedChains:    int64SliceValue(record, "allowedChains"),
		AllowedContracts: stringSliceValue(record, "allowedContracts"),
		RateLimitPerMin:  intValue(record, "rateLimitPerMinute"),
		CreatedAt:        stringValue(record, "createdAt"),
		UpdatedAt:        stringValue(record, "updatedAt"),
	}
}

func engineAppFromPrefixedRecord(record *neo4j.Record, prefix string) EngineApp {
	return EngineApp{
		ID:               stringValue(record, prefix+"Id"),
		Name:             stringValue(record, prefix+"Name"),
		Environment:      stringValue(record, prefix+"Environment"),
		Status:           stringValue(record, prefix+"Status"),
		AllowedChains:    int64SliceValue(record, prefix+"AllowedChains"),
		AllowedContracts: stringSliceValue(record, prefix+"AllowedContracts"),
		RateLimitPerMin:  intValue(record, prefix+"RateLimitPerMinute"),
		CreatedAt:        stringValue(record, prefix+"CreatedAt"),
		UpdatedAt:        stringValue(record, prefix+"UpdatedAt"),
	}
}

func engineAPIKeyFromRecord(record *neo4j.Record) EngineAPIKey {
	return engineAPIKeyFromRecordWithAppID(record, "appId")
}

func engineAPIKeyFromRecordWithAppID(record *neo4j.Record, appIDKey string) EngineAPIKey {
	return EngineAPIKey{
		ID:             stringValue(record, "id"),
		AppID:          stringValue(record, appIDKey),
		AppName:        stringValue(record, "appName"),
		Name:           stringValue(record, "name"),
		Prefix:         stringValue(record, "prefix"),
		Scopes:         stringSliceValue(record, "scopes"),
		AllowedOrigins: stringSliceValue(record, "allowedOrigins"),
		AllowedIPs:     stringSliceValue(record, "allowedIPs"),
		Status:         stringValue(record, "status"),
		ExpiresAt:      stringValue(record, "expiresAt"),
		LastUsedAt:     stringValue(record, "lastUsedAt"),
		CreatedAt:      stringValue(record, "createdAt"),
		RevokedAt:      stringValue(record, "revokedAt"),
	}
}

func engineAPIUsageFromRecord(record *neo4j.Record) EngineAPIUsage {
	return EngineAPIUsage{
		ID:         stringValue(record, "id"),
		AppID:      stringValue(record, "appId"),
		KeyID:      stringValue(record, "keyId"),
		Method:     stringValue(record, "method"),
		Path:       stringValue(record, "path"),
		StatusCode: intValue(record, "statusCode"),
		IPAddress:  stringValue(record, "ipAddress"),
		UserAgent:  stringValue(record, "userAgent"),
		CreatedAt:  stringValue(record, "createdAt"),
	}
}

func normalizeEngineEnvironment(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "production", "live":
		return "production"
	case "test", "testing", "development", "dev":
		return "development"
	default:
		return "development"
	}
}

func normalizeEngineScopes(scopes []string) ([]string, error) {
	normalized := normalizeStringSlice(scopes)
	if len(normalized) == 0 {
		return []string{"transactions:read"}, nil
	}
	for _, scope := range normalized {
		if !validEngineScopes[scope] {
			return nil, errors.New("invalid engine scope: " + scope)
		}
	}
	return normalized, nil
}

func normalizeRateLimit(value int64) int64 {
	if value <= 0 {
		return 60
	}
	if value > 10000 {
		return 10000
	}
	return value
}

func normalizeStringSlice(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		result = append(result, trimmed)
	}
	return result
}

func normalizeInt64Slice(values []int64) []int64 {
	seen := map[int64]bool{}
	result := []int64{}
	for _, value := range values {
		if value <= 0 || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func int64SliceValue(record *neo4j.Record, key string) []int64 {
	value, ok := record.Get(key)
	if !ok || value == nil {
		return []int64{}
	}
	switch typed := value.(type) {
	case []int64:
		return typed
	case []any:
		result := make([]int64, 0, len(typed))
		for _, item := range typed {
			switch number := item.(type) {
			case int64:
				result = append(result, number)
			case int:
				result = append(result, int64(number))
			}
		}
		return result
	default:
		return []int64{}
	}
}
