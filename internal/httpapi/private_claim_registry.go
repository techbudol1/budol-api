package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"strings"
	"time"

	"github.com/techbudol1/budol-api/internal/gmrengine"
	"github.com/techbudol1/budol-api/internal/store"
)

const privateClaimFieldModulusText = "21888242871839275222246405745257275088548364400416034343698204186575808495617"

var privateClaimRegistryABI = json.RawMessage(`[
  {
    "type": "function",
    "name": "acceptRoot",
    "stateMutability": "nonpayable",
    "inputs": [
      {"name": "marketId", "type": "bytes32"},
      {"name": "root", "type": "bytes32"},
      {"name": "leafCount", "type": "uint256"}
    ],
    "outputs": []
  },
  {
    "type": "function",
    "name": "setMarketResolution",
    "stateMutability": "nonpayable",
    "inputs": [
      {"name": "marketId", "type": "bytes32"},
      {"name": "outcome", "type": "uint256"}
    ],
    "outputs": []
  },
  {
    "type": "function",
    "name": "registerZKVerifyClaim",
    "stateMutability": "nonpayable",
    "inputs": [
      {"name": "marketId", "type": "bytes32"},
      {"name": "root", "type": "bytes32"},
      {"name": "nullifierHash", "type": "bytes32"},
      {"name": "zkProofSubmissionId", "type": "bytes32"}
    ],
    "outputs": []
  }
]`)

func (s Server) privateClaimRegistryConfigured() bool {
	return s.gmrEngine != nil &&
		s.gmrEngine.Configured() &&
		strings.TrimSpace(s.cfg.PrivateClaimRegistryAddress) != "" &&
		s.cfg.PrivateClaimRegistryChainID > 0
}

func (s Server) requirePrivateClaimRegistryForStrictClaims() error {
	if !s.cfg.PrivateClaimRegistryRequired {
		return nil
	}
	if !s.privateClaimRegistryConfigured() {
		return errors.New("private claim registry is required but not configured")
	}
	return nil
}

func (s Server) acceptPrivateClaimRegistryRoot(ctx context.Context, pollID string, status string, outcome string) {
	if !s.privateClaimRegistryConfigured() {
		return
	}
	tree, err := s.store.PrivateClaimTree(ctx, pollID)
	if err != nil {
		log.Printf("private claim registry root load failed: %v", err)
		return
	}
	tree, err = s.privateClaimTreeWithRoot(ctx, tree, "registry_accept_root")
	if err != nil {
		log.Printf("private claim registry root compute failed: %v", err)
		return
	}
	if tree.LeafCount <= 0 || isZeroField(tree.Root) {
		return
	}
	marketID := privateClaimTextToBytes32(pollID)
	root, err := privateClaimFieldToBytes32(tree.Root)
	if err != nil {
		log.Printf("private claim registry root encode failed: %v", err)
		return
	}
	if _, err := s.gmrEngine.ContractWrite(ctx, gmrengine.ContractWriteRequest{
		ABI:             privateClaimRegistryABI,
		Args:            []string{marketID, root, fmt.Sprintf("%d", tree.LeafCount)},
		ChainID:         s.cfg.PrivateClaimRegistryChainID,
		ContractAddress: s.cfg.PrivateClaimRegistryAddress,
		FunctionName:    "acceptRoot",
	}); err != nil {
		log.Printf("private claim registry acceptRoot failed: %v", err)
		return
	}

	if strings.EqualFold(strings.TrimSpace(status), "resolved") {
		resolvedOutcome, err := privateClaimResolvedOutcomeForAdminOutcome(outcome)
		if err != nil {
			log.Printf("private claim registry resolution encode failed: %v", err)
			return
		}
		if _, err := s.gmrEngine.ContractWrite(ctx, gmrengine.ContractWriteRequest{
			ABI:             privateClaimRegistryABI,
			Args:            []string{marketID, resolvedOutcome},
			ChainID:         s.cfg.PrivateClaimRegistryChainID,
			ContractAddress: s.cfg.PrivateClaimRegistryAddress,
			FunctionName:    "setMarketResolution",
		}); err != nil {
			log.Printf("private claim registry setMarketResolution failed: %v", err)
		}
	}
}

