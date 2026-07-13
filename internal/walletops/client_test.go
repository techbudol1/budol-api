package walletops

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func TestTokenQuantity(t *testing.T) {
	got, err := TokenQuantity("100", 18)
	if err != nil {
		t.Fatal(err)
	}
	if got != "100000000000000000000" {
		t.Fatalf("unexpected quantity: %s", got)
	}
}

func TestTokenQuantityRejectsNonPositive(t *testing.T) {
	for _, amount := range []string{"0", "-1"} {
		if _, err := TokenQuantity(amount, 18); err == nil {
			t.Fatalf("expected %s to be rejected", amount)
		}
	}
}

func TestSendTokenSupportsMultipleRecipients(t *testing.T) {
	client := &Client{
		httpClient: &http.Client{Transport: walletopsRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			var payload struct {
				ChainID      int `json:"chainId"`
				TokenAddress string
				Recipients   []TokenRecipient
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload.ChainID != 421614 {
				t.Fatalf("unexpected chain id: %d", payload.ChainID)
			}
			if len(payload.Recipients) != 2 {
				t.Fatalf("expected 2 recipients, got %d", len(payload.Recipients))
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewBufferString(`{"result":{"transactionIds":["0xabc"]}}`)),
			}, nil
		})},
		sendURL:   "http://walletops.test/send",
		secretKey: "secret",
	}
	result, err := client.SendToken(context.Background(), SendTokenRequest{
		ChainID:      421614,
		TokenAddress: "0x12fF5d28F93c1CABDA4Bd0ddf8906FF7E4Df1c4e",
		Recipients: []TokenRecipient{
			{Address: "0x1111111111111111111111111111111111111111", Quantity: "100"},
			{Address: "0x2222222222222222222222222222222222222222", Quantity: "200"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.TransactionIDs) != 1 || result.TransactionIDs[0] != "0xabc" {
		t.Fatalf("unexpected transaction ids: %#v", result.TransactionIDs)
	}
}

func TestTransactionStatusReturnsFailureDetails(t *testing.T) {
	client := &Client{
		httpClient: &http.Client{Transport: walletopsRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path != "/v1/transactions/tx-1" {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body: io.NopCloser(bytes.NewBufferString(`{
					"result": {
						"id": "tx-1",
						"status": "FAILED",
						"transactionHash": null,
						"errorMessage": "Error: SIGNING_FAILED",
						"executionResult": {
							"error": {
								"errorCode": "SIGNING_FAILED",
								"innerError": {
									"message": "EOA not found"
								}
							}
						}
					}
				}`)),
			}, nil
		})},
		sendURL:   "https://api.walletops.com/v1/wallets/send",
		secretKey: "secret",
	}

	status, err := client.TransactionStatus(context.Background(), "tx-1")
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != "FAILED" {
		t.Fatalf("unexpected status: %#v", status)
	}
	if status.ErrorMessage != "Error: SIGNING_FAILED" {
		t.Fatalf("unexpected error message: %s", status.ErrorMessage)
	}
}

type walletopsRoundTripFunc func(*http.Request) (*http.Response, error)

func (f walletopsRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
