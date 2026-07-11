package evm

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
	"sort"
	"strings"
	"time"
)

const balanceOfSelector = "70a08231"
const transferTopic = "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"

var evmAddressPattern = regexp.MustCompile(`^0x[0-9a-fA-F]{40}$`)

type Client struct {
	httpClient *http.Client
	rpcURL     string
}

type TokenBalance struct {
	Raw           string `json:"raw"`
	Formatted     string `json:"formatted"`
	Decimals      int    `json:"decimals"`
	WalletAddress string `json:"walletAddress"`
	TokenAddress  string `json:"tokenAddress"`
	FetchedAt     string `json:"fetchedAt"`
}

type NativeBalance struct {
	Raw           string `json:"raw"`
	Formatted     string `json:"formatted"`
	Decimals      int    `json:"decimals"`
	WalletAddress string `json:"walletAddress"`
	Symbol        string `json:"symbol"`
	FetchedAt     string `json:"fetchedAt"`
}

type TokenTransfer struct {
	Direction       string `json:"direction"`
	Counterparty    string `json:"counterparty"`
	AmountRaw       string `json:"amountRaw"`
	Amount          string `json:"amount"`
	TokenAddress    string `json:"tokenAddress"`
	TokenSymbol     string `json:"tokenSymbol"`
	TokenLabel      string `json:"tokenLabel"`
	TransactionHash string `json:"transactionHash"`
	BlockNumber     uint64 `json:"blockNumber"`
	LogIndex        uint64 `json:"logIndex"`
	Timestamp       string `json:"timestamp"`
}

type TransactionReceiptStatus struct {
	Confirmed bool
	Failed    bool
	Pending   bool
}

type receiptLog struct {
	Address         string   `json:"address"`
	Data            string   `json:"data"`
	Topics          []string `json:"topics"`
	TransactionHash string   `json:"transactionHash"`
}

func NewClient(rpcURL string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 8 * time.Second},
		rpcURL:     strings.TrimSpace(rpcURL),
	}
}

