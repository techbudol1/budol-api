package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

type User struct {
	ID              string `json:"id"`
	WalletAddress   string `json:"walletAddress"`
	PublicAlias     string `json:"publicAlias"`
	WalletOpsUserID string `json:"walletopsUserId,omitempty"`
	ProviderUserID  string `json:"providerUserId,omitempty"`
	AuthProvider    string `json:"authProvider,omitempty"`
	AuthType        string `json:"authType"`
	Email           string `json:"email,omitempty"`
	Phone           string `json:"phone,omitempty"`
	Role            string `json:"role"`
	Status          string `json:"status"`
	WalletCustody   string `json:"walletCustody"`
	Notes           string `json:"notes,omitempty"`
	CreatedAt       string `json:"createdAt"`
	LastLoginAt     string `json:"lastLoginAt"`
	LoginCount      int64  `json:"loginCount"`
	IsNew           bool   `json:"-"`
}

type TokenGrant struct {
	ID             string   `json:"id"`
	UserID         string   `json:"userId"`
	WalletAddress  string   `json:"walletAddress"`
	GrantType      string   `json:"grantType"`
	Status         string   `json:"status"`
	TokenAddress   string   `json:"tokenAddress"`
	ChainID        int64    `json:"chainId"`
	Amount         string   `json:"amount"`
	Quantity       string   `json:"quantity"`
	TransactionIDs []string `json:"transactionIds"`
	Error          string   `json:"error,omitempty"`
	CreatedAt      string   `json:"createdAt"`
	UpdatedAt      string   `json:"updatedAt"`
}

type WalletOpsIdentity struct {
	WalletAddress   string
	WalletOpsUserID string
	AuthProvider    string
	Email           string
	Phone           string
	RawJSON         string
}

type LegacyEmbeddedIdentity struct {
	WalletAddress        string
	LegacyEmbeddedUserID string
	AuthProvider         string
	Email                string
	Phone                string
	RawJSON              string
}

type GoogleManagedIdentity struct {
	Email         string
	GoogleUserID  string
	Metadata      string
	WalletAddress string
}

type SocialManagedIdentity struct {
	AuthProvider   string
	AuthType       string
	Email          string
	Metadata       string
	ProviderUserID string
	WalletAddress  string
}

type ExternalWalletIdentity struct {
	WalletAddress string
	Metadata      string
}

type WalletLoginChallenge struct {
	Address    string
	Nonce      string
	Message    string
	ExpiresAt  string
	CreatedAt  string
	ConsumedAt string
}

type UserStore interface {
	UpsertFromWalletOps(ctx context.Context, identity WalletOpsIdentity) (User, error)
	UpsertFromLegacyEmbedded(ctx context.Context, identity LegacyEmbeddedIdentity) (User, error)
	UpsertExternalWallet(ctx context.Context, identity ExternalWalletIdentity) (User, error)
	UpsertGoogleManaged(ctx context.Context, identity GoogleManagedIdentity) (User, error)
	UpsertSocialManaged(ctx context.Context, identity SocialManagedIdentity) (User, error)
	GetSocialManaged(ctx context.Context, authType string, providerUserID string, email string) (User, bool, error)
	UpdateUserWalletAddress(ctx context.Context, userID string, walletAddress string) (User, error)
	UpdatePublicAlias(ctx context.Context, userID string, publicAlias string) (User, error)
	CreateWalletLoginChallenge(ctx context.Context, address string, nonce string, message string, expiresAt time.Time) (WalletLoginChallenge, error)
	ConsumeWalletLoginChallenge(ctx context.Context, address string, nonce string, now time.Time) (WalletLoginChallenge, bool, error)
	GetByID(ctx context.Context, id string) (User, bool, error)
	CreateTokenGrantIfMissing(ctx context.Context, user User, input TokenGrantInput) (TokenGrant, bool, error)
	ListTokenGrants(ctx context.Context, grantType string, statuses []string, limit int64) ([]TokenGrant, error)
	ReconcileWelcomeTokenGrantTransfer(ctx context.Context, userID string, walletAddress string, amount string, transactionHash string, transferredAt string) (TokenGrant, bool, error)
	UpdateTokenGrantStatus(ctx context.Context, id string, status string, transactionIDs []string, errorMessage string, input *TokenGrantInput) (TokenGrant, error)
	Close(ctx context.Context) error
}

type TokenGrantInput struct {
	GrantType    string
	TokenAddress string
	ChainID      int64
	Amount       string
	Quantity     string
}

type MemgraphUserStore struct {
	driver neo4j.DriverWithContext
}

func NewMemgraphUserStore(ctx context.Context, uri string, username string, password string) (*MemgraphUserStore, error) {
	auth := neo4j.NoAuth()
	if username != "" {
		auth = neo4j.BasicAuth(username, password, "")
	}

	driver, err := neo4j.NewDriverWithContext(uri, auth)
	if err != nil {
		return nil, err
	}
	if err := driver.VerifyConnectivity(ctx); err != nil {
		_ = driver.Close(ctx)
		return nil, err
	}

	return &MemgraphUserStore{driver: driver}, nil
}

