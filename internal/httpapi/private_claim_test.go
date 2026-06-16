package httpapi

import (
	"encoding/json"
	"testing"

	"budol/server/internal/config"
	"budol/server/internal/gmrengine"
	"budol/server/internal/store"
)

func TestValidatePrivateClaimProofMatchesPublicSignals(t *testing.T) {
	submission := gmrengine.ZKProofSubmission{
		ID:            "proof-1",
		PublicSignals: `["123","1","456"]`,
		Status:        "submitted",
	}
	if err := validatePrivateClaimProof(submission, "123", "1", "456"); err != nil {
		t.Fatalf("expected proof to validate: %v", err)
	}
}

func TestValidatePrivateClaimProofRejectsWrongNullifier(t *testing.T) {
	submission := gmrengine.ZKProofSubmission{
		ID:            "proof-1",
		PublicSignals: `["123","1","456"]`,
		Status:        "submitted",
	}
	if err := validatePrivateClaimProof(submission, "123", "1", "999"); err == nil {
		t.Fatal("expected wrong nullifier to be rejected")
	}
}

func TestValidatePrivateClaimProofRequiresSubmittedStatus(t *testing.T) {
	submission := gmrengine.ZKProofSubmission{
		ID:            "proof-1",
		PublicSignals: `["123","1","456"]`,
		Status:        "queued",
	}
	if err := validatePrivateClaimProof(submission, "123", "1", "456"); err == nil {
		t.Fatal("expected queued proof to be rejected")
	}
}

func TestValidatePrivateClaimProofContextMatchesClaim(t *testing.T) {
	server := Server{cfg: config.Config{
		WelcomeTokenChainID:         421614,
		WelcomeTokenContract:        "0x4f1211149760079b1dea3717d806467c2f744468",
		PrivateClaimRegistryAddress: "0x0000000000000000000000000000000000000001",
	}}
	trade := store.Trade{
		ID:       "trade-1",
		PollID:   "poll-1",
		PollSlug: "poll-slug",
	}
	context, err := server.privateClaimProofContext(trade, "123", "456")
	if err != nil {
		t.Fatalf("expected context: %v", err)
	}
	submission := gmrengine.ZKProofSubmission{Context: string(context)}
	if err := server.validatePrivateClaimProofContext(submission, trade, "123", "456"); err != nil {
		t.Fatalf("expected context to validate: %v", err)
	}

	var tampered map[string]any
	if err := json.Unmarshal(context, &tampered); err != nil {
		t.Fatal(err)
	}
	tampered["tradeId"] = "other-trade"
	tamperedBytes, _ := json.Marshal(tampered)
	submission.Context = string(tamperedBytes)
	if err := server.validatePrivateClaimProofContext(submission, trade, "123", "456"); err == nil {
		t.Fatal("expected tampered context to be rejected")
	}
}