func (c *Client) NativeBalance(ctx context.Context, walletAddress string) (NativeBalance, error) {
	walletAddress = strings.TrimSpace(walletAddress)
	if c.rpcURL == "" {
		return NativeBalance{}, errors.New("RPC URL is not configured")
	}
	if !evmAddressPattern.MatchString(walletAddress) {
		return NativeBalance{}, errors.New("invalid wallet address")
	}

	var rpcResponse struct {
		Result string `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := c.rpc(ctx, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "eth_getBalance",
		"params":  []any{walletAddress, "latest"},
	}, &rpcResponse); err != nil {
		return NativeBalance{}, err
	}
	if rpcResponse.Error != nil {
		return NativeBalance{}, fmt.Errorf("RPC native balance read failed: %s", rpcResponse.Error.Message)
	}
	value, err := parseHexBigInt(rpcResponse.Result)
	if err != nil {
		return NativeBalance{}, err
	}
	return NativeBalance{
		Raw:           value.String(),
		Formatted:     formatUnits(value, 18),
		Decimals:      18,
		WalletAddress: walletAddress,
		Symbol:        "ETH",
		FetchedAt:     time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func (c *Client) ERC20TransferHistory(ctx context.Context, tokenAddress string, walletAddress string, decimals int) ([]TokenTransfer, error) {
	tokenAddress = strings.TrimSpace(tokenAddress)
	walletAddress = strings.TrimSpace(walletAddress)
	if c.rpcURL == "" {
		return nil, errors.New("RPC URL is not configured")
	}
	if !evmAddressPattern.MatchString(tokenAddress) {
		return nil, errors.New("invalid token address")
	}
	if !evmAddressPattern.MatchString(walletAddress) {
		return nil, errors.New("invalid wallet address")
	}
	if decimals < 0 {
		return nil, errors.New("invalid token decimals")
	}

	if transfers, err := c.alchemyTransferHistory(ctx, tokenAddress, walletAddress, decimals); err == nil {
		return transfers, nil
	}

	sentLogs, err := c.transferLogs(ctx, tokenAddress, walletAddress, decimals, "sent")
	if err != nil {
		return nil, err
	}
	receivedLogs, err := c.transferLogs(ctx, tokenAddress, walletAddress, decimals, "received")
	if err != nil {
		return nil, err
	}

	transfersByKey := map[string]TokenTransfer{}
	blockNumbers := map[uint64]struct{}{}
	for _, transfer := range append(sentLogs, receivedLogs...) {
		key := transfer.TransactionHash + ":" + fmt.Sprint(transfer.LogIndex)
		transfersByKey[key] = transfer
		blockNumbers[transfer.BlockNumber] = struct{}{}
	}

	timestamps := map[uint64]string{}
	for blockNumber := range blockNumbers {
		timestamp, err := c.blockTimestamp(ctx, blockNumber)
		if err == nil {
			timestamps[blockNumber] = timestamp
		}
	}

	transfers := make([]TokenTransfer, 0, len(transfersByKey))
	for _, transfer := range transfersByKey {
		transfer.Timestamp = timestamps[transfer.BlockNumber]
		transfers = append(transfers, transfer)
	}

	sort.Slice(transfers, func(i int, j int) bool {
		if transfers[i].BlockNumber == transfers[j].BlockNumber {
			return transfers[i].LogIndex > transfers[j].LogIndex
		}
		return transfers[i].BlockNumber > transfers[j].BlockNumber
	})
	if len(transfers) > 60 {
		transfers = transfers[:60]
	}
	return transfers, nil
}

func (c *Client) alchemyTransferHistory(ctx context.Context, tokenAddress string, walletAddress string, decimals int) ([]TokenTransfer, error) {
	if !strings.Contains(strings.ToLower(c.rpcURL), "alchemy") {
		return nil, errors.New("alchemy transfer API is not available for this RPC URL")
	}
	sentTransfers, err := c.alchemyAssetTransfers(ctx, tokenAddress, walletAddress, decimals, "sent")
	if err != nil {
		return nil, err
	}
	receivedTransfers, err := c.alchemyAssetTransfers(ctx, tokenAddress, walletAddress, decimals, "received")
	if err != nil {
		return nil, err
	}
	transfers := append(sentTransfers, receivedTransfers...)
	sort.Slice(transfers, func(i int, j int) bool {
		if transfers[i].BlockNumber == transfers[j].BlockNumber {
			return transfers[i].LogIndex > transfers[j].LogIndex
		}
		return transfers[i].BlockNumber > transfers[j].BlockNumber
	})
	if len(transfers) > 60 {
		transfers = transfers[:60]
	}
	return transfers, nil
}

func (c *Client) ERC20Balance(ctx context.Context, tokenAddress string, walletAddress string, decimals int) (TokenBalance, error) {
	tokenAddress = strings.TrimSpace(tokenAddress)
	walletAddress = strings.TrimSpace(walletAddress)
	if c.rpcURL == "" {
		return TokenBalance{}, errors.New("RPC URL is not configured")
	}
	if !evmAddressPattern.MatchString(tokenAddress) {
		return TokenBalance{}, errors.New("invalid token address")
	}
	if !evmAddressPattern.MatchString(walletAddress) {
		return TokenBalance{}, errors.New("invalid wallet address")
	}
	if decimals < 0 {
		return TokenBalance{}, errors.New("invalid token decimals")
	}

	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "eth_call",
		"params": []any{
			map[string]string{
				"to":   tokenAddress,
				"data": balanceOfCalldata(walletAddress),
			},
			"latest",
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return TokenBalance{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.rpcURL, bytes.NewReader(body))
	if err != nil {
		return TokenBalance{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return TokenBalance{}, err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return TokenBalance{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return TokenBalance{}, fmt.Errorf("RPC balance read failed: status %d", resp.StatusCode)
	}

	var rpcResponse struct {
		Result string `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(responseBody, &rpcResponse); err != nil {
		return TokenBalance{}, fmt.Errorf("invalid RPC response: %w", err)
	}
	if rpcResponse.Error != nil {
		return TokenBalance{}, fmt.Errorf("RPC balance read failed: %s", rpcResponse.Error.Message)
	}

	value, err := parseHexBigInt(rpcResponse.Result)
	if err != nil {
		return TokenBalance{}, err
	}

	return TokenBalance{
		Raw:           value.String(),
		Formatted:     formatUnits(value, decimals),
		Decimals:      decimals,
		WalletAddress: walletAddress,
		TokenAddress:  tokenAddress,
		FetchedAt:     time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func (c *Client) WaitForERC20Transfer(ctx context.Context, tokenAddress string, transactionHash string, from string, to string, amountRaw string) (bool, error) {
	for attempt := 0; attempt < 8; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return false, ctx.Err()
			case <-time.After(1500 * time.Millisecond):
			}
		}
		found, pending, err := c.ERC20TransferInTransaction(ctx, tokenAddress, transactionHash, from, to, amountRaw)
		if err != nil {
			return false, err
		}
		if found {
			return true, nil
		}
		if !pending {
			return false, nil
		}
	}
	return false, nil
}