func (s *MemgraphUserStore) UpsertFromWalletOps(ctx context.Context, identity WalletOpsIdentity) (User, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	params := map[string]any{
		"newID":           uuid.NewString(),
		"publicAlias":     newPublicAlias(),
		"walletAddress":   identity.WalletAddress,
		"walletopsUserId": identity.WalletOpsUserID,
		"authProvider":    identity.AuthProvider,
		"email":           identity.Email,
		"phone":           identity.Phone,
		"rawWalletOps":    identity.RawJSON,
		"now":             now,
	}

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		record, err := tx.Run(ctx, `
MERGE (u:User {walletAddress: $walletAddress})
ON CREATE SET
  u.id = $newID,
  u.publicAlias = $publicAlias,
  u.createdAt = $now,
  u.loginCount = 0
SET
  u.publicAlias = CASE WHEN coalesce(u.publicAlias, "") = "" THEN $publicAlias ELSE u.publicAlias END,
  u.updatedAt = $now,
  u.lastLoginAt = $now,
  u.loginCount = coalesce(u.loginCount, 0) + 1,
  u.walletopsUserId = $walletopsUserId,
  u.authProvider = $authProvider,
  u.authType = "walletops_social",
  u.walletCustody = "managed",
  u.email = $email,
  u.phone = $phone,
  u.walletopsProfile = $rawWalletOps,
  u.role = coalesce(u.role, "user"),
  u.status = coalesce(u.status, "active"),
  u.notes = coalesce(u.notes, "")
RETURN
  u.id AS id,
  u.walletAddress AS walletAddress,
  coalesce(u.publicAlias, "") AS publicAlias,
  coalesce(u.walletopsUserId, "") AS walletopsUserId,
  coalesce(u.providerUserId, u.googleUserId, u.legacyEmbeddedUserId, "") AS providerUserId,
  coalesce(u.authProvider, "") AS authProvider,
  coalesce(u.authType, "") AS authType,
  coalesce(u.email, "") AS email,
  coalesce(u.phone, "") AS phone,
  coalesce(u.role, "user") AS role,
  coalesce(u.status, "active") AS status,
  coalesce(u.walletCustody, "") AS walletCustody,
  coalesce(u.notes, "") AS notes,
  u.createdAt AS createdAt,
  u.lastLoginAt AS lastLoginAt,
  u.loginCount AS loginCount,
  (u.id = $newID) AS isNew
`, params)
		if err != nil {
			return nil, err
		}

		if record.Next(ctx) {
			return userFromRecord(record.Record()), nil
		}
		return nil, record.Err()
	})
	if err != nil {
		return User{}, err
	}

	return result.(User), nil
}

func (s *MemgraphUserStore) UpsertFromLegacyEmbedded(ctx context.Context, identity LegacyEmbeddedIdentity) (User, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	params := map[string]any{
		"newID":                uuid.NewString(),
		"publicAlias":          newPublicAlias(),
		"walletAddress":        identity.WalletAddress,
		"legacyEmbeddedUserId": identity.LegacyEmbeddedUserID,
		"authProvider":         identity.AuthProvider,
		"email":                identity.Email,
		"phone":                identity.Phone,
		"rawLegacyEmbedded":    identity.RawJSON,
		"now":                  now,
	}

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		record, err := tx.Run(ctx, `
MERGE (u:User {walletAddress: $walletAddress})
ON CREATE SET
  u.id = $newID,
  u.publicAlias = $publicAlias,
  u.createdAt = $now,
  u.loginCount = 0
SET
  u.publicAlias = CASE WHEN coalesce(u.publicAlias, "") = "" THEN $publicAlias ELSE u.publicAlias END,
  u.updatedAt = $now,
  u.lastLoginAt = $now,
  u.loginCount = coalesce(u.loginCount, 0) + 1,
  u.legacyEmbeddedUserId = $legacyEmbeddedUserId,
  u.walletopsUserId = "",
  u.authProvider = $authProvider,
  u.authType = "embedded_oauth",
  u.walletCustody = "managed",
  u.email = $email,
  u.phone = $phone,
  u.legacyEmbeddedProfile = $rawLegacyEmbedded,
  u.role = coalesce(u.role, "user"),
  u.status = coalesce(u.status, "active"),
  u.notes = coalesce(u.notes, "")
RETURN
  u.id AS id,
  u.walletAddress AS walletAddress,
  coalesce(u.publicAlias, "") AS publicAlias,
  coalesce(u.walletopsUserId, "") AS walletopsUserId,
  coalesce(u.providerUserId, u.googleUserId, u.legacyEmbeddedUserId, "") AS providerUserId,
  coalesce(u.authProvider, "") AS authProvider,
  coalesce(u.authType, "") AS authType,
  coalesce(u.email, "") AS email,
  coalesce(u.phone, "") AS phone,
  coalesce(u.role, "user") AS role,
  coalesce(u.status, "active") AS status,
  coalesce(u.walletCustody, "") AS walletCustody,
  coalesce(u.notes, "") AS notes,
  u.createdAt AS createdAt,
  u.lastLoginAt AS lastLoginAt,
  u.loginCount AS loginCount,
  (u.id = $newID) AS isNew
`, params)
		if err != nil {
			return nil, err
		}

		if record.Next(ctx) {
			return userFromRecord(record.Record()), nil
		}
		return nil, record.Err()
	})
	if err != nil {
		return User{}, err
	}

	return result.(User), nil
}

