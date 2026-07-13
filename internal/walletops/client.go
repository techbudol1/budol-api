package walletops

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
)

var evmAddressPattern = regexp.MustCompile(`^0x[0-9a-fA-F]{40}$`)

func IsEVMAddress(value string) bool {
	return evmAddressPattern.MatchString(strings.TrimSpace(value))
}

type Client struct {
	httpClient *http.Client
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

func NewClient(sendURL string, secretKey string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		secretKey:  secretKey,
		sendURL:    sendURL,
	}
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
		return SendTokenResult{}, fmt.Errorf("walletops token send failed: status %d: %s", resp.StatusCode, string(responseBody))
	}

	var payloadResponse struct {
		Result struct {
			TransactionIDs []string `json:"transactionIds"`
		} `json:"result"`
	}
	if err := json.Unmarshal(responseBody, &payloadResponse); err != nil {
		return SendTokenResult{}, fmt.Errorf("invalid walletops send response: %w", err)
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
		return TransactionStatusResult{}, fmt.Errorf("walletops transaction status failed: status %d: %s", resp.StatusCode, string(responseBody))
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
		return TransactionStatusResult{}, fmt.Errorf("invalid walletops transaction status response: %w", err)
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
