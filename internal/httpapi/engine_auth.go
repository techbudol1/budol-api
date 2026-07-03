package httpapi

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/techbudol1/budol-api/internal/store"

	"github.com/gofiber/fiber/v2"
)

const engineAPIKeyHeader = "X-Budol-Engine-Key"
const enginePrincipalLocalKey = "budol_engine_principal"

var defaultEngineScopes = []string{
	"transactions:write",
	"transactions:read",
	"wallets:read",
	"wallets:write",
	"contracts:read",
	"contracts:write",
}

type enginePrincipal struct {
	App store.EngineApp
	Key store.EngineAPIKey
}

type engineRateLimiter struct {
	mu      sync.Mutex
	buckets map[string]engineRateBucket
}

type engineRateBucket struct {
	Window int64
	Count  int64
}

type engineAppResponse struct {
	App store.EngineApp `json:"app"`
}

type engineAppsResponse struct {
	Apps []store.EngineApp `json:"apps"`
}

type engineAPIKeyRequest struct {
	Name           string   `json:"name"`
	Scopes         []string `json:"scopes"`
	AllowedOrigins []string `json:"allowedOrigins"`
	AllowedIPs     []string `json:"allowedIps"`
	ExpiresAt      string   `json:"expiresAt"`
}

type engineAPIKeyResponse struct {
	Key       store.EngineAPIKey `json:"key"`
	Secret    string             `json:"secret,omitempty"`
	Plaintext string             `json:"plaintext,omitempty"`
}

type engineAPIKeysResponse struct {
	Keys []store.EngineAPIKey `json:"keys"`
}

type engineAppDetailResponse struct {
	App   store.EngineApp        `json:"app"`
	Keys  []store.EngineAPIKey   `json:"keys"`
	Usage []store.EngineAPIUsage `json:"usage"`
}

type engineUsageResponse struct {
	Usage []store.EngineAPIUsage `json:"usage"`
}

func newEngineRateLimiter() *engineRateLimiter {
	return &engineRateLimiter{buckets: map[string]engineRateBucket{}}
}

func (l *engineRateLimiter) allow(keyID string, limit int64) bool {
	if limit <= 0 {
		limit = 60
	}
	now := time.Now().Unix() / 60
	l.mu.Lock()
	defer l.mu.Unlock()
	bucket := l.buckets[keyID]
	if bucket.Window != now {
		bucket = engineRateBucket{Window: now}
	}
	if bucket.Count >= limit {
		l.buckets[keyID] = bucket
		return false
	}
	bucket.Count++
	l.buckets[keyID] = bucket
	return true
}

func (s Server) adminEngineApps(c *fiber.Ctx) error {
	apps, err := s.store.ListEngineApps(c.Context())
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load engine apps")
	}
	return c.JSON(engineAppsResponse{Apps: apps})
}

func (s Server) adminCreateEngineApp(c *fiber.Ctx) error {
	var request store.EngineAppInput
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	app, err := s.store.CreateEngineApp(c.Context(), request)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	_, _ = s.store.CreateAdminActivity(c.Context(), store.AdminActivityInput{
		Actor:      s.adminActor(c),
		Action:     "engine_app_created",
		TargetType: "engine_app",
		TargetID:   app.ID,
		Detail:     app.Name + " engine app created.",
		IPAddress:  c.IP(),
		UserAgent:  c.Get("User-Agent"),
	})
	return c.JSON(engineAppResponse{App: app})
}

func (s Server) adminEngineApp(c *fiber.Ctx) error {
	app, ok, err := s.store.GetEngineApp(c.Context(), c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load engine app")
	}
	if !ok {
		return fiber.NewError(fiber.StatusNotFound, "engine app not found")
	}
	keys, err := s.store.ListEngineAPIKeys(c.Context(), app.ID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load engine api keys")
	}
	usage, err := s.store.ListEngineAPIUsage(c.Context(), app.ID, 50)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load engine usage")
	}
	return c.JSON(engineAppDetailResponse{App: app, Keys: keys, Usage: usage})
}

