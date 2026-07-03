package thirdweb

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/techbudol1/budol-api/internal/store"
)

var evmAddressPattern = regexp.MustCompile(`^0x[0-9a-fA-F]{40}$`)

func IsEVMAddress(value string) bool {
	return evmAddressPattern.MatchString(strings.TrimSpace(value))
}

type Client struct {
	httpClient *http.Client
	meURL      string
	secretKey  string
	sendURL    string
}

type SendTokenRequest struct {
	ChainID      int
	From         string
	Recipient    string
	TokenAddress string
	Quantity     string
	Recipients   []TokenRecipient
}

type TokenRecipient struct {
	Address  string
	Quantity string
}

type SendTokenResult struct {
	TransactionIDs []string
	RawJSON        string
}

type TransactionStatusResult struct {
	ID              string
	Status          string
	TransactionHash string
	ErrorMessage    string
	RawJSON         string
}

func NewClient(meURL string, sendURL string, secretKey string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		meURL:      meURL,
		secretKey:  secretKey,
		sendURL:    sendURL,
	}
}

func (c *Client) VerifyAuthToken(ctx context.Context, authToken string) (store.ThirdwebIdentity, error) {
	authToken = strings.TrimSpace(authToken)
	if authToken == "" {
		return store.ThirdwebIdentity{}, errors.New("missing thirdweb auth token")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.meURL, nil)
	if err != nil {
		return store.ThirdwebIdentity{}, err
	}
	req.Header.Set("Authorization", "Bearer "+authToken)
	req.Header.Set("x-secret-key", c.secretKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return store.ThirdwebIdentity{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return store.ThirdwebIdentity{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return store.ThirdwebIdentity{}, fmt.Errorf("thirdweb token verification failed: status %d", resp.StatusCode)
	}

	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return store.ThirdwebIdentity{}, fmt.Errorf("invalid thirdweb response: %w", err)
	}

	identity := store.ThirdwebIdentity{
		WalletAddress:  strings.ToLower(findEVMAddress(payload)),
		ThirdwebUserID: findStringByKeys(payload, "userId", "user_id", "id", "sub"),
		AuthProvider:   findStringByKeys(payload, "authProvider", "auth_provider", "type", "strategy"),
		Email:          findStringByKeys(payload, "email"),
		Phone:          findStringByKeys(payload, "phone", "phoneNumber", "phone_number"),
		RawJSON:        string(body),
	}
	if identity.WalletAddress == "" {
		return store.ThirdwebIdentity{}, errors.New("verified thirdweb response did not include an EVM wallet address")
	}

	return identity, nil
}

func (c *Client) SendToken(ctx context.Context, request SendTokenRequest) (SendTokenResult, error) {
	if request.ChainID <= 0 {
		return SendTokenResult{}, errors.New("missing chain id")
	}
	if request.From != "" && !evmAddressPattern.MatchString(request.From) {
		return SendTokenResult{}, errors.New("invalid sender address")
	}
	if request.TokenAddress != "" && !evmAddressPattern.MatchString(request.TokenAddress) {
		return SendTokenResult{}, errors.New("invalid token address")
	}

	recipients := request.Recipients
	if len(recipients) == 0 {
		recipients = []TokenRecipient{
			{
				Address:  request.Recipient,
				Quantity: request.Quantity,
			},
		}
	}

	payloadRecipients := make([]map[string]string, 0, len(recipients))
	for _, recipient := range recipients {
		if !evmAddressPattern.MatchString(recipient.Address) {
			return SendTokenResult{}, errors.New("invalid recipient address")
		}
		if strings.TrimSpace(recipient.Quantity) == "" {
			return SendTokenResult{}, errors.New("missing token quantity")
		}
		payloadRecipients = append(payloadRecipients, map[string]string{
			"address":  recipient.Address,
			"quantity": recipient.Quantity,
		})
	}

	payload := map[string]any{
		"chainId":    request.ChainID,
		"recipients": payloadRecipients,
	}
	if request.From != "" {
		payload["from"] = request.From
	}
	if request.TokenAddress != "" {
		payload["tokenAddress"] = request.TokenAddress
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return SendTokenResult{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.sendURL, bytes.NewReader(body))
	if err != nil {
		return SendTokenResult{}, err
	}
	req.Header.Set("x-secret-key", c.secretKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return SendTokenResult{}, err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return SendTokenResult{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return SendTokenResult{}, fmt.Errorf("thirdweb token send failed: status %d: %s", resp.StatusCode, string(responseBody))
	}

	var payloadResponse struct {
		Result struct {
			TransactionIDs []string `json:"transactionIds"`
		} `json:"result"`
	}
	if err := json.Unmarshal(responseBody, &payloadResponse); err != nil {
		return SendTokenResult{}, fmt.Errorf("invalid thirdweb send response: %w", err)
	}

	return SendTokenResult{
		TransactionIDs: payloadResponse.Result.TransactionIDs,
		RawJSON:        string(responseBody),
	}, nil
}

func (c *Client) TransactionStatus(ctx context.Context, transactionID string) (TransactionStatusResult, error) {
	transactionID = strings.TrimSpace(transactionID)
	if transactionID == "" {
		return TransactionStatusResult{}, errors.New("missing transaction id")
	}
	statusURL := strings.TrimRight(strings.TrimSuffix(c.sendURL, "/wallets/send"), "/") + "/transactions/" + transactionID

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, statusURL, nil)
	if err != nil {
		return TransactionStatusResult{}, err
	}
	req.Header.Set("x-secret-key", c.secretKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return TransactionStatusResult{}, err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return TransactionStatusResult{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return TransactionStatusResult{}, fmt.Errorf("thirdweb transaction status failed: status %d: %s", resp.StatusCode, string(responseBody))
	}

	var payload struct {
		Result struct {
			ID              string `json:"id"`
			Status          string `json:"status"`
			TransactionHash string `json:"transactionHash"`
			ErrorMessage    string `json:"errorMessage"`
			ExecutionResult struct {
				Error struct {
					ErrorCode  string `json:"errorCode"`
					InnerError struct {
						Message string `json:"message"`
					} `json:"innerError"`
				} `json:"error"`
			} `json:"executionResult"`
		} `json:"result"`
	}
	if err := json.Unmarshal(responseBody, &payload); err != nil {
		return TransactionStatusResult{}, fmt.Errorf("invalid thirdweb transaction status response: %w", err)
	}

	errorMessage := strings.TrimSpace(payload.Result.ErrorMessage)
	if errorMessage == "" {
		errorMessage = strings.TrimSpace(payload.Result.ExecutionResult.Error.InnerError.Message)
	}
	if errorMessage == "" {
		errorMessage = strings.TrimSpace(payload.Result.ExecutionResult.Error.ErrorCode)
	}

	return TransactionStatusResult{
		ID:              payload.Result.ID,
		Status:          strings.ToUpper(payload.Result.Status),
		TransactionHash: payload.Result.TransactionHash,
		ErrorMessage:    errorMessage,
		RawJSON:         string(responseBody),
	}, nil
}

func TokenQuantity(amount string, decimals int) (string, error) {
	amount = strings.TrimSpace(amount)
	if amount == "" {
		return "", errors.New("missing amount")
	}
	if decimals < 0 {
		return "", errors.New("invalid decimals")
	}

	parts := strings.Split(amount, ".")
	if len(parts) > 2 {
		return "", errors.New("invalid amount")
	}

	whole := parts[0]
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	if whole == "" {
		whole = "0"
	}
	if len(fraction) > decimals {
		return "", errors.New("amount has more decimal places than token supports")
	}
	fraction += strings.Repeat("0", decimals-len(fraction))

	value := new(big.Int)
	if _, ok := value.SetString(whole+fraction, 10); !ok {
		return "", errors.New("invalid amount")
	}
	if value.Sign() <= 0 {
		return "", errors.New("amount must be greater than zero")
	}
	return value.String(), nil
}

func findEVMAddress(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range []string{"walletAddress", "wallet_address", "address"} {
			if candidate, ok := typed[key].(string); ok && evmAddressPattern.MatchString(candidate) {
				return candidate
			}
		}
		for _, child := range typed {
			if found := findEVMAddress(child); found != "" {
				return found
			}
		}
	case []any:
		for _, child := range typed {
			if found := findEVMAddress(child); found != "" {
				return found
			}
		}
	case string:
		if evmAddressPattern.MatchString(typed) {
			return typed
		}
	}
	return ""
}

func findStringByKeys(value any, keys ...string) string {
	allowed := map[string]struct{}{}
	for _, key := range keys {
		allowed[strings.ToLower(key)] = struct{}{}
	}
	return findStringByKeySet(value, allowed)
}

func findStringByKeySet(value any, keys map[string]struct{}) string {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if _, ok := keys[strings.ToLower(key)]; ok {
				if text, ok := child.(string); ok {
					return text
				}
			}
		}
		for _, child := range typed {
			if found := findStringByKeySet(child, keys); found != "" {
				return found
			}
		}
	case []any:
		for _, child := range typed {
			if found := findStringByKeySet(child, keys); found != "" {
				return found
			}
		}
	}
	return ""
}
