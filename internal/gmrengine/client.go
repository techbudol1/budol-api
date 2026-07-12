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

type TransferWithPermitRequest struct {
	Amount          string
	ChainID         int
	ContractAddress string
	Deadline        string
	Decimals        int
	Owner           string
	Recipient       string
	R               string
	S               string
	V               int
}

type TransferResult struct {
	TransactionIDs []string `json:"transactionIds"`
	RawJSON        string   `json:"-"`
}

type TransferWithPermitResult struct {
	PermitTransactionHash   string   `json:"permitTransactionHash"`
	TransactionIDs          []string `json:"transactionIds"`
	TransferTransactionHash string   `json:"transferTransactionHash"`
	RawJSON                 string   `json:"-"`
}

type AuthMeResult struct {
	App struct {
		GasFreeEnabled bool   `json:"gasFreeEnabled"`
		ID             string `json:"id"`
		Name           string `json:"name"`
	} `json:"app"`
}

type GasFreeSettingsResult struct {
	App struct {
		GasFreeEnabled bool   `json:"gasFreeEnabled"`
		ID             string `json:"id"`
		Name           string `json:"name"`
	} `json:"app"`
}

type ProjectWallet struct {
	Address        string `json:"address"`
	IsDefaultAdmin bool   `json:"isDefaultAdmin"`
	Status         string `json:"status"`
	WalletType     string `json:"walletType"`
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
	Address       string
	AuthProvider  string
	Email         string
	Metadata      string
	UserID        string
	WalletCustody string
	WalletType    string
}