func (s *MemgraphUserStore) UpsertGoogleManaged(ctx context.Context, identity GoogleManagedIdentity) (User, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	params := map[string]any{
		"newID":         uuid.NewString(),
		"publicAlias":   newPublicAlias(),
		"walletAddress": identity.WalletAddress,
		"googleUserId":  identity.GoogleUserID,
		"email":         identity.Email,
		"metadata":      identity.Metadata,
		"now":           now,
	}

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		record, err := tx.Run(ctx, `
MERGE (u:User {authType: "google_oauth", email: $email})
ON CREATE SET
  u.id = $newID,
  u.publicAlias = $publicAlias,
  u.createdAt = $now,
  u.loginCount = 0
SET
  u.publicAlias = CASE WHEN coalesce(u.publicAlias, "") = "" THEN $publicAlias ELSE u.publicAlias END,
  u.updatedAt = $now,
  u.lastLoginAt = $now,
  u.loginCount = coalesce(u.loginCount, 0) + 1,
  u.walletAddress = CASE WHEN $walletAddress <> "" THEN $walletAddress ELSE coalesce(u.walletAddress, "") END,
  u.googleUserId = $googleUserId,
  u.walletopsUserId = "",
  u.authProvider = "google",
  u.authType = "google_oauth",
  u.walletCustody = "managed",
  u.email = $email,
  u.phone = "",
  u.oauthProfile = $metadata,
  u.role = coalesce(u.role, "user"),
  u.status = coalesce(u.status, "active"),
  u.notes = coalesce(u.notes, "")
RETURN
  u.id AS id,
  u.walletAddress AS walletAddress,
  coalesce(u.publicAlias, "") AS publicAlias,
  coalesce(u.walletopsUserId, "") AS walletopsUserId,
  coalesce(u.providerUserId, u.googleUserId, u.legacyEmbeddedUserId, "") AS providerUserId,
  coalesce(u.authProvider, "") AS authProvider,
  coalesce(u.authType, "") AS authType,
  coalesce(u.email, "") AS email,
  coalesce(u.phone, "") AS phone,
  coalesce(u.role, "user") AS role,
  coalesce(u.status, "active") AS status,
  coalesce(u.walletCustody, "") AS walletCustody,
  coalesce(u.notes, "") AS notes,
  u.createdAt AS createdAt,
  u.lastLoginAt AS lastLoginAt,
  u.loginCount AS loginCount,
  (u.id = $newID) AS isNew
`, params)
		if err != nil {
			return nil, err
		}
		if record.Next(ctx) {
			return userFromRecord(record.Record()), nil
		}
		return nil, record.Err()
	})
	if err != nil {
		return User{}, err
	}
	return result.(User), nil
}

