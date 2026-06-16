package privy

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"budol/server/internal/store"

	"github.com/golang-jwt/jwt/v5"
)

type Client struct {
	apiBase         string
	appID           string
	appSecret       string
	verificationKey string
	httpClient      *http.Client
}

type VerifiedClaims struct {
	UserID    string
	SessionID string
}

func NewClient(apiBase string, appID string, appSecret string, verificationKey string) *Client {
	return &Client{
		apiBase:         strings.TrimRight(apiBase, "/"),
		appID:           strings.TrimSpace(appID),
		appSecret:       strings.TrimSpace(appSecret),
		verificationKey: strings.TrimSpace(verificationKey),
		httpClient:      &http.Client{Timeout: 12 * time.Second},
	}
}

func (c *Client) VerifyAccessToken(accessToken string) (VerifiedClaims, error) {
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return VerifiedClaims{}, errors.New("missing Privy access token")
	}
	if c.appID == "" {
		return VerifiedClaims{}, errors.New("PRIVY_APP_ID is not configured")
	}
	key, err := parseVerificationKey(c.verificationKey)
	if err != nil {
		return VerifiedClaims{}, fmt.Errorf("invalid PRIVY_VERIFICATION_KEY: %w", err)
	}

	claims := jwt.RegisteredClaims{}
	parsed, err := jwt.ParseWithClaims(accessToken, &claims, func(token *jwt.Token) (any, error) {
		switch token.Method.Alg() {
		case jwt.SigningMethodES256.Alg(), jwt.SigningMethodEdDSA.Alg():
			return key, nil
		default:
			return nil, fmt.Errorf("unexpected Privy token signing method %s", token.Method.Alg())
		}
	}, jwt.WithIssuer("privy.io"), jwt.WithAudience(c.appID))
	if err != nil {
		return VerifiedClaims{}, err
	}
	if !parsed.Valid {
		return VerifiedClaims{}, errors.New("invalid Privy access token")
	}
	if claims.Subject == "" {
		return VerifiedClaims{}, errors.New("Privy access token is missing user subject")
	}

	sessionID := ""
	if rawClaims, _, err := jwt.NewParser().ParseUnverified(accessToken, jwt.MapClaims{}); err == nil {
		if mapClaims, ok := rawClaims.Claims.(jwt.MapClaims); ok {
			sessionID, _ = mapClaims["sid"].(string)
		}
	}

	return VerifiedClaims{
		UserID:    claims.Subject,
		SessionID: sessionID,
	}, nil
}

func (c *Client) UserIdentity(ctx context.Context, userID string) (store.PrivyIdentity, error) {
	if c.appID == "" || c.appSecret == "" {
		return store.PrivyIdentity{}, errors.New("PRIVY_APP_ID and PRIVY_APP_SECRET are required")
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return store.PrivyIdentity{}, errors.New("missing Privy user ID")
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiBase+"/v1/users/"+url.PathEscape(userID), nil)
	if err != nil {
		return store.PrivyIdentity{}, err
	}
	request.SetBasicAuth(c.appID, c.appSecret)
	request.Header.Set("privy-app-id", c.appID)
	request.Header.Set("Accept", "application/json")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return store.PrivyIdentity{}, err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return store.PrivyIdentity{}, fmt.Errorf("Privy user lookup failed: status %d", response.StatusCode)
	}

	var payload map[string]any
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return store.PrivyIdentity{}, fmt.Errorf("invalid Privy user response: %w", err)
	}
	raw, _ := json.Marshal(payload)

	identity := store.PrivyIdentity{
		PrivyUserID:  stringValue(payload, "id"),
		AuthProvider: "privy",
		RawJSON:      string(raw),
	}
	if identity.PrivyUserID == "" {
		identity.PrivyUserID = userID
	}

	if linked, ok := payload["linked_accounts"].([]any); ok {
		for _, item := range linked {
			account, ok := item.(map[string]any)
			if !ok {
				continue
			}
			accountType := stringValue(account, "type")
			switch accountType {
			case "wallet":
				if identity.WalletAddress == "" && strings.EqualFold(stringValue(account, "chain_type"), "ethereum") {
					identity.WalletAddress = firstNonEmpty(stringValue(account, "address"), stringValue(account, "wallet_address"))
				}
			case "google_oauth":
				identity.AuthProvider = "google"
				identity.Email = firstNonEmpty(identity.Email, stringValue(account, "email"))
			case "email":
				identity.Email = firstNonEmpty(identity.Email, stringValue(account, "address"))
			case "phone":
				identity.Phone = firstNonEmpty(identity.Phone, stringValue(account, "number"))
			}
		}
	}

	if identity.WalletAddress == "" {
		return store.PrivyIdentity{}, errors.New("Privy user does not have an embedded Ethereum wallet yet")
	}

	return identity, nil
}

func parseVerificationKey(value string) (any, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, errors.New("copy the verification key from Privy dashboard App settings > Basics")
	}
	block, _ := pem.Decode([]byte(value))
	if block == nil {
		decoded, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			return nil, errors.New("expected PEM public key")
		}
		block = &pem.Block{Bytes: decoded}
	}
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	switch publicKey := key.(type) {
	case *ecdsa.PublicKey:
		return publicKey, nil
	case ed25519.PublicKey:
		return publicKey, nil
	default:
		return nil, fmt.Errorf("unsupported public key type %T", publicKey)
	}
}

func stringValue(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := payload[key].(string); ok {
			trimmed := strings.TrimSpace(value)
			if trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