type UserWallet struct {
	ID            string `json:"id"`
	AppID         string `json:"appId"`
	UserID        string `json:"userId"`
	Address       string `json:"address"`
	AuthProvider  string `json:"authProvider"`
	Email         string `json:"email"`
	Status        string `json:"status"`
	Metadata      string `json:"metadata"`
	WalletCustody string `json:"walletCustody"`
	WalletType    string `json:"walletType"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
	LastSeenAt    string `json:"lastSeenAt"`
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

func (c *Client) AuthMe(ctx context.Context) (AuthMeResult, error) {
	if !c.Configured() {
		return AuthMeResult{}, errors.New("GMR Engine is not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/auth/me", nil)
	if err != nil {
		return AuthMeResult{}, err
	}
	c.setHeaders(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return AuthMeResult{}, err
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return AuthMeResult{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return AuthMeResult{}, fmt.Errorf("GMR Engine auth lookup failed: status %d: %s", resp.StatusCode, string(responseBody))
	}
	var result AuthMeResult
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return AuthMeResult{}, fmt.Errorf("invalid GMR Engine auth response: %w", err)
	}
	return result, nil
}

func (c *Client) UpdateGasFree(ctx context.Context, enabled bool) (GasFreeSettingsResult, error) {
	if !c.Configured() {
		return GasFreeSettingsResult{}, errors.New("GMR Engine is not configured")
	}
	body, err := json.Marshal(map[string]any{"gasFreeEnabled": enabled})
	if err != nil {
		return GasFreeSettingsResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, c.baseURL+"/v1/app/gas-free", bytes.NewReader(body))
	if err != nil {
		return GasFreeSettingsResult{}, err
	}
	c.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return GasFreeSettingsResult{}, err
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return GasFreeSettingsResult{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return GasFreeSettingsResult{}, fmt.Errorf("GMR Engine gas-free update failed: status %d: %s", resp.StatusCode, string(responseBody))
	}
	var result GasFreeSettingsResult
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return GasFreeSettingsResult{}, fmt.Errorf("invalid GMR Engine gas-free response: %w", err)
	}
	return result, nil
}

func (c *Client) DefaultAdminWallet(ctx context.Context) (ProjectWallet, bool, error) {
	if !c.Configured() {
		return ProjectWallet{}, false, errors.New("GMR Engine is not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/wallets?limit=100", nil)
	if err != nil {
		return ProjectWallet{}, false, err
	}
	c.setHeaders(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ProjectWallet{}, false, err
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return ProjectWallet{}, false, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ProjectWallet{}, false, fmt.Errorf("GMR Engine wallet lookup failed: status %d: %s", resp.StatusCode, string(responseBody))
	}
	var payload struct {
		Wallets []ProjectWallet `json:"wallets"`
	}
	if err := json.Unmarshal(responseBody, &payload); err != nil {
		return ProjectWallet{}, false, fmt.Errorf("invalid GMR Engine wallet response: %w", err)
	}
	for _, wallet := range payload.Wallets {
		if wallet.IsDefaultAdmin && strings.EqualFold(wallet.Status, "active") {
			return wallet, true, nil
		}
	}
	return ProjectWallet{}, false, nil
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

func (c *Client) TransferERC20WithPermit(ctx context.Context, request TransferWithPermitRequest) (TransferWithPermitResult, error) {
	if !c.Configured() {
		return TransferWithPermitResult{}, errors.New("GMR Engine is not configured")
	}
	payload := map[string]any{
		"amount":          strings.TrimSpace(request.Amount),
		"chainId":         request.ChainID,
		"contractAddress": strings.TrimSpace(request.ContractAddress),
		"deadline":        strings.TrimSpace(request.Deadline),
		"decimals":        request.Decimals,
		"owner":           strings.TrimSpace(request.Owner),
		"recipient":       strings.TrimSpace(request.Recipient),
		"r":               strings.TrimSpace(request.R),
		"s":               strings.TrimSpace(request.S),
		"v":               request.V,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return TransferWithPermitResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/erc20/transfer-with-permit", bytes.NewReader(body))
	if err != nil {
		return TransferWithPermitResult{}, err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return TransferWithPermitResult{}, err
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return TransferWithPermitResult{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return TransferWithPermitResult{}, fmt.Errorf("GMR Engine gas-free transfer failed: status %d: %s", resp.StatusCode, string(responseBody))
	}
	var result TransferWithPermitResult
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return TransferWithPermitResult{}, fmt.Errorf("invalid GMR Engine gas-free transfer response: %w", err)
	}
	result.RawJSON = string(responseBody)
	return result, nil
}

func (c *Client) TransferManagedERC20WithPermit(ctx context.Context, request TransferWithPermitRequest) (TransferWithPermitResult, error) {
	if !c.Configured() {
		return TransferWithPermitResult{}, errors.New("GMR Engine is not configured")
	}
	payload := map[string]any{
		"amount":          strings.TrimSpace(request.Amount),
		"chainId":         request.ChainID,
		"contractAddress": strings.TrimSpace(request.ContractAddress),
		"deadline":        strings.TrimSpace(request.Deadline),
		"decimals":        request.Decimals,
		"owner":           strings.TrimSpace(request.Owner),
		"recipient":       strings.TrimSpace(request.Recipient),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return TransferWithPermitResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/erc20/managed-transfer-with-permit", bytes.NewReader(body))
	if err != nil {
		return TransferWithPermitResult{}, err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return TransferWithPermitResult{}, err
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return TransferWithPermitResult{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return TransferWithPermitResult{}, fmt.Errorf("GMR Engine managed transfer failed: status %d: %s", resp.StatusCode, string(responseBody))
	}
	var result TransferWithPermitResult
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return TransferWithPermitResult{}, fmt.Errorf("invalid GMR Engine managed transfer response: %w", err)
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
		"address":       strings.TrimSpace(request.Address),
		"authProvider":  strings.TrimSpace(request.AuthProvider),
		"email":         strings.TrimSpace(request.Email),
		"metadata":      strings.TrimSpace(request.Metadata),
		"userID":        strings.TrimSpace(request.UserID),
		"walletCustody": strings.TrimSpace(request.WalletCustody),
		"walletType":    strings.TrimSpace(request.WalletType),
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

func (c *Client) CreateManagedUserWallet(ctx context.Context, request UserWalletRequest) (UserWallet, error) {
	if !c.Configured() {
		return UserWallet{}, errors.New("GMR Engine is not configured")
	}
	payload := map[string]any{
		"authProvider": strings.TrimSpace(request.AuthProvider),
		"email":        strings.TrimSpace(request.Email),
		"metadata":     strings.TrimSpace(request.Metadata),
		"userID":       strings.TrimSpace(request.UserID),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return UserWallet{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/user-wallets/managed", bytes.NewReader(body))
	if err != nil {
		return UserWallet{}, err
	}
	c.setHeaders(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return UserWallet{}, err
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return UserWallet{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return UserWallet{}, fmt.Errorf("GMR Engine managed user wallet failed: status %d: %s", resp.StatusCode, string(responseBody))
	}
	var result struct {
		Wallet UserWallet `json:"wallet"`
	}
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return UserWallet{}, fmt.Errorf("invalid GMR Engine managed wallet response: %w", err)
	}
	if strings.TrimSpace(result.Wallet.Address) == "" {
		return UserWallet{}, errors.New("GMR Engine did not return a managed wallet address")
	}
	return result.Wallet, nil
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