func (s *MemgraphUserStore) UpsertSocialManaged(ctx context.Context, identity SocialManagedIdentity) (User, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	params := map[string]any{
		"authProvider":   identity.AuthProvider,
		"authType":       identity.AuthType,
		"email":          identity.Email,
		"metadata":       identity.Metadata,
		"newID":          uuid.NewString(),
		"now":            now,
		"publicAlias":    newPublicAlias(),
		"providerUserID": identity.ProviderUserID,
		"walletAddress":  identity.WalletAddress,
	}

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		record, err := tx.Run(ctx, `
MERGE (u:User {authType: $authType, providerUserId: $providerUserID})
ON CREATE SET
  u.id = $newID,
  u.publicAlias = $publicAlias,
  u.createdAt = $now,
  u.loginCount = 0
SET
  u.publicAlias = CASE WHEN coalesce(u.publicAlias, "") = "" THEN $publicAlias ELSE u.publicAlias END,
  u.updatedAt = $now,
  u.lastLoginAt = $now,
  u.loginCount = coalesce(u.loginCount, 0) + 1,
  u.walletAddress = CASE WHEN $walletAddress <> "" THEN $walletAddress ELSE coalesce(u.walletAddress, "") END,
  u.providerUserId = $providerUserID,
  u.walletopsUserId = "",
  u.authProvider = $authProvider,
  u.authType = $authType,
  u.walletCustody = "managed",
  u.email = $email,
  u.phone = "",
  u.oauthProfile = $metadata,
  u.role = coalesce(u.role, "user"),
  u.status = coalesce(u.status, "active"),
  u.notes = coalesce(u.notes, "")
RETURN
  u.id AS id,
  u.walletAddress AS walletAddress,
  coalesce(u.publicAlias, "") AS publicAlias,
  coalesce(u.walletopsUserId, "") AS walletopsUserId,
  coalesce(u.providerUserId, u.googleUserId, u.legacyEmbeddedUserId, "") AS providerUserId,
  coalesce(u.authProvider, "") AS authProvider,
  coalesce(u.authType, "") AS authType,
  coalesce(u.email, "") AS email,
  coalesce(u.phone, "") AS phone,
  coalesce(u.role, "user") AS role,
  coalesce(u.status, "active") AS status,
  coalesce(u.walletCustody, "") AS walletCustody,
  coalesce(u.notes, "") AS notes,
  u.createdAt AS createdAt,
  u.lastLoginAt AS lastLoginAt,
  u.loginCount AS loginCount,
  (u.id = $newID) AS isNew
`, params)
		if err != nil {
			return nil, err
		}
		if record.Next(ctx) {
			return userFromRecord(record.Record()), nil
		}
		return nil, record.Err()
	})
	if err != nil {
		return User{}, err
	}
	return result.(User), nil
}

func (s *MemgraphUserStore) GetSocialManaged(ctx context.Context, authType string, providerUserID string, email string) (User, bool, error) {
	params := map[string]any{
		"authType":       authType,
		"email":          email,
		"providerUserID": providerUserID,
	}

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		record, err := tx.Run(ctx, `
MATCH (u:User)
WHERE u.authType = $authType
  AND (
    ($providerUserID <> "" AND u.providerUserId = $providerUserID)
    OR ($email <> "" AND toLower(coalesce(u.email, "")) = toLower($email))
  )
RETURN
  u.id AS id,
  u.walletAddress AS walletAddress,
  coalesce(u.publicAlias, "") AS publicAlias,
  coalesce(u.walletopsUserId, "") AS walletopsUserId,
  coalesce(u.providerUserId, u.googleUserId, u.legacyEmbeddedUserId, "") AS providerUserId,
  coalesce(u.authProvider, "") AS authProvider,
  coalesce(u.authType, "") AS authType,
  coalesce(u.email, "") AS email,
  coalesce(u.phone, "") AS phone,
  coalesce(u.role, "user") AS role,
  coalesce(u.status, "active") AS status,
  coalesce(u.walletCustody, "") AS walletCustody,
  coalesce(u.notes, "") AS notes,
  u.createdAt AS createdAt,
  u.lastLoginAt AS lastLoginAt,
  u.loginCount AS loginCount,
  false AS isNew
LIMIT 1
`, params)
		if err != nil {
			return nil, err
		}
		if record.Next(ctx) {
			return userFromRecord(record.Record()), nil
		}
		return nil, record.Err()
	})
	if err != nil {
		return User{}, false, err
	}
	if result == nil {
		return User{}, false, nil
	}
	return result.(User), true, nil
}

func (s *MemgraphUserStore) UpsertExternalWallet(ctx context.Context, identity ExternalWalletIdentity) (User, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	params := map[string]any{
		"newID":         uuid.NewString(),
		"publicAlias":   newPublicAlias(),
		"walletAddress": identity.WalletAddress,
		"metadata":      identity.Metadata,
		"now":           now,
	}

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		record, err := tx.Run(ctx, `
MERGE (u:User {walletAddress: $walletAddress})
ON CREATE SET
  u.id = $newID,
  u.publicAlias = $publicAlias,
  u.createdAt = $now,
  u.loginCount = 0
SET
  u.publicAlias = CASE WHEN coalesce(u.publicAlias, "") = "" THEN $publicAlias ELSE u.publicAlias END,
  u.updatedAt = $now,
  u.lastLoginAt = $now,
  u.loginCount = coalesce(u.loginCount, 0) + 1,
  u.walletAddress = $walletAddress,
  u.walletopsUserId = "",
  u.authProvider = "evm_wallet",
  u.authType = "evm_wallet",
  u.walletCustody = "external",
  u.email = coalesce(u.email, ""),
  u.phone = coalesce(u.phone, ""),
  u.walletLoginProfile = $metadata,
  u.role = coalesce(u.role, "user"),
  u.status = coalesce(u.status, "active"),
  u.notes = coalesce(u.notes, "")
RETURN
  u.id AS id,
  u.walletAddress AS walletAddress,
  coalesce(u.publicAlias, "") AS publicAlias,
  coalesce(u.walletopsUserId, "") AS walletopsUserId,
  coalesce(u.providerUserId, u.googleUserId, u.legacyEmbeddedUserId, "") AS providerUserId,
  coalesce(u.authProvider, "") AS authProvider,
  coalesce(u.authType, "") AS authType,
  coalesce(u.email, "") AS email,
  coalesce(u.phone, "") AS phone,
  coalesce(u.role, "user") AS role,
  coalesce(u.status, "active") AS status,
  coalesce(u.walletCustody, "") AS walletCustody,
  coalesce(u.notes, "") AS notes,
  u.createdAt AS createdAt,
  u.lastLoginAt AS lastLoginAt,
  u.loginCount AS loginCount,
  (u.id = $newID) AS isNew
`, params)
		if err != nil {
			return nil, err
		}
		if record.Next(ctx) {
			return userFromRecord(record.Record()), nil
		}
		return nil, record.Err()
	})
	if err != nil {
		return User{}, err
	}
	return result.(User), nil
}