func (c *Client) ERC20TransferInTransaction(ctx context.Context, tokenAddress string, transactionHash string, from string, to string, amountRaw string) (bool, bool, error) {
	tokenAddress = strings.ToLower(strings.TrimSpace(tokenAddress))
	transactionHash = strings.ToLower(strings.TrimSpace(transactionHash))
	from = strings.ToLower(strings.TrimSpace(from))
	to = strings.ToLower(strings.TrimSpace(to))
	amountRaw = strings.TrimSpace(amountRaw)
	if c.rpcURL == "" {
		return false, false, errors.New("RPC URL is not configured")
	}
	if !evmAddressPattern.MatchString(tokenAddress) {
		return false, false, errors.New("invalid token address")
	}
	if !evmAddressPattern.MatchString(from) || !evmAddressPattern.MatchString(to) {
		return false, false, errors.New("invalid escrow wallet address")
	}
	if !strings.HasPrefix(transactionHash, "0x") || len(transactionHash) != 66 {
		return false, false, errors.New("invalid transaction hash")
	}

	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "eth_getTransactionReceipt",
		"params":  []any{transactionHash},
	}
	var rpcResponse struct {
		Result *struct {
			Status string       `json:"status"`
			Logs   []receiptLog `json:"logs"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := c.rpc(ctx, payload, &rpcResponse); err != nil {
		return false, false, err
	}
	if rpcResponse.Error != nil {
		return false, false, fmt.Errorf("RPC transaction receipt failed: %s", rpcResponse.Error.Message)
	}
	if rpcResponse.Result == nil {
		return false, true, nil
	}
	if strings.ToLower(rpcResponse.Result.Status) != "0x1" {
		return false, false, nil
	}

	fromTopic := topicAddress(from)
	toTopic := topicAddress(to)
	for _, log := range rpcResponse.Result.Logs {
		if strings.ToLower(log.Address) != tokenAddress || len(log.Topics) < 3 {
			continue
		}
		if strings.ToLower(log.Topics[0]) != transferTopic {
			continue
		}
		if strings.ToLower(log.Topics[1]) != fromTopic || strings.ToLower(log.Topics[2]) != toTopic {
			continue
		}
		value, err := parseHexBigInt(log.Data)
		if err != nil {
			continue
		}
		if value.String() == amountRaw {
			return true, false, nil
		}
	}
	return false, false, nil
}

func (c *Client) TransactionReceiptStatus(ctx context.Context, transactionHash string) (TransactionReceiptStatus, error) {
	transactionHash = strings.ToLower(strings.TrimSpace(transactionHash))
	if c.rpcURL == "" {
		return TransactionReceiptStatus{}, errors.New("RPC URL is not configured")
	}
	if !strings.HasPrefix(transactionHash, "0x") || len(transactionHash) != 66 {
		return TransactionReceiptStatus{}, errors.New("invalid transaction hash")
	}

	var rpcResponse struct {
		Result *struct {
			Status string `json:"status"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := c.rpc(ctx, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "eth_getTransactionReceipt",
		"params":  []any{transactionHash},
	}, &rpcResponse); err != nil {
		return TransactionReceiptStatus{}, err
	}
	if rpcResponse.Error != nil {
		return TransactionReceiptStatus{}, fmt.Errorf("RPC transaction receipt failed: %s", rpcResponse.Error.Message)
	}
	if rpcResponse.Result == nil {
		return TransactionReceiptStatus{Pending: true}, nil
	}
	if strings.EqualFold(rpcResponse.Result.Status, "0x1") {
		return TransactionReceiptStatus{Confirmed: true}, nil
	}
	return TransactionReceiptStatus{Failed: true}, nil
}

