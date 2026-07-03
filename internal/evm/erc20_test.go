package evm

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"testing"
)

func TestBalanceOfCalldata(t *testing.T) {
	got := balanceOfCalldata("0x81d9b68eA5F8185a54b2e427770253Cc195B4892")
	want := "0x70a0823100000000000000000000000081d9b68ea5f8185a54b2e427770253cc195b4892"
	if got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
}

func TestTopicAddress(t *testing.T) {
	got := topicAddress("0x81d9b68eA5F8185a54b2e427770253Cc195B4892")
	want := "0x00000000000000000000000081d9b68ea5f8185a54b2e427770253cc195b4892"
	if got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
}

func TestAddressFromTopic(t *testing.T) {
	got := addressFromTopic("0x00000000000000000000000081d9b68ea5f8185a54b2e427770253cc195b4892")
	want := "0x81d9b68ea5f8185a54b2e427770253cc195b4892"
	if got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
}

func TestFormatUnits(t *testing.T) {
	value := new(big.Int)
	value.SetString("123450000000000000000", 10)
	if got := formatUnits(value, 18); got != "123.45" {
		t.Fatalf("unexpected formatted value: %s", got)
	}
}

func TestERC20TransferHistory(t *testing.T) {
	requestNumber := 0
	client := &Client{
		rpcURL: "http://rpc.test",
		httpClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			requestNumber++
			var payload struct {
				Method string `json:"method"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}

			response := `{"jsonrpc":"2.0","id":1,"result":[]}`
			if payload.Method == "eth_getLogs" && requestNumber == 1 {
				response = `{"jsonrpc":"2.0","id":1,"result":[{
					"data":"0x56bc75e2d63100000",
					"topics":[
						"0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef",
						"0x00000000000000000000000081d9b68ea5f8185a54b2e427770253cc195b4892",
						"0x0000000000000000000000001111111111111111111111111111111111111111"
					],
					"blockNumber":"0x10",
					"logIndex":"0x1",
					"transactionHash":"0xabc"
				}]}`
			}
			if payload.Method == "eth_getLogs" && requestNumber == 2 {
				response = `{"jsonrpc":"2.0","id":1,"result":[{
					"data":"0x2b5e3af16b1880000",
					"topics":[
						"0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef",
						"0x0000000000000000000000002222222222222222222222222222222222222222",
						"0x00000000000000000000000081d9b68ea5f8185a54b2e427770253cc195b4892"
					],
					"blockNumber":"0x11",
					"logIndex":"0x0",
					"transactionHash":"0xdef"
				}]}`
			}
			if payload.Method == "eth_getBlockByNumber" {
				response = `{"jsonrpc":"2.0","id":1,"result":{"timestamp":"0x65a0bc00"}}`
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewBufferString(response)),
			}, nil
		})},
	}

	transfers, err := client.ERC20TransferHistory(
		context.Background(),
		"0x12fF5d28F93c1CABDA4Bd0ddf8906FF7E4Df1c4e",
		"0x81d9b68eA5F8185a54b2e427770253Cc195B4892",
		18,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(transfers) != 2 {
		t.Fatalf("expected 2 transfers, got %d", len(transfers))
	}
	if transfers[0].Direction != "received" || transfers[0].Amount != "50" {
		t.Fatalf("unexpected first transfer: %#v", transfers[0])
	}
	if transfers[1].Direction != "sent" || transfers[1].Amount != "100" {
		t.Fatalf("unexpected second transfer: %#v", transfers[1])
	}
}

func TestERC20Balance(t *testing.T) {
	client := &Client{
		rpcURL: "http://rpc.test",
		httpClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			var payload struct {
				Method string `json:"method"`
				Params []any  `json:"params"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload.Method != "eth_call" {
				t.Fatalf("unexpected method: %s", payload.Method)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"result":"0x56bc75e2d63100000"}`)),
			}, nil
		})},
	}

	balance, err := client.ERC20Balance(
		context.Background(),
		"0x12fF5d28F93c1CABDA4Bd0ddf8906FF7E4Df1c4e",
		"0x81d9b68eA5F8185a54b2e427770253Cc195B4892",
		18,
	)
	if err != nil {
		t.Fatal(err)
	}
	if balance.Formatted != "100" {
		t.Fatalf("unexpected balance: %#v", balance)
	}
}

func TestERC20TransferInTransaction(t *testing.T) {
	client := &Client{
		rpcURL: "http://rpc.test",
		httpClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			var payload struct {
				Method string `json:"method"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload.Method != "eth_getTransactionReceipt" {
				t.Fatalf("unexpected method: %s", payload.Method)
			}
			response := `{"jsonrpc":"2.0","id":1,"result":{
				"status":"0x1",
				"logs":[{
					"address":"0x12fF5d28F93c1CABDA4Bd0ddf8906FF7E4Df1c4e",
					"data":"0x56bc75e2d63100000",
					"topics":[
						"0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef",
						"0x00000000000000000000000081d9b68ea5f8185a54b2e427770253cc195b4892",
						"0x000000000000000000000000c6488d9fb82c1cb112482643c62478d4d28bcb0e"
					]
				}]
			}}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewBufferString(response)),
			}, nil
		})},
	}
	found, pending, err := client.ERC20TransferInTransaction(
		context.Background(),
		"0x12fF5d28F93c1CABDA4Bd0ddf8906FF7E4Df1c4e",
		"0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"0x81d9b68eA5F8185a54b2e427770253Cc195B4892",
		"0xC6488D9Fb82C1cB112482643C62478D4D28bcB0E",
		"100000000000000000000",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !found || pending {
		t.Fatalf("expected confirmed transfer, found=%v pending=%v", found, pending)
	}
}

func TestTransactionReceiptStatusConfirmed(t *testing.T) {
	client := receiptStatusTestClient(t, `{"jsonrpc":"2.0","id":1,"result":{"status":"0x1"}}`)
	status, err := client.TransactionReceiptStatus(context.Background(), "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil {
		t.Fatal(err)
	}
	if !status.Confirmed || status.Pending || status.Failed {
		t.Fatalf("unexpected receipt status: %#v", status)
	}
}

func TestTransactionReceiptStatusPending(t *testing.T) {
	client := receiptStatusTestClient(t, `{"jsonrpc":"2.0","id":1,"result":null}`)
	status, err := client.TransactionReceiptStatus(context.Background(), "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil {
		t.Fatal(err)
	}
	if !status.Pending || status.Confirmed || status.Failed {
		t.Fatalf("unexpected receipt status: %#v", status)
	}
}

func receiptStatusTestClient(t *testing.T, response string) *Client {
	t.Helper()
	return &Client{
		rpcURL: "http://rpc.test",
		httpClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			var payload struct {
				Method string `json:"method"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload.Method != "eth_getTransactionReceipt" {
				t.Fatalf("unexpected method: %s", payload.Method)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewBufferString(response)),
			}, nil
		})},
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