func (s *MemgraphUserStore) CreateWalletLoginChallenge(ctx context.Context, address string, nonce string, message string, expiresAt time.Time) (WalletLoginChallenge, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	params := map[string]any{
		"address":   address,
		"nonce":     nonce,
		"message":   message,
		"expiresAt": expiresAt.UTC().Format(time.RFC3339),
		"now":       now,
	}

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
CREATE (c:WalletLoginChallenge {
  address: $address,
  nonce: $nonce,
  message: $message,
  expiresAt: $expiresAt,
  createdAt: $now,
  consumedAt: ""
})
RETURN c.address AS address, c.nonce AS nonce, c.message AS message, c.expiresAt AS expiresAt, c.createdAt AS createdAt, c.consumedAt AS consumedAt
`, params)
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return walletLoginChallengeFromRecord(rows.Record()), nil
		}
		return nil, rows.Err()
	})
	if err != nil {
		return WalletLoginChallenge{}, err
	}
	return result.(WalletLoginChallenge), nil
}

func (s *MemgraphUserStore) ConsumeWalletLoginChallenge(ctx context.Context, address string, nonce string, now time.Time) (WalletLoginChallenge, bool, error) {
	params := map[string]any{
		"address": address,
		"nonce":   nonce,
		"now":     now.UTC().Format(time.RFC3339),
	}

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (c:WalletLoginChallenge {address: $address, nonce: $nonce})
WHERE coalesce(c.consumedAt, "") = "" AND c.expiresAt >= $now
SET c.consumedAt = $now
RETURN c.address AS address, c.nonce AS nonce, c.message AS message, c.expiresAt AS expiresAt, c.createdAt AS createdAt, c.consumedAt AS consumedAt
`, params)
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return walletLoginChallengeFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, nil
	})
	if err != nil {
		return WalletLoginChallenge{}, false, err
	}
	if result == nil {
		return WalletLoginChallenge{}, false, nil
	}
	return result.(WalletLoginChallenge), true, nil
}

func (s *MemgraphUserStore) Close(ctx context.Context) error {
	return s.driver.Close(ctx)
}

func (s *MemgraphUserStore) UpdateUserWalletAddress(ctx context.Context, userID string, walletAddress string) (User, error) {
	userID = strings.TrimSpace(userID)
	walletAddress = strings.TrimSpace(walletAddress)
	if userID == "" {
		return User{}, errors.New("user id is required")
	}
	if walletAddress == "" {
		return User{}, errors.New("wallet address is required")
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		record, err := tx.Run(ctx, `
MATCH (u:User {id: $id})
SET u.walletAddress = $walletAddress,
    u.publicAlias = CASE WHEN coalesce(u.publicAlias, "") = "" THEN $publicAlias ELSE u.publicAlias END,
    u.updatedAt = $now
RETURN
  u.id AS id,
  u.walletAddress AS walletAddress,
  coalesce(u.publicAlias, "") AS publicAlias,
  coalesce(u.walletopsUserId, "") AS walletopsUserId,
  coalesce(u.providerUserId, u.googleUserId, u.legacyEmbeddedUserId, "") AS providerUserId,
  coalesce(u.authProvider, "") AS authProvider,
  coalesce(u.authType, "") AS authType,
  coalesce(u.email, "") AS email,
  coalesce(u.phone, "") AS phone,
  coalesce(u.role, "user") AS role,
  coalesce(u.status, "active") AS status,
  coalesce(u.walletCustody, "") AS walletCustody,
  coalesce(u.notes, "") AS notes,
  u.createdAt AS createdAt,
  u.lastLoginAt AS lastLoginAt,
  u.loginCount AS loginCount,
  false AS isNew
`, map[string]any{
			"id":            userID,
			"now":           time.Now().UTC().Format(time.RFC3339),
			"publicAlias":   newPublicAlias(),
			"walletAddress": walletAddress,
		})
		if err != nil {
			return nil, err
		}
		if record.Next(ctx) {
			return userFromRecord(record.Record()), nil
		}
		if err := record.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("user not found")
	})
	if err != nil {
		return User{}, err
	}
	return result.(User), nil
}