func (c *Client) transferLogs(ctx context.Context, tokenAddress string, walletAddress string, decimals int, direction string) ([]TokenTransfer, error) {
	walletTopic := topicAddress(walletAddress)
	topics := []any{transferTopic, nil, nil}
	if direction == "sent" {
		topics[1] = walletTopic
	} else {
		topics[2] = walletTopic
	}

	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "eth_getLogs",
		"params": []any{
			map[string]any{
				"fromBlock": "0x0",
				"toBlock":   "latest",
				"address":   tokenAddress,
				"topics":    topics,
			},
		},
	}

	var rpcResponse struct {
		Result []struct {
			Data            string   `json:"data"`
			Topics          []string `json:"topics"`
			BlockNumber     string   `json:"blockNumber"`
			LogIndex        string   `json:"logIndex"`
			TransactionHash string   `json:"transactionHash"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := c.rpc(ctx, payload, &rpcResponse); err != nil {
		return nil, err
	}
	if rpcResponse.Error != nil {
		return nil, fmt.Errorf("RPC transfer history failed: %s", rpcResponse.Error.Message)
	}

	transfers := make([]TokenTransfer, 0, len(rpcResponse.Result))
	for _, item := range rpcResponse.Result {
		if len(item.Topics) < 3 {
			continue
		}
		value, err := parseHexBigInt(item.Data)
		if err != nil {
			continue
		}
		blockNumber, _ := parseHexUint(item.BlockNumber)
		logIndex, _ := parseHexUint(item.LogIndex)
		counterpartyTopic := item.Topics[1]
		if direction == "sent" {
			counterpartyTopic = item.Topics[2]
		}
		transfers = append(transfers, TokenTransfer{
			Direction:       direction,
			Counterparty:    addressFromTopic(counterpartyTopic),
			AmountRaw:       value.String(),
			Amount:          formatUnits(value, decimals),
			TransactionHash: item.TransactionHash,
			BlockNumber:     blockNumber,
			LogIndex:        logIndex,
		})
	}
	return transfers, nil
}

func (c *Client) alchemyAssetTransfers(ctx context.Context, tokenAddress string, walletAddress string, decimals int, direction string) ([]TokenTransfer, error) {
	params := map[string]any{
		"category":          []string{"erc20"},
		"contractAddresses": []string{tokenAddress},
		"fromBlock":         "0x0",
		"maxCount":          "0x3c",
		"order":             "desc",
		"toBlock":           "latest",
		"withMetadata":      true,
	}
	if direction == "sent" {
		params["fromAddress"] = walletAddress
	} else {
		params["toAddress"] = walletAddress
	}
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "alchemy_getAssetTransfers",
		"params":  []any{params},
	}
	var rpcResponse struct {
		Result struct {
			Transfers []struct {
				BlockNum    string `json:"blockNum"`
				Category    string `json:"category"`
				From        string `json:"from"`
				Hash        string `json:"hash"`
				To          string `json:"to"`
				UniqueID    string `json:"uniqueId"`
				RawContract struct {
					Address string `json:"address"`
					Value   string `json:"value"`
				} `json:"rawContract"`
				Metadata struct {
					BlockTimestamp string `json:"blockTimestamp"`
				} `json:"metadata"`
			} `json:"transfers"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := c.rpc(ctx, payload, &rpcResponse); err != nil {
		return nil, err
	}
	if rpcResponse.Error != nil {
		return nil, fmt.Errorf("Alchemy transfer history failed: %s", rpcResponse.Error.Message)
	}
	transfers := make([]TokenTransfer, 0, len(rpcResponse.Result.Transfers))
	for index, item := range rpcResponse.Result.Transfers {
		if !strings.EqualFold(item.RawContract.Address, tokenAddress) {
			continue
		}
		value, err := parseHexBigInt(item.RawContract.Value)
		if err != nil {
			continue
		}
		blockNumber, _ := parseHexUint(item.BlockNum)
		counterparty := item.From
		if direction == "sent" {
			counterparty = item.To
		}
		transfers = append(transfers, TokenTransfer{
			Direction:       direction,
			Counterparty:    strings.ToLower(counterparty),
			AmountRaw:       value.String(),
			Amount:          formatUnits(value, decimals),
			TransactionHash: item.Hash,
			BlockNumber:     blockNumber,
			LogIndex:        uint64(index),
			Timestamp:       normalizeRPCTimestamp(item.Metadata.BlockTimestamp),
		})
	}
	return transfers, nil
}

func (c *Client) blockTimestamp(ctx context.Context, blockNumber uint64) (string, error) {
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "eth_getBlockByNumber",
		"params":  []any{fmt.Sprintf("0x%x", blockNumber), false},
	}
	var rpcResponse struct {
		Result struct {
			Timestamp string `json:"timestamp"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := c.rpc(ctx, payload, &rpcResponse); err != nil {
		return "", err
	}
	if rpcResponse.Error != nil {
		return "", fmt.Errorf("RPC block read failed: %s", rpcResponse.Error.Message)
	}
	timestamp, err := parseHexUint(rpcResponse.Result.Timestamp)
	if err != nil {
		return "", err
	}
	return time.Unix(int64(timestamp), 0).UTC().Format(time.RFC3339), nil
}

func (c *Client) rpc(ctx context.Context, payload map[string]any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.rpcURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

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
		return fmt.Errorf("RPC request failed: status %d", resp.StatusCode)
	}

	if err := json.Unmarshal(responseBody, out); err != nil {
		return fmt.Errorf("invalid RPC response: %w", err)
	}
	return nil
}

func balanceOfCalldata(walletAddress string) string {
	address := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(walletAddress)), "0x")
	return "0x" + balanceOfSelector + strings.Repeat("0", 64-len(address)) + address
}

func topicAddress(walletAddress string) string {
	address := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(walletAddress)), "0x")
	return "0x" + strings.Repeat("0", 64-len(address)) + address
}

func addressFromTopic(topic string) string {
	topic = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(topic)), "0x")
	if len(topic) < 40 {
		return ""
	}
	return "0x" + topic[len(topic)-40:]
}

func normalizeRPCTimestamp(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err == nil {
		return parsed.UTC().Format(time.RFC3339)
	}
	parsed, err = time.Parse("2006-01-02T15:04:05.000Z", value)
	if err == nil {
		return parsed.UTC().Format(time.RFC3339)
	}
	return value
}

func parseHexBigInt(value string) (*big.Int, error) {
	value = strings.TrimPrefix(strings.TrimSpace(value), "0x")
	if value == "" {
		return nil, errors.New("empty RPC balance result")
	}
	result := new(big.Int)
	if _, ok := result.SetString(value, 16); !ok {
		return nil, errors.New("invalid RPC balance result")
	}
	return result, nil
}

func parseHexUint(value string) (uint64, error) {
	parsed := new(big.Int)
	if _, ok := parsed.SetString(strings.TrimPrefix(strings.TrimSpace(value), "0x"), 16); !ok {
		return 0, errors.New("invalid hex value")
	}
	return parsed.Uint64(), nil
}

func formatUnits(value *big.Int, decimals int) string {
	if decimals == 0 {
		return value.String()
	}
	base := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	whole := new(big.Int).Div(value, base)
	fraction := new(big.Int).Mod(value, base).String()
	fraction = strings.Repeat("0", decimals-len(fraction)) + fraction
	fraction = strings.TrimRight(fraction, "0")
	if fraction == "" {
		return whole.String()
	}
	return whole.String() + "." + fraction
}