func (s Server) adminCreateEngineAPIKey(c *fiber.Ctx) error {
	var request engineAPIKeyRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	app, ok, err := s.store.GetEngineApp(c.Context(), c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load engine app")
	}
	if !ok {
		return fiber.NewError(fiber.StatusNotFound, "engine app not found")
	}
	if len(request.Scopes) == 0 {
		request.Scopes = defaultEngineScopes
	}
	keySecret, err := generateEngineAPIKey(app.Environment)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to generate engine api key")
	}
	key, err := s.store.CreateEngineAPIKey(c.Context(), app.ID, store.EngineAPIKeyInput{
		Name:           request.Name,
		Scopes:         request.Scopes,
		AllowedOrigins: request.AllowedOrigins,
		AllowedIPs:     request.AllowedIPs,
		ExpiresAt:      request.ExpiresAt,
	}, keySecret.Prefix, hashEngineAPIKey(keySecret.Plaintext))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	_, _ = s.store.CreateAdminActivity(c.Context(), store.AdminActivityInput{
		Actor:      s.adminActor(c),
		Action:     "engine_api_key_created",
		TargetType: "engine_api_key",
		TargetID:   key.ID,
		Detail:     key.Name + " key created for " + app.Name + ".",
		IPAddress:  c.IP(),
		UserAgent:  c.Get("User-Agent"),
	})
	return c.JSON(engineAPIKeyResponse{Key: key, Secret: keySecret.Secret, Plaintext: keySecret.Plaintext})
}

func (s Server) adminEngineAPIKeys(c *fiber.Ctx) error {
	keys, err := s.store.ListEngineAPIKeys(c.Context(), c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load engine api keys")
	}
	return c.JSON(engineAPIKeysResponse{Keys: keys})
}

func (s Server) adminEngineUsage(c *fiber.Ctx) error {
	limit := int64(100)
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil {
			limit = parsed
		}
	}
	usage, err := s.store.ListEngineAPIUsage(c.Context(), c.Params("id"), limit)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load engine usage")
	}
	return c.JSON(engineUsageResponse{Usage: usage})
}

func (s Server) adminRevokeEngineAPIKey(c *fiber.Ctx) error {
	key, err := s.store.RevokeEngineAPIKey(c.Context(), c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	}
	_, _ = s.store.CreateAdminActivity(c.Context(), store.AdminActivityInput{
		Actor:      s.adminActor(c),
		Action:     "engine_api_key_revoked",
		TargetType: "engine_api_key",
		TargetID:   key.ID,
		Detail:     key.Name + " key revoked.",
		IPAddress:  c.IP(),
		UserAgent:  c.Get("User-Agent"),
	})
	return c.JSON(engineAPIKeyResponse{Key: key})
}

func (s Server) adminRotateEngineAPIKey(c *fiber.Ctx) error {
	oldKey, err := s.store.RevokeEngineAPIKey(c.Context(), c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	}
	app, ok, err := s.store.GetEngineApp(c.Context(), oldKey.AppID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load engine app")
	}
	if !ok {
		return fiber.NewError(fiber.StatusNotFound, "engine app not found")
	}
	keySecret, err := generateEngineAPIKey(app.Environment)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to generate engine api key")
	}
	key, err := s.store.CreateEngineAPIKey(c.Context(), app.ID, store.EngineAPIKeyInput{
		Name:           oldKey.Name + " rotated",
		Scopes:         oldKey.Scopes,
		AllowedOrigins: oldKey.AllowedOrigins,
		AllowedIPs:     oldKey.AllowedIPs,
		ExpiresAt:      oldKey.ExpiresAt,
	}, keySecret.Prefix, hashEngineAPIKey(keySecret.Plaintext))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	_, _ = s.store.CreateAdminActivity(c.Context(), store.AdminActivityInput{
		Actor:      s.adminActor(c),
		Action:     "engine_api_key_rotated",
		TargetType: "engine_api_key",
		TargetID:   key.ID,
		Detail:     oldKey.Name + " key rotated.",
		IPAddress:  c.IP(),
		UserAgent:  c.Get("User-Agent"),
	})
	return c.JSON(engineAPIKeyResponse{Key: key, Secret: keySecret.Secret, Plaintext: keySecret.Plaintext})
}

func (s Server) engineAuthMe(c *fiber.Ctx) error {
	principal, ok := c.Locals(enginePrincipalLocalKey).(enginePrincipal)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "engine auth required")
	}
	return c.JSON(fiber.Map{
		"app": principal.App,
		"key": principal.Key,
	})
}

func (s Server) requireEngineScope(scope string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		principal, err := s.enginePrincipal(c)
		if err != nil {
			return err
		}
		if !engineHasScope(principal.Key.Scopes, scope) {
			voidRecordEngineUsage(s, c, principal, fiber.StatusForbidden)
			return fiber.NewError(fiber.StatusForbidden, "engine api key missing scope "+scope)
		}
		if !engineAllowedOrigin(principal.Key.AllowedOrigins, c.Get("Origin")) {
			voidRecordEngineUsage(s, c, principal, fiber.StatusForbidden)
			return fiber.NewError(fiber.StatusForbidden, "origin is not allowed for this engine api key")
		}
		if !engineAllowedIP(principal.Key.AllowedIPs, c.IP()) {
			voidRecordEngineUsage(s, c, principal, fiber.StatusForbidden)
			return fiber.NewError(fiber.StatusForbidden, "ip is not allowed for this engine api key")
		}
		if !s.engineLimiter.allow(principal.Key.ID, principal.App.RateLimitPerMin) {
			voidRecordEngineUsage(s, c, principal, fiber.StatusTooManyRequests)
			return fiber.NewError(fiber.StatusTooManyRequests, "engine api key rate limit exceeded")
		}
		c.Locals(enginePrincipalLocalKey, principal)
		err = c.Next()
		status := c.Response().StatusCode()
		if err != nil {
			if fiberError, ok := err.(*fiber.Error); ok {
				status = fiberError.Code
			} else {
				status = fiber.StatusInternalServerError
			}
		}
		voidRecordEngineUsage(s, c, principal, status)
		return err
	}
}