func (s *MemgraphUserStore) UpdatePublicAlias(ctx context.Context, userID string, publicAlias string) (User, error) {
	userID = strings.TrimSpace(userID)
	publicAlias = strings.TrimSpace(publicAlias)
	if userID == "" {
		return User{}, errors.New("user id is required")
	}
	if publicAlias == "" {
		return User{}, errors.New("display name is required")
	}

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (u:User {id: $id})
OPTIONAL MATCH (other:User)
WHERE other.id <> $id AND toLower(trim(coalesce(other.publicAlias, ""))) = $publicAliasKey
WITH u, count(other) AS matchingUsers
WHERE matchingUsers = 0
SET u.publicAlias = $publicAlias,
    u.updatedAt = $now
RETURN
  u.id AS id,
  u.walletAddress AS walletAddress,
  coalesce(u.publicAlias, "") AS publicAlias,
  coalesce(u.walletopsUserId, "") AS walletopsUserId,
  coalesce(u.providerUserId, u.googleUserId, u.legacyEmbeddedUserId, "") AS providerUserId,
  coalesce(u.authProvider, "") AS authProvider,
  coalesce(u.authType, "") AS authType,
  coalesce(u.email, "") AS email,
  coalesce(u.phone, "") AS phone,
  coalesce(u.role, "user") AS role,
  coalesce(u.status, "active") AS status,
  coalesce(u.walletCustody, "") AS walletCustody,
  coalesce(u.notes, "") AS notes,
  u.createdAt AS createdAt,
  u.lastLoginAt AS lastLoginAt,
  u.loginCount AS loginCount,
  false AS isNew
`, map[string]any{
			"id":             userID,
			"now":            time.Now().UTC().Format(time.RFC3339),
			"publicAlias":    publicAlias,
			"publicAliasKey": strings.ToLower(publicAlias),
		})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return userFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("display name is already in use")
	})
	if err != nil {
		return User{}, err
	}
	return result.(User), nil
}

func (s *MemgraphUserStore) GetByID(ctx context.Context, id string) (User, bool, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		record, err := tx.Run(ctx, `
MATCH (u:User {id: $id})
RETURN
  u.id AS id,
  u.walletAddress AS walletAddress,
  coalesce(u.publicAlias, "") AS publicAlias,
  coalesce(u.walletopsUserId, "") AS walletopsUserId,
  coalesce(u.providerUserId, u.googleUserId, u.legacyEmbeddedUserId, "") AS providerUserId,
  coalesce(u.authProvider, "") AS authProvider,
  coalesce(u.authType, "") AS authType,
  coalesce(u.email, "") AS email,
  coalesce(u.phone, "") AS phone,
  coalesce(u.role, "user") AS role,
  coalesce(u.status, "active") AS status,
  coalesce(u.walletCustody, "") AS walletCustody,
  coalesce(u.notes, "") AS notes,
  u.createdAt AS createdAt,
  u.lastLoginAt AS lastLoginAt,
  u.loginCount AS loginCount
`, map[string]any{"id": id})
		if err != nil {
			return nil, err
		}
		if record.Next(ctx) {
			return userFromRecord(record.Record()), nil
		}
		if err := record.Err(); err != nil {
			return nil, err
		}
		return nil, nil
	})
	if err != nil {
		return User{}, false, err
	}
	if result == nil {
		return User{}, false, nil
	}
	return result.(User), true, nil
}

func userFromRecord(record *neo4j.Record) User {
	return User{
		ID:              stringValue(record, "id"),
		WalletAddress:   stringValue(record, "walletAddress"),
		PublicAlias:     stringValue(record, "publicAlias"),
		WalletOpsUserID: stringValue(record, "walletopsUserId"),
		ProviderUserID:  stringValue(record, "providerUserId"),
		AuthProvider:    stringValue(record, "authProvider"),
		AuthType:        stringValue(record, "authType"),
		Email:           stringValue(record, "email"),
		Phone:           stringValue(record, "phone"),
		Role:            stringValue(record, "role"),
		Status:          stringValue(record, "status"),
		WalletCustody:   stringValue(record, "walletCustody"),
		Notes:           stringValue(record, "notes"),
		CreatedAt:       stringValue(record, "createdAt"),
		LastLoginAt:     stringValue(record, "lastLoginAt"),
		LoginCount:      intValue(record, "loginCount"),
		IsNew:           boolValue(record, "isNew"),
	}
}

func walletLoginChallengeFromRecord(record *neo4j.Record) WalletLoginChallenge {
	return WalletLoginChallenge{
		Address:    stringValue(record, "address"),
		Nonce:      stringValue(record, "nonce"),
		Message:    stringValue(record, "message"),
		ExpiresAt:  stringValue(record, "expiresAt"),
		CreatedAt:  stringValue(record, "createdAt"),
		ConsumedAt: stringValue(record, "consumedAt"),
	}
}

func (s *MemgraphUserStore) CreateTokenGrantIfMissing(ctx context.Context, user User, input TokenGrantInput) (TokenGrant, bool, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	params := map[string]any{
		"newID":         uuid.NewString(),
		"userID":        user.ID,
		"walletAddress": user.WalletAddress,
		"grantType":     input.GrantType,
		"tokenAddress":  input.TokenAddress,
		"chainID":       input.ChainID,
		"amount":        input.Amount,
		"quantity":      input.Quantity,
		"now":           now,
	}

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (u:User {id: $userID})
MERGE (g:TokenGrant {userId: $userID, grantType: $grantType})
ON CREATE SET
  g.id = $newID,
  g.walletAddress = $walletAddress,
  g.status = "pending",
  g.tokenAddress = $tokenAddress,
  g.chainId = $chainID,
  g.amount = $amount,
  g.quantity = $quantity,
  g.transactionIds = [],
  g.error = "",
  g.createdAt = $now
SET g.updatedAt = $now
MERGE (u)-[:RECEIVED_TOKEN_GRANT]->(g)
RETURN
  g.id AS id,
  g.userId AS userId,
  g.walletAddress AS walletAddress,
  g.grantType AS grantType,
  g.status AS status,
  g.tokenAddress AS tokenAddress,
  g.chainId AS chainId,
  g.amount AS amount,
  g.quantity AS quantity,
  g.transactionIds AS transactionIds,
  coalesce(g.error, "") AS error,
  g.createdAt AS createdAt,
  g.updatedAt AS updatedAt,
  (g.id = $newID) AS created
`, params)
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			record := rows.Record()
			return struct {
				grant   TokenGrant
				created bool
			}{
				grant:   tokenGrantFromRecord(record),
				created: boolValue(record, "created"),
			}, nil
		}
		return nil, rows.Err()
	})
	if err != nil {
		return TokenGrant{}, false, err
	}
	typed := result.(struct {
		grant   TokenGrant
		created bool
	})
	return typed.grant, typed.created, nil
}

