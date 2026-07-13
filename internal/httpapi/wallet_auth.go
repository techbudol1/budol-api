package httpapi

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/techbudol1/budol-api/internal/gmrengine"
	"github.com/techbudol1/budol-api/internal/store"
	"github.com/techbudol1/budol-api/internal/walletops"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gofiber/fiber/v2"
)

const (
	googleOAuthStateCookie = "budol_google_oauth_state"
)

type walletNonceRequest struct {
	Address string `json:"address"`
}

type walletNonceResponse struct {
	Address   string `json:"address"`
	ExpiresAt string `json:"expiresAt"`
	Message   string `json:"message"`
	Nonce     string `json:"nonce"`
}

type walletVerifyRequest struct {
	Address   string `json:"address"`
	Message   string `json:"message"`
	Nonce     string `json:"nonce"`
	Signature string `json:"signature"`
}

func (s Server) walletLoginNonce(c *fiber.Ctx) error {
	var request walletNonceRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	address := normalizeEVMAddress(request.Address)
	if !walletops.IsEVMAddress(address) {
		return fiber.NewError(fiber.StatusBadRequest, "valid wallet address is required")
	}
	nonce, err := randomURLToken(24)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to create wallet challenge")
	}
	expiresAt := time.Now().UTC().Add(10 * time.Minute)
	message := walletLoginMessage(address, nonce, expiresAt)
	challenge, err := s.store.CreateWalletLoginChallenge(c.Context(), address, nonce, message, expiresAt)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to save wallet challenge")
	}
	return c.JSON(walletNonceResponse{
		Address:   challenge.Address,
		ExpiresAt: challenge.ExpiresAt,
		Message:   challenge.Message,
		Nonce:     challenge.Nonce,
	})
}

func (s Server) walletLoginVerify(c *fiber.Ctx) error {
	var request walletVerifyRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	address := normalizeEVMAddress(request.Address)
	if !walletops.IsEVMAddress(address) {
		return fiber.NewError(fiber.StatusBadRequest, "valid wallet address is required")
	}
	challenge, ok, err := s.store.ConsumeWalletLoginChallenge(c.Context(), address, strings.TrimSpace(request.Nonce), time.Now().UTC())
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to verify wallet challenge")
	}
	if !ok || strings.TrimSpace(challenge.Message) == "" || challenge.Message != request.Message {
		return fiber.NewError(fiber.StatusUnauthorized, "wallet login challenge expired or was already used")
	}
	if !signatureMatchesAddress(challenge.Message, request.Signature, address) {
		return fiber.NewError(fiber.StatusUnauthorized, "wallet signature does not match the requested address")
	}
	metadataBytes, _ := json.Marshal(map[string]any{
		"address":    address,
		"method":     "personal_sign",
		"nonce":      challenge.Nonce,
		"verifiedAt": time.Now().UTC().Format(time.RFC3339),
	})
	user, err := s.store.UpsertExternalWallet(c.Context(), store.ExternalWalletIdentity{
		WalletAddress: address,
		Metadata:      string(metadataBytes),
	})
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to save wallet user")
	}
	s.registerExternalEngineUserWallet(c, user, string(metadataBytes))
	s.grantWelcomeTokens(c, user)
	if s.shouldCreateLoginNotification(c, user.ID) {
		_, _ = s.store.CreateNotification(c.Context(), user.ID, "login", "Wallet login successful", "Your external EVM wallet is connected to BudolPH.", "/account")
	}
	if err := s.issueUserSession(c, user); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"user": user})
}

func (s Server) startGoogleOAuth(c *fiber.Ctx) error {
	if strings.TrimSpace(s.cfg.GoogleOAuthClientID) == "" || strings.TrimSpace(s.cfg.GoogleOAuthClientSecret) == "" {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Google OAuth is not configured")
	}
	state, err := randomURLToken(32)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to create OAuth state")
	}
	c.Cookie(&fiber.Cookie{
		Name:     googleOAuthStateCookie,
		Value:    state,
		Expires:  time.Now().UTC().Add(10 * time.Minute),
		HTTPOnly: true,
		SameSite: fiber.CookieSameSiteLaxMode,
		Secure:   s.cfg.IsProduction(),
		Path:     "/",
	})
	query := url.Values{}
	query.Set("client_id", s.cfg.GoogleOAuthClientID)
	query.Set("redirect_uri", s.cfg.GoogleOAuthRedirectURL)
	query.Set("response_type", "code")
	query.Set("scope", "openid email profile")
	query.Set("state", state)
	query.Set("access_type", "offline")
	query.Set("prompt", "select_account")
	return c.Redirect("https://accounts.google.com/o/oauth2/v2/auth?"+query.Encode(), fiber.StatusFound)
}