func (s Server) registerPrivateClaimOnRegistry(ctx context.Context, trade store.Trade, root string, nullifierHash string, zkProofSubmissionID string) (string, string, error) {
	if !s.privateClaimRegistryConfigured() {
		if s.cfg.PrivateClaimRegistryRequired {
			return "", "", errors.New("private claim registry is required but not configured")
		}
		return "", "", nil
	}
	if isZeroField(root) {
		return "", "", errors.New("private claim registry root is zero")
	}
	rootBytes, err := privateClaimFieldToBytes32(root)
	if err != nil {
		return "", "", err
	}
	nullifierBytes, err := privateClaimFieldToBytes32(nullifierHash)
	if err != nil {
		return "", "", err
	}
	result, err := s.gmrEngine.ContractWrite(ctx, gmrengine.ContractWriteRequest{
		ABI:             privateClaimRegistryABI,
		Args:            []string{privateClaimTextToBytes32(trade.PollID), rootBytes, nullifierBytes, privateClaimTextToBytes32(zkProofSubmissionID)},
		ChainID:         s.cfg.PrivateClaimRegistryChainID,
		ContractAddress: s.cfg.PrivateClaimRegistryAddress,
		FunctionName:    "registerZKVerifyClaim",
	})
	if err != nil {
		return "", "", err
	}
	status := result.Transaction.Status
	if status == "" {
		status = "queued"
	}
	if s.cfg.PrivateClaimRegistryRequired {
		transaction, err := s.waitForGMRTransaction(ctx, result.Transaction.ID, s.cfg.PrivateClaimRegistryConfirmTimeout)
		if err != nil {
			return result.Transaction.ID, "failed", err
		}
		status = transaction.Status
	}
	return result.Transaction.ID, status, nil
}

func (s Server) waitForGMRTransaction(ctx context.Context, transactionID string, timeout time.Duration) (gmrengine.Transaction, error) {
	transactionID = strings.TrimSpace(transactionID)
	if transactionID == "" {
		return gmrengine.Transaction{}, errors.New("transaction ID is required")
	}
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		transaction, err := s.gmrEngine.Transaction(waitCtx, transactionID)
		if err != nil {
			return gmrengine.Transaction{}, err
		}
		switch strings.ToLower(strings.TrimSpace(transaction.Status)) {
		case "confirmed":
			return transaction, nil
		case "failed":
			if strings.TrimSpace(transaction.Error) != "" {
				return transaction, errors.New(transaction.Error)
			}
			return transaction, errors.New("GMR Engine transaction failed")
		}
		select {
		case <-waitCtx.Done():
			return transaction, errors.New("timed out waiting for GMR Engine transaction confirmation")
		case <-ticker.C:
		}
	}
}

func privateClaimResolvedOutcomeForAdminOutcome(outcome string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(outcome)) {
	case "a":
		return "1", nil
	case "b":
		return "2", nil
	default:
		return "", errors.New("resolved markets require outcome a or b")
	}
}

func privateClaimFieldToBytes32(value string) (string, error) {
	number, ok := fieldElement(value)
	if !ok {
		return "", errors.New("field value is invalid")
	}
	modulus, _ := new(big.Int).SetString(privateClaimFieldModulusText, 10)
	number.Mod(number, modulus)
	return privateClaimBigIntToBytes32(number), nil
}

func privateClaimTextToBytes32(value string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(value)))
	number := new(big.Int).SetBytes(digest[:])
	modulus, _ := new(big.Int).SetString(privateClaimFieldModulusText, 10)
	number.Mod(number, modulus)
	return privateClaimBigIntToBytes32(number)
}

func privateClaimBigIntToBytes32(number *big.Int) string {
	bytes := number.Bytes()
	if len(bytes) > 32 {
		bytes = bytes[len(bytes)-32:]
	}
	padded := make([]byte, 32)
	copy(padded[32-len(bytes):], bytes)
	return "0x" + hex.EncodeToString(padded)
}

func isZeroField(value string) bool {
	number, ok := fieldElement(value)
	return ok && number.Sign() == 0
}