func (s *MemgraphUserStore) UpdateTokenGrantStatus(ctx context.Context, id string, status string, transactionIDs []string, errorMessage string, input *TokenGrantInput) (TokenGrant, error) {
	tokenAddress := ""
	chainID := int64(0)
	amount := ""
	quantity := ""
	if input != nil {
		tokenAddress = input.TokenAddress
		chainID = input.ChainID
		amount = input.Amount
		quantity = input.Quantity
	}
	params := map[string]any{
		"id":             id,
		"status":         status,
		"transactionIDs": transactionIDs,
		"error":          errorMessage,
		"tokenAddress":   tokenAddress,
		"chainID":        chainID,
		"amount":         amount,
		"quantity":       quantity,
		"updateDetails":  input != nil,
		"now":            time.Now().UTC().Format(time.RFC3339),
	}

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (g:TokenGrant {id: $id})
SET
  g.status = $status,
  g.transactionIds = $transactionIDs,
  g.error = $error,
  g.tokenAddress = CASE WHEN $updateDetails THEN $tokenAddress ELSE g.tokenAddress END,
  g.chainId = CASE WHEN $updateDetails THEN $chainID ELSE g.chainId END,
  g.amount = CASE WHEN $updateDetails THEN $amount ELSE g.amount END,
  g.quantity = CASE WHEN $updateDetails THEN $quantity ELSE g.quantity END,
  g.updatedAt = $now
RETURN
  g.id AS id,
  g.userId AS userId,
  g.walletAddress AS walletAddress,
  g.grantType AS grantType,
  g.status AS status,
  g.tokenAddress AS tokenAddress,
  g.chainId AS chainId,
  g.amount AS amount,
  g.quantity AS quantity,
  g.transactionIds AS transactionIds,
  coalesce(g.error, "") AS error,
  g.createdAt AS createdAt,
  g.updatedAt AS updatedAt
`, params)
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return tokenGrantFromRecord(rows.Record()), nil
		}
		return nil, rows.Err()
	})
	if err != nil {
		return TokenGrant{}, err
	}
	return result.(TokenGrant), nil
}

func (s *MemgraphUserStore) ReconcileWelcomeTokenGrantTransfer(ctx context.Context, userID string, walletAddress string, amount string, transactionHash string, transferredAt string) (TokenGrant, bool, error) {
	params := map[string]any{
		"userID":          strings.TrimSpace(userID),
		"walletAddress":   strings.ToLower(strings.TrimSpace(walletAddress)),
		"amount":          strings.TrimSpace(amount),
		"transactionHash": strings.ToLower(strings.TrimSpace(transactionHash)),
		"transferredAt":   strings.TrimSpace(transferredAt),
		"now":             time.Now().UTC().Format(time.RFC3339),
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:User {id: $userID})-[:RECEIVED_TOKEN_GRANT]->(g:TokenGrant {grantType: "welcome"})
WHERE toLower(coalesce(g.walletAddress, "")) = $walletAddress
  AND coalesce(g.amount, "") = $amount
  AND (
    (g.status = "sent" AND $transactionHash IN coalesce(g.transactionIds, []))
    OR (
      g.status IN ["pending", "failed", "submitted"]
      AND ($transferredAt = "" OR g.createdAt <= $transferredAt)
    )
  )
WITH g, g.status = "sent" AS alreadyTracked
ORDER BY g.updatedAt DESC
LIMIT 1
SET
  g.status = "sent",
  g.transactionIds = CASE WHEN alreadyTracked THEN g.transactionIds ELSE [$transactionHash] END,
  g.error = "",
  g.updatedAt = CASE WHEN alreadyTracked THEN g.updatedAt ELSE $now END
RETURN
  g.id AS id,
  g.userId AS userId,
  g.walletAddress AS walletAddress,
  g.grantType AS grantType,
  g.status AS status,
  g.tokenAddress AS tokenAddress,
  g.chainId AS chainId,
  g.amount AS amount,
  g.quantity AS quantity,
  coalesce(g.transactionIds, []) AS transactionIds,
  coalesce(g.error, "") AS error,
  g.createdAt AS createdAt,
  g.updatedAt AS updatedAt,
  alreadyTracked
`, params)
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			record := rows.Record()
			return struct {
				grant          TokenGrant
				alreadyTracked bool
			}{
				grant:          tokenGrantFromRecord(record),
				alreadyTracked: boolValue(record, "alreadyTracked"),
			}, nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, nil
	})
	if err != nil {
		return TokenGrant{}, false, err
	}
	if result == nil {
		return TokenGrant{}, false, nil
	}
	typed := result.(struct {
		grant          TokenGrant
		alreadyTracked bool
	})
	return typed.grant, true, nil
}