func (s Server) googleOAuthCallback(c *fiber.Ctx) error {
	if strings.TrimSpace(s.cfg.GoogleOAuthClientID) == "" || strings.TrimSpace(s.cfg.GoogleOAuthClientSecret) == "" {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Google OAuth is not configured")
	}
	if strings.TrimSpace(c.Query("state")) == "" || c.Query("state") != c.Cookies(googleOAuthStateCookie) {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid Google OAuth state")
	}
	c.Cookie(&fiber.Cookie{Name: googleOAuthStateCookie, Value: "", Expires: time.Now().UTC().Add(-time.Hour), HTTPOnly: true, SameSite: fiber.CookieSameSiteLaxMode, Secure: s.cfg.IsProduction(), Path: "/"})
	code := strings.TrimSpace(c.Query("code"))
	if code == "" {
		return fiber.NewError(fiber.StatusBadRequest, "missing Google OAuth code")
	}
	profile, metadata, err := s.googleOAuthProfile(c, code)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, err.Error())
	}
	if strings.TrimSpace(profile.Email) == "" {
		return fiber.NewError(fiber.StatusUnauthorized, "Google account did not return an email")
	}
	if s.gmrEngine == nil || !s.gmrEngine.Configured() {
		return fiber.NewError(fiber.StatusServiceUnavailable, "GMR Engine is required to create a managed BudolPH wallet")
	}
	managedWallet, err := s.gmrEngine.CreateManagedUserWallet(c.Context(), gmrengine.UserWalletRequest{
		AuthProvider: "google",
		Email:        profile.Email,
		Metadata:     metadata,
		UserID:       profile.Subject,
	})
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	user, err := s.store.UpsertGoogleManaged(c.Context(), store.GoogleManagedIdentity{
		Email:         profile.Email,
		GoogleUserID:  profile.Subject,
		Metadata:      metadata,
		WalletAddress: managedWallet.Address,
	})
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to save Google user")
	}
	s.grantWelcomeTokens(c, user)
	if s.shouldCreateLoginNotification(c, user.ID) {
		_, _ = s.store.CreateNotification(c.Context(), user.ID, "login", "Google login successful", "Your managed BudolPH wallet is ready.", "/account")
	}
	if err := s.issueUserSession(c, user); err != nil {
		return err
	}
	return c.Redirect(s.oauthSuccessRedirect("google"), fiber.StatusFound)
}

func (s Server) oauthSuccessRedirect(provider string) string {
	target, err := url.Parse(s.cfg.PublicAppURL)
	if err != nil {
		return s.cfg.PublicAppURL
	}
	query := target.Query()
	query.Set("login", provider)
	target.RawQuery = query.Encode()
	return target.String()
}

func (s Server) issueUserSession(c *fiber.Ctx, user store.User) error {
	token, err := s.sessions.Issue(user)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to issue session")
	}
	c.Cookie(&fiber.Cookie{
		Name:     s.cfg.SessionCookieName,
		Value:    token,
		Expires:  time.Now().UTC().Add(s.sessions.TTL()),
		HTTPOnly: true,
		SameSite: fiber.CookieSameSiteLaxMode,
		Secure:   s.cfg.IsProduction(),
		Path:     "/",
	})
	return nil
}

func (s Server) registerExternalEngineUserWallet(c *fiber.Ctx, user store.User, metadata string) {
	if s.gmrEngine == nil || !s.gmrEngine.Configured() {
		return
	}
	_ = s.gmrEngine.UpsertUserWallet(c.Context(), gmrengine.UserWalletRequest{
		Address:       user.WalletAddress,
		AuthProvider:  "evm_wallet",
		Email:         user.Email,
		Metadata:      metadata,
		UserID:        user.ID,
		WalletCustody: "external",
		WalletType:    "external_evm",
	})
}

func (s Server) registerManagedEngineUserWallet(c *fiber.Ctx, user store.User, metadata string) {
	if s.gmrEngine == nil || !s.gmrEngine.Configured() || strings.TrimSpace(user.WalletAddress) == "" {
		return
	}
	_ = s.gmrEngine.UpsertUserWallet(c.Context(), gmrengine.UserWalletRequest{
		Address:       user.WalletAddress,
		AuthProvider:  firstNonEmpty(user.AuthProvider, "google"),
		Email:         user.Email,
		Metadata:      metadata,
		UserID:        user.ID,
		WalletCustody: "managed",
		WalletType:    firstNonEmpty(user.AuthType, "google_oauth"),
	})
}