func (s Server) enginePrincipal(c *fiber.Ctx) (enginePrincipal, error) {
	keyValue, err := engineAPIKeyFromRequest(c)
	if err != nil {
		return enginePrincipal{}, fiber.NewError(fiber.StatusUnauthorized, err.Error())
	}
	prefix, ok := engineAPIKeyPrefix(keyValue)
	if !ok {
		return enginePrincipal{}, fiber.NewError(fiber.StatusUnauthorized, "invalid engine api key")
	}
	verification, ok, err := s.store.VerifyEngineAPIKey(c.Context(), prefix, hashEngineAPIKey(keyValue))
	if err != nil {
		return enginePrincipal{}, fiber.NewError(fiber.StatusInternalServerError, "failed to verify engine api key")
	}
	if !ok {
		return enginePrincipal{}, fiber.NewError(fiber.StatusUnauthorized, "invalid engine api key")
	}
	return enginePrincipal{App: verification.App, Key: verification.Key}, nil
}

func engineAPIKeyFromRequest(c *fiber.Ctx) (string, error) {
	authHeader := strings.TrimSpace(c.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		return strings.TrimSpace(authHeader[7:]), nil
	}
	header := strings.TrimSpace(c.Get(engineAPIKeyHeader))
	if header != "" {
		return header, nil
	}
	return "", errors.New("missing engine api key")
}

type generatedEngineAPIKey struct {
	Prefix    string
	Secret    string
	Plaintext string
}

func generateEngineAPIKey(environment string) (generatedEngineAPIKey, error) {
	randomPrefix, err := randomURLToken(8)
	if err != nil {
		return generatedEngineAPIKey{}, err
	}
	secret, err := randomURLToken(32)
	if err != nil {
		return generatedEngineAPIKey{}, err
	}
	envPart := "dev"
	if strings.EqualFold(environment, "production") {
		envPart = "live"
	}
	prefix := "bdl_" + envPart + "_" + strings.ToLower(randomPrefix)
	return generatedEngineAPIKey{
		Prefix:    prefix,
		Secret:    secret,
		Plaintext: prefix + "." + secret,
	}, nil
}

func randomURLToken(byteCount int) (string, error) {
	buffer := make([]byte, byteCount)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func hashEngineAPIKey(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])
}

func engineAPIKeyPrefix(value string) (string, bool) {
	prefix, _, ok := strings.Cut(strings.TrimSpace(value), ".")
	if !ok || !strings.HasPrefix(prefix, "bdl_") {
		return "", false
	}
	return prefix, true
}

func engineHasScope(scopes []string, required string) bool {
	for _, scope := range scopes {
		if subtle.ConstantTimeCompare([]byte(scope), []byte("admin")) == 1 || subtle.ConstantTimeCompare([]byte(scope), []byte(required)) == 1 {
			return true
		}
	}
	return false
}

func engineAllowedOrigin(allowed []string, origin string) bool {
	if len(allowed) == 0 || strings.TrimSpace(origin) == "" {
		return true
	}
	for _, value := range allowed {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(origin)) {
			return true
		}
	}
	return false
}

func engineAllowedIP(allowed []string, ipAddress string) bool {
	if len(allowed) == 0 {
		return true
	}
	ip := net.ParseIP(strings.TrimSpace(ipAddress))
	for _, value := range allowed {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, network, err := net.ParseCIDR(value); err == nil && ip != nil && network.Contains(ip) {
			return true
		}
		if value == ipAddress {
			return true
		}
	}
	return false
}

func voidRecordEngineUsage(s Server, c *fiber.Ctx, principal enginePrincipal, status int) {
	_ = s.store.RecordEngineAPIUsage(c.Context(), store.EngineAPIUsageInput{
		AppID:      principal.App.ID,
		KeyID:      principal.Key.ID,
		Method:     c.Method(),
		Path:       c.Path(),
		StatusCode: status,
		IPAddress:  c.IP(),
		UserAgent:  c.Get("User-Agent"),
	})
}
