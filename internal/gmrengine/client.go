package gmrengine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

type TransferRequest struct {
	Amount          string
	ChainID         int
	ContractAddress string
	Decimals        int
	Recipient       string
}

type TransferResult struct {
	TransactionIDs []string `json:"transactionIds"`
	RawJSON        string   `json:"-"`
}

type BalanceResult struct {
	ContractAddress string `json:"contractAddress"`
	Decimals        int64  `json:"decimals"`
	Name            string `json:"name"`
	OwnedBalance    string `json:"ownedBalance"`
	OwnedBalanceRaw string `json:"ownedBalanceRaw"`
	Symbol          string `json:"symbol"`
	TotalSupply     string `json:"totalSupply"`
	TotalSupplyRaw  string `json:"totalSupplyRaw"`
	WalletAddress   string `json:"walletAddress"`
}

type ContractWriteRequest struct {
	ABI             json.RawMessage
	Args            []string
	ChainID         int
	ContractAddress string
	FunctionName    string
	Value           string
	WalletAddress   string
}

type ContractWriteResult struct {
	Transaction struct {
		ID              string `json:"id"`
		Status          string `json:"status"`
		TransactionHash string `json:"transactionHash"`
	} `json:"transaction"`
	RawJSON string `json:"-"`
}

type Transaction struct {
	ID              string   `json:"id"`
	Status          string   `json:"status"`
	TransactionHash string   `json:"transactionHash"`
	Error           string   `json:"error"`
	AttemptLog      []string `json:"attemptLog"`
}

type ZKProofSubmission struct {
	ID                string `json:"id"`
	Context           string `json:"context"`
	PublicSignals     string `json:"publicSignals"`
	Status            string `json:"status"`
	ZKVerifyNetwork   string `json:"zkVerifyNetwork"`
	TransactionResult string `json:"transactionResult"`
	Error             string `json:"error"`
}

type ZKProofSubmitRequest struct {
	Context       json.RawMessage
	DomainID      int64
	Proof         json.RawMessage
	ProofSystem   string
	PublicSignals json.RawMessage
	VK            json.RawMessage
}

type UserWalletRequest struct {
	Address      string
	AuthProvider string
	Email        string
	Metadata     string
	UserID       string
}

func NewClient(baseURL string, apiKey string) *Client {
	return &Client{
		apiKey:     strings.TrimSpace(apiKey),
		baseURL:    strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		httpClient: &http.Client{Timeout: 2 * time.Minute},
	}
}

func (c *Client) Configured() bool {
	return c != nil && c.baseURL != "" && c.apiKey != ""
}