type googleOAuthUserProfile struct {
	Email   string `json:"email"`
	Subject string `json:"sub"`
}

func (s Server) googleOAuthProfile(c *fiber.Ctx, code string) (googleOAuthUserProfile, string, error) {
	form := url.Values{}
	form.Set("client_id", s.cfg.GoogleOAuthClientID)
	form.Set("client_secret", s.cfg.GoogleOAuthClientSecret)
	form.Set("code", code)
	form.Set("grant_type", "authorization_code")
	form.Set("redirect_uri", s.cfg.GoogleOAuthRedirectURL)
	request, err := http.NewRequestWithContext(c.Context(), http.MethodPost, "https://oauth2.googleapis.com/token", strings.NewReader(form.Encode()))
	if err != nil {
		return googleOAuthUserProfile{}, "", err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return googleOAuthUserProfile{}, "", err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return googleOAuthUserProfile{}, "", err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return googleOAuthUserProfile{}, "", fmt.Errorf("Google OAuth token exchange failed")
	}
	var tokenPayload struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tokenPayload); err != nil {
		return googleOAuthUserProfile{}, "", err
	}
	if strings.TrimSpace(tokenPayload.AccessToken) == "" {
		return googleOAuthUserProfile{}, "", fmt.Errorf("Google OAuth did not return an access token")
	}
	userRequest, err := http.NewRequestWithContext(c.Context(), http.MethodGet, "https://openidconnect.googleapis.com/v1/userinfo", nil)
	if err != nil {
		return googleOAuthUserProfile{}, "", err
	}
	userRequest.Header.Set("Authorization", "Bearer "+tokenPayload.AccessToken)
	userResponse, err := http.DefaultClient.Do(userRequest)
	if err != nil {
		return googleOAuthUserProfile{}, "", err
	}
	defer userResponse.Body.Close()
	userBody, err := io.ReadAll(io.LimitReader(userResponse.Body, 1<<20))
	if err != nil {
		return googleOAuthUserProfile{}, "", err
	}
	if userResponse.StatusCode < 200 || userResponse.StatusCode >= 300 {
		return googleOAuthUserProfile{}, "", fmt.Errorf("Google user lookup failed")
	}
	var profile googleOAuthUserProfile
	if err := json.Unmarshal(userBody, &profile); err != nil {
		return googleOAuthUserProfile{}, "", err
	}
	return profile, string(bytes.TrimSpace(userBody)), nil
}

func (s Server) userByGoogleEmail(c *fiber.Ctx, email string) (store.User, bool, error) {
	users, err := s.store.ListUsers(c.Context())
	if err != nil {
		return store.User{}, false, err
	}
	for _, user := range users {
		if strings.EqualFold(user.Email, email) && strings.EqualFold(user.AuthType, "google_oauth") {
			return user, true, nil
		}
	}
	return store.User{}, false, nil
}

func (s Server) userBySocialEmail(c *fiber.Ctx, authType string, providerUserID string, email string) (store.User, bool, error) {
	return s.store.GetSocialManaged(c.Context(), authType, providerUserID, email)
}

func walletLoginMessage(address string, nonce string, expiresAt time.Time) string {
	return fmt.Sprintf("BudolPH wallet login\n\nWallet: %s\nNonce: %s\nExpires: %s\n\nOnly sign this message on budolph.xyz.", address, nonce, expiresAt.UTC().Format(time.RFC3339))
}

func signatureMatchesAddress(message string, signatureHex string, address string) bool {
	signatureHex = strings.TrimPrefix(strings.TrimSpace(signatureHex), "0x")
	signature, err := hex.DecodeString(signatureHex)
	if err != nil || len(signature) != 65 {
		return false
	}
	if signature[64] >= 27 {
		signature[64] -= 27
	}
	hash := crypto.Keccak256Hash([]byte(fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(message), message)))
	publicKey, err := crypto.SigToPub(hash.Bytes(), signature)
	if err != nil {
		return false
	}
	recovered := crypto.PubkeyToAddress(*publicKey).Hex()
	return strings.EqualFold(recovered, address)
}

func normalizeEVMAddress(address string) string {
	address = strings.TrimSpace(address)
	if address == "" {
		return ""
	}
	if !strings.HasPrefix(strings.ToLower(address), "0x") {
		return ""
	}
	return address
}
