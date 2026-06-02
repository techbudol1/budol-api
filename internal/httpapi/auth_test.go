package httpapi

import (
	"encoding/json"
	"testing"
)

func TestAuthTokenFromAuthResult(t *testing.T) {
	raw := json.RawMessage(`{"storedToken":{"cookieString":"thirdweb-jwt-token"}}`)

	token, err := authTokenFromAuthResult(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "thirdweb-jwt-token" {
		t.Fatalf("expected token from authResult, got %q", token)
	}
}

func TestAuthTokenFromRequestPrefersExplicitAuthToken(t *testing.T) {
	token, err := authTokenFromRequest(ThirdwebLoginRequest{
		AuthToken:  "direct-token",
		AuthResult: json.RawMessage(`{"storedToken":{"cookieString":"callback-token"}}`),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "direct-token" {
		t.Fatalf("expected direct token, got %q", token)
	}
}