func (c *Client) Transaction(ctx context.Context, id string) (Transaction, error) {
	if !c.Configured() {
		return Transaction{}, errors.New("GMR Engine is not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/transactions/"+url.PathEscape(strings.TrimSpace(id)), nil)
	if err != nil {
		return Transaction{}, err
	}
	c.setHeaders(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Transaction{}, err
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Transaction{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Transaction{}, fmt.Errorf("GMR Engine transaction lookup failed: status %d: %s", resp.StatusCode, string(responseBody))
	}
	var payload struct {
		Transaction Transaction `json:"transaction"`
	}
	if err := json.Unmarshal(responseBody, &payload); err != nil {
		return Transaction{}, fmt.Errorf("invalid GMR Engine transaction response: %w", err)
	}
	return payload.Transaction, nil
}

func (c *Client) ContractWrite(ctx context.Context, request ContractWriteRequest) (ContractWriteResult, error) {
	if !c.Configured() {
		return ContractWriteResult{}, errors.New("GMR Engine is not configured")
	}
	payload := map[string]any{
		"abi":             request.ABI,
		"args":            request.Args,
		"chainId":         request.ChainID,
		"contractAddress": strings.TrimSpace(request.ContractAddress),
		"functionName":    strings.TrimSpace(request.FunctionName),
		"value":           strings.TrimSpace(request.Value),
		"walletAddress":   strings.TrimSpace(request.WalletAddress),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return ContractWriteResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/contracts/write", bytes.NewReader(body))
	if err != nil {
		return ContractWriteResult{}, err
	}
	c.setHeaders(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ContractWriteResult{}, err
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return ContractWriteResult{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ContractWriteResult{}, fmt.Errorf("GMR Engine contract write failed: status %d: %s", resp.StatusCode, string(responseBody))
	}
	var result ContractWriteResult
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return ContractWriteResult{}, fmt.Errorf("invalid GMR Engine contract write response: %w", err)
	}
	result.RawJSON = string(responseBody)
	return result, nil
}

func (c *Client) TransferERC20(ctx context.Context, request TransferRequest) (TransferResult, error) {
	if !c.Configured() {
		return TransferResult{}, errors.New("GMR Engine is not configured")
	}
	payload := map[string]any{
		"amount":          strings.TrimSpace(request.Amount),
		"chainId":         request.ChainID,
		"contractAddress": strings.TrimSpace(request.ContractAddress),
		"decimals":        request.Decimals,
		"recipient":       strings.TrimSpace(request.Recipient),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return TransferResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/erc20/transfer", bytes.NewReader(body))
	if err != nil {
		return TransferResult{}, err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return TransferResult{}, err
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return TransferResult{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return TransferResult{}, fmt.Errorf("GMR Engine transfer failed: status %d: %s", resp.StatusCode, string(responseBody))
	}
	var result TransferResult
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return TransferResult{}, fmt.Errorf("invalid GMR Engine transfer response: %w", err)
	}
	result.RawJSON = string(responseBody)
	return result, nil
}

func (c *Client) ERC20Balance(ctx context.Context, chainID int, contractAddress string, walletAddress string) (BalanceResult, error) {
	if !c.Configured() {
		return BalanceResult{}, errors.New("GMR Engine is not configured")
	}
	query := url.Values{}
	query.Set("chainId", fmt.Sprintf("%d", chainID))
	query.Set("contractAddress", strings.TrimSpace(contractAddress))
	query.Set("walletAddress", strings.TrimSpace(walletAddress))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/erc20/balance?"+query.Encode(), nil)
	if err != nil {
		return BalanceResult{}, err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return BalanceResult{}, err
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return BalanceResult{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return BalanceResult{}, fmt.Errorf("GMR Engine balance failed: status %d: %s", resp.StatusCode, string(responseBody))
	}
	var payload struct {
		Balance BalanceResult `json:"balance"`
	}
	if err := json.Unmarshal(responseBody, &payload); err != nil {
		return BalanceResult{}, fmt.Errorf("invalid GMR Engine balance response: %w", err)
	}
	return payload.Balance, nil
}

func (c *Client) UpsertUserWallet(ctx context.Context, request UserWalletRequest) error {
	if !c.Configured() {
		return errors.New("GMR Engine is not configured")
	}
	payload := map[string]any{
		"address":      strings.TrimSpace(request.Address),
		"authProvider": strings.TrimSpace(request.AuthProvider),
		"email":        strings.TrimSpace(request.Email),
		"metadata":     strings.TrimSpace(request.Metadata),
		"userID":       strings.TrimSpace(request.UserID),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/user-wallets", bytes.NewReader(body))
	if err != nil {
		return err
	}
	c.setHeaders(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("GMR Engine user wallet upsert failed: status %d: %s", resp.StatusCode, string(responseBody))
	}
	return nil
}

func (c *Client) ZKProofSubmission(ctx context.Context, id string) (ZKProofSubmission, error) {
	if !c.Configured() {
		return ZKProofSubmission{}, errors.New("GMR Engine is not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/zkverify/proofs/"+url.PathEscape(strings.TrimSpace(id)), nil)
	if err != nil {
		return ZKProofSubmission{}, err
	}
	c.setHeaders(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ZKProofSubmission{}, err
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return ZKProofSubmission{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ZKProofSubmission{}, fmt.Errorf("GMR Engine zk proof lookup failed: status %d: %s", resp.StatusCode, string(responseBody))
	}
	var payload struct {
		Submission ZKProofSubmission `json:"submission"`
	}
	if err := json.Unmarshal(responseBody, &payload); err != nil {
		return ZKProofSubmission{}, fmt.Errorf("invalid GMR Engine zk proof response: %w", err)
	}
	return payload.Submission, nil
}

func (c *Client) SubmitZKProof(ctx context.Context, request ZKProofSubmitRequest) (ZKProofSubmission, error) {
	if !c.Configured() {
		return ZKProofSubmission{}, errors.New("GMR Engine is not configured")
	}
	payload := map[string]any{
		"context":       json.RawMessage("null"),
		"domainId":      request.DomainID,
		"proof":         request.Proof,
		"proofSystem":   firstNonEmpty(request.ProofSystem, "groth16"),
		"publicSignals": request.PublicSignals,
		"vk":            request.VK,
	}
	if len(request.Context) > 0 {
		payload["context"] = request.Context
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return ZKProofSubmission{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/zkverify/proofs", bytes.NewReader(body))
	if err != nil {
		return ZKProofSubmission{}, err
	}
	c.setHeaders(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ZKProofSubmission{}, err
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return ZKProofSubmission{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ZKProofSubmission{}, fmt.Errorf("GMR Engine zk proof submission failed: status %d: %s", resp.StatusCode, string(responseBody))
	}
	var payloadResponse struct {
		Submission ZKProofSubmission `json:"submission"`
	}
	if err := json.Unmarshal(responseBody, &payloadResponse); err != nil {
		return ZKProofSubmission{}, fmt.Errorf("invalid GMR Engine zk proof submission response: %w", err)
	}
	return payloadResponse.Submission, nil
}

func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GMR-Engine-Key", c.apiKey)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