func (s *MemgraphUserStore) ListTokenGrants(ctx context.Context, grantType string, statuses []string, limit int64) ([]TokenGrant, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	params := map[string]any{
		"grantType": grantType,
		"statuses":  statuses,
		"limit":     limit,
	}

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (g:TokenGrant)
WHERE ($grantType = "" OR g.grantType = $grantType)
  AND (size($statuses) = 0 OR g.status IN $statuses)
RETURN
  g.id AS id,
  g.userId AS userId,
  g.walletAddress AS walletAddress,
  g.grantType AS grantType,
  g.status AS status,
  g.tokenAddress AS tokenAddress,
  g.chainId AS chainId,
  g.amount AS amount,
  g.quantity AS quantity,
  coalesce(g.transactionIds, []) AS transactionIds,
  coalesce(g.error, "") AS error,
  g.createdAt AS createdAt,
  g.updatedAt AS updatedAt
ORDER BY g.updatedAt DESC
LIMIT $limit
`, params)
		if err != nil {
			return nil, err
		}
		grants := []TokenGrant{}
		for rows.Next(ctx) {
			grants = append(grants, tokenGrantFromRecord(rows.Record()))
		}
		return grants, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]TokenGrant), nil
}

func tokenGrantFromRecord(record *neo4j.Record) TokenGrant {
	return TokenGrant{
		ID:             stringValue(record, "id"),
		UserID:         stringValue(record, "userId"),
		WalletAddress:  stringValue(record, "walletAddress"),
		GrantType:      stringValue(record, "grantType"),
		Status:         stringValue(record, "status"),
		TokenAddress:   stringValue(record, "tokenAddress"),
		ChainID:        intValue(record, "chainId"),
		Amount:         stringValue(record, "amount"),
		Quantity:       stringValue(record, "quantity"),
		TransactionIDs: stringSliceValue(record, "transactionIds"),
		Error:          stringValue(record, "error"),
		CreatedAt:      stringValue(record, "createdAt"),
		UpdatedAt:      stringValue(record, "updatedAt"),
	}
}

func stringValue(record *neo4j.Record, key string) string {
	value, ok := record.Get(key)
	if !ok || value == nil {
		return ""
	}
	if typed, ok := value.(string); ok {
		return typed
	}
	return ""
}

func intValue(record *neo4j.Record, key string) int64 {
	value, ok := record.Get(key)
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case int64:
		return typed
	case int:
		return int64(typed)
	default:
		return 0
	}
}

func stringSliceValue(record *neo4j.Record, key string) []string {
	value, ok := record.Get(key)
	if !ok || value == nil {
		return []string{}
	}
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				result = append(result, text)
			}
		}
		return result
	default:
		return []string{}
	}
}
