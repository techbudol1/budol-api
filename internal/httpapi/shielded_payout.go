package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	"budol/server/internal/gmrengine"
	"budol/server/internal/store"
	"budol/server/internal/thirdweb"

	"github.com/gofiber/fiber/v2"
)

var erc20ApproveABI = json.RawMessage(`[
  {
    "type": "function",
    "name": "approve",
    "stateMutability": "nonpayable",
    "inputs": [
      {"name": "spender", "type": "address"},
      {"name": "amount", "type": "uint256"}
    ],
    "outputs": [{"name": "", "type": "bool"}]
  }
]`)

var shieldedPayoutPoolABI = json.RawMessage(`[
  {
    "type": "function",
    "name": "depositAndCredit",
    "stateMutability": "nonpayable",
    "inputs": [{"name": "noteCommitment", "type": "bytes32"}],
    "outputs": []
  },
  {
    "type": "function",
    "name": "withdrawWithZKVerify",
    "stateMutability": "nonpayable",
    "inputs": [
      {"name": "noteCommitment", "type": "bytes32"},
      {"name": "nullifierHash", "type": "bytes32"},
      {"name": "zkProofSubmissionId", "type": "bytes32"},
      {"name": "recipient", "type": "address"},
      {"name": "relayer", "type": "address"},
      {"name": "relayerFee", "type": "uint256"}
    ],
    "outputs": []
  },
  {
    "type": "function",
    "name": "withdrawVerified",
    "stateMutability": "nonpayable",
    "inputs": [
      {"name": "noteCommitment", "type": "bytes32"},
      {"name": "nullifierHash", "type": "bytes32"},
      {"name": "recipient", "type": "address"},
      {"name": "relayer", "type": "address"},
      {"name": "relayerFee", "type": "uint256"},
      {"name": "proof", "type": "bytes"},
      {"name": "publicSignals", "type": "uint256[]"}
    ],
    "outputs": []
  }
]`)

type ShieldedWithdrawalProofSubmissionRequest struct {
	NoteCommitment string          `json:"noteCommitment"`
	NullifierHash  string          `json:"nullifierHash"`
	Proof          json.RawMessage `json:"proof"`
	ProofSystem    string          `json:"proofSystem"`
	PublicSignals  json.RawMessage `json:"publicSignals"`
	Recipient      string          `json:"recipient"`
	VK             json.RawMessage `json:"vk"`
}

type ShieldedWithdrawalRequest struct {
	NoteCommitment      string          `json:"noteCommitment"`
	NullifierHash       string          `json:"nullifierHash"`
	PublicSignals       json.RawMessage `json:"publicSignals"`
	Recipient           string          `json:"recipient"`
	SolidityProof       string          `json:"solidityProof"`
	ZKProofSubmissionID string          `json:"zkProofSubmissionId"`
}

type AdminShieldedWithdrawalActionRequest struct {
	Reason string `json:"reason"`
}

type shieldedWithdrawalExecutionResult struct {
	Status          string
	TransactionID   string
	TransactionHash string
}

type shieldedPayoutCreditResult struct {
	PayoutError    string
	PayoutStatus   string
	TransactionIDs []string
}

const shieldedWithdrawalQueuePausedSetting = "shielded_withdrawal_queue_paused"

func (s Server) shieldedPayoutConfigured() bool {
	return s.gmrEngine != nil &&
		s.gmrEngine.Configured() &&
		s.cfg.ShieldedPayoutEnabled &&
		len(s.cfg.ShieldedPayoutPools) > 0 &&
		s.cfg.ShieldedPayoutPoolChainID > 0
}

func (s Server) requireShieldedPayoutForStrictClaims() error {
	if !s.cfg.ShieldedPayoutRequired {
		return nil
	}
	if !s.shieldedPayoutConfigured() {
		return errors.New("shielded payout is required but not configured")
	}
	return nil
}

func (s Server) privateClaimShieldedPayoutConfig(c *fiber.Ctx) error {
	defaultPoolAddress := strings.ToLower(strings.TrimSpace(s.cfg.ShieldedPayoutPoolAddress))
	defaultDenomination := strings.TrimSpace(s.cfg.ShieldedPayoutDenomination)
	if (defaultPoolAddress == "" || defaultDenomination == "") && len(s.cfg.ShieldedPayoutPools) > 0 {
		defaultPoolAddress = strings.ToLower(strings.TrimSpace(s.cfg.ShieldedPayoutPools[0].PoolAddress))
		defaultDenomination = strings.TrimSpace(s.cfg.ShieldedPayoutPools[0].Denomination)
	}
	return c.JSON(fiber.Map{
		"enabled":      s.shieldedPayoutConfigured(),
		"chainId":      s.cfg.ShieldedPayoutPoolChainID,
		"poolAddress":  defaultPoolAddress,
		"pools":        s.shieldedPayoutPools(),
		"tokenAddress": strings.ToLower(strings.TrimSpace(s.cfg.WelcomeTokenContract)),
		"denomination": defaultDenomination,
		"version":      "budol-shielded-payout-v1",
	})
}

func (s Server) shieldedWithdrawalArtifact(c *fiber.Ctx) error {
	file := strings.TrimSpace(c.Params("file"))
	switch file {
	case "shielded_withdrawal.wasm", "shielded_withdrawal_final.zkey", "verification_key.json":
	default:
		return fiber.NewError(fiber.StatusNotFound, "shielded withdrawal artifact not found")
	}
	path, err := shieldedWithdrawalPublicArtifactPath(file)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	}
	if err := s.verifyShieldedWithdrawalArtifactChecksum(path); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	if strings.HasSuffix(file, ".wasm") {
		c.Set(fiber.HeaderContentType, "application/wasm")
	} else if strings.HasSuffix(file, ".json") {
		c.Set(fiber.HeaderContentType, "application/json")
	} else {
		c.Set(fiber.HeaderContentType, "application/octet-stream")
	}
	c.Set("Cross-Origin-Resource-Policy", "cross-origin")
	return c.SendFile(path)
}

func (s Server) submitShieldedWithdrawalProof(c *fiber.Ctx) error {
	if _, err := s.authenticatedUser(c); err != nil {
		return err
	}
	if !s.shieldedPayoutConfigured() {
		return fiber.NewError(fiber.StatusBadGateway, "shielded payout pool is not configured")
	}
	var request ShieldedWithdrawalProofSubmissionRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	request.NoteCommitment = strings.ToLower(strings.TrimSpace(request.NoteCommitment))
	request.NullifierHash = strings.ToLower(strings.TrimSpace(request.NullifierHash))
	request.Recipient = strings.TrimSpace(request.Recipient)
	if !isBytes32Hex(request.NoteCommitment) {
		return fiber.NewError(fiber.StatusBadRequest, "noteCommitment must be a 32-byte hex string")
	}
	if !isBytes32Hex(request.NullifierHash) {
		return fiber.NewError(fiber.StatusBadRequest, "nullifierHash must be a 32-byte hex string")
	}
	if !thirdweb.IsEVMAddress(request.Recipient) {
		return fiber.NewError(fiber.StatusBadRequest, "recipient must be a valid EVM address")
	}
	if len(request.Proof) == 0 || string(request.Proof) == "null" {
		return fiber.NewError(fiber.StatusBadRequest, "proof is required")
	}
	if len(request.PublicSignals) == 0 || string(request.PublicSignals) == "null" {
		return fiber.NewError(fiber.StatusBadRequest, "publicSignals is required")
	}
	if len(request.VK) == 0 || string(request.VK) == "null" {
		return fiber.NewError(fiber.StatusBadRequest, "vk is required")
	}
	poolAddress, denomination, err := s.validateShieldedWithdrawalPublicSignals(request.PublicSignals, request.NoteCommitment, request.NullifierHash, request.Recipient)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	proofSystem := strings.TrimSpace(request.ProofSystem)
	if proofSystem == "" {
		proofSystem = "groth16"
	}
	context, _ := s.shieldedWithdrawalProofContext(request.NoteCommitment, request.NullifierHash, request.Recipient, poolAddress, denomination)
	submission, err := s.gmrEngine.SubmitZKProof(c.Context(), gmrengine.ZKProofSubmitRequest{
		Context:       context,
		DomainID:      1,
		Proof:         request.Proof,
		ProofSystem:   proofSystem,
		PublicSignals: request.PublicSignals,
		VK:            request.VK,
	})
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
		"submission": submission,
	})
}

func (s Server) listShieldedWithdrawals(c *fiber.Ctx) error {
	user, err := s.authenticatedUser(c)
	if err != nil {
		return err
	}
	withdrawals, err := s.store.ListUserShieldedWithdrawals(c.Context(), user.ID, 50)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load shielded withdrawals")
	}
	return c.JSON(fiber.Map{"withdrawals": withdrawals})
}

func (s Server) withdrawShieldedPayout(c *fiber.Ctx) error {
	user, err := s.authenticatedUser(c)
	if err != nil {
		return err
	}
	if !s.shieldedPayoutConfigured() {
		return fiber.NewError(fiber.StatusBadGateway, "shielded payout pool is not configured")
	}
	var request ShieldedWithdrawalRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	request.NoteCommitment = strings.ToLower(strings.TrimSpace(request.NoteCommitment))
	request.NullifierHash = strings.ToLower(strings.TrimSpace(request.NullifierHash))
	request.Recipient = strings.TrimSpace(request.Recipient)
	if !isBytes32Hex(request.NoteCommitment) {
		return fiber.NewError(fiber.StatusBadRequest, "noteCommitment must be a 32-byte hex string")
	}
	if !isBytes32Hex(request.NullifierHash) {
		return fiber.NewError(fiber.StatusBadRequest, "nullifierHash must be a 32-byte hex string")
	}
	if !thirdweb.IsEVMAddress(request.Recipient) {
		return fiber.NewError(fiber.StatusBadRequest, "recipient must be a valid EVM address")
	}
	withdrawalMode := strings.ToLower(strings.TrimSpace(s.cfg.ShieldedWithdrawalMode))
	if withdrawalMode == "" {
		withdrawalMode = "zkverify"
	}
	if withdrawalMode == "zkverify" && strings.TrimSpace(request.ZKProofSubmissionID) == "" {
		return fiber.NewError(fiber.StatusBadRequest, "zkProofSubmissionId is required")
	}
	relayer := s.shieldedWithdrawalRelayerAddress()
	relayerFee := s.shieldedWithdrawalRelayerFee()
	zkProofSubmissionRef := privateClaimTextToBytes32(request.ZKProofSubmissionID)
	if withdrawalMode == "verified" {
		if !isHexBytes(request.SolidityProof) {
			return fiber.NewError(fiber.StatusBadRequest, "solidityProof must be ABI-encoded Groth16 proof bytes")
		}
		if len(request.PublicSignals) == 0 || string(request.PublicSignals) == "null" {
			return fiber.NewError(fiber.StatusBadRequest, "publicSignals is required")
		}
		if _, _, err := s.validateShieldedWithdrawalPublicSignals(request.PublicSignals, request.NoteCommitment, request.NullifierHash, request.Recipient); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
	} else {
		submission, err := s.gmrEngine.ZKProofSubmission(c.Context(), request.ZKProofSubmissionID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		if err := s.validateShieldedWithdrawalProof(submission, request.NoteCommitment, request.NullifierHash, request.Recipient); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
	}
	poolAddress, denomination, err := s.validateShieldedWithdrawalPublicSignals(request.PublicSignals, request.NoteCommitment, request.NullifierHash, request.Recipient)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	withdrawal, err := s.store.CreateShieldedWithdrawal(c.Context(), user.ID, store.ShieldedWithdrawalInput{
		Denomination:         denomination,
		Mode:                 withdrawalMode,
		NoteCommitment:       request.NoteCommitment,
		NullifierHash:        request.NullifierHash,
		PoolAddress:          poolAddress,
		PublicSignals:        string(request.PublicSignals),
		Recipient:            request.Recipient,
		Relayer:              relayer,
		RelayerFee:           relayerFee,
		SolidityProof:        strings.TrimSpace(request.SolidityProof),
		ZKProofSubmissionID:  strings.TrimSpace(request.ZKProofSubmissionID),
		ZKProofSubmissionRef: zkProofSubmissionRef,
		ExecuteAfter:         s.nextShieldedWithdrawalExecutionTime(),
	})
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	_, _ = s.store.CreateNotification(c.Context(), user.ID, "shielded_withdrawal", "Shielded withdrawal queued", "Your shielded BUDOL payout withdrawal was queued for relayed processing.", "/portfolio")
	return c.JSON(fiber.Map{
		"noteCommitment":       request.NoteCommitment,
		"nullifierHash":        request.NullifierHash,
		"recipient":            request.Recipient,
		"status":               withdrawal.Status,
		"transaction":          nil,
		"withdrawal":           withdrawal,
		"withdrawalMode":       withdrawalMode,
		"zkProofSubmissionId":  request.ZKProofSubmissionID,
		"zkProofSubmissionRef": zkProofSubmissionRef,
	})
}

func (s Server) retryShieldedWithdrawal(c *fiber.Ctx) error {
	user, err := s.authenticatedUser(c)
	if err != nil {
		return err
	}
	if !s.shieldedPayoutConfigured() {
		return fiber.NewError(fiber.StatusBadGateway, "shielded payout pool is not configured")
	}
	withdrawalID := strings.TrimSpace(c.Params("id"))
	if withdrawalID == "" {
		return fiber.NewError(fiber.StatusBadRequest, "withdrawal id is required")
	}
	withdrawal, err := s.store.RetryShieldedWithdrawal(c.Context(), user.ID, withdrawalID, s.nextShieldedWithdrawalExecutionTime())
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	_, _ = s.store.CreateNotification(c.Context(), user.ID, "shielded_withdrawal", "Shielded withdrawal retry queued", "Your failed shielded withdrawal was queued for another relayed attempt.", "/portfolio")
	return c.JSON(fiber.Map{"withdrawal": withdrawal})
}

func (s Server) adminShieldedWithdrawals(c *fiber.Ctx) error {
	status := strings.ToLower(strings.TrimSpace(c.Query("status")))
	limit := int64(c.QueryInt("limit", 100))
	withdrawals, err := s.store.ListShieldedWithdrawals(c.Context(), status, limit)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load shielded withdrawals")
	}
	paused, _ := s.shieldedWithdrawalQueuePaused(c.Context())
	return c.JSON(fiber.Map{
		"config": fiber.Map{
			"baseBackoffSeconds":     int(s.shieldedWithdrawalBaseBackoff() / time.Second),
			"batchLimit":             s.cfg.ShieldedWithdrawalBatchLimit,
			"delaySeconds":           int(s.cfg.ShieldedWithdrawalDelay / time.Second),
			"maxAttempts":            s.shieldedWithdrawalMaxAttempts(),
			"mode":                   s.cfg.ShieldedWithdrawalMode,
			"paused":                 paused,
			"staleProcessingSeconds": int(s.shieldedWithdrawalStaleProcessing() / time.Second),
			"supportedPayoutPools":   s.shieldedPayoutPools(),
		},
		"withdrawals": withdrawals,
	})
}

func (s Server) adminShieldedWithdrawalQueue(c *fiber.Ctx) error {
	paused, err := s.shieldedWithdrawalQueuePaused(c.Context())
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load shielded withdrawal queue")
	}
	return c.JSON(fiber.Map{
		"paused": paused,
		"config": fiber.Map{
			"baseBackoffSeconds":     int(s.shieldedWithdrawalBaseBackoff() / time.Second),
			"batchLimit":             s.cfg.ShieldedWithdrawalBatchLimit,
			"delaySeconds":           int(s.cfg.ShieldedWithdrawalDelay / time.Second),
			"maxAttempts":            s.shieldedWithdrawalMaxAttempts(),
			"pollIntervalSeconds":    int(s.cfg.ShieldedWithdrawalPollInterval / time.Second),
			"staleProcessingSeconds": int(s.shieldedWithdrawalStaleProcessing() / time.Second),
		},
	})
}

func (s Server) adminPauseShieldedWithdrawalQueue(c *fiber.Ctx) error {
	if err := s.store.SetSystemSetting(c.Context(), shieldedWithdrawalQueuePausedSetting, "true"); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to pause shielded withdrawal queue")
	}
	_, _ = s.store.CreateAdminActivity(c.Context(), adminActivity(c, s.adminActor(c), "pause_shielded_queue", "shielded_withdrawal_queue", "", "Shielded queue paused", "Automatic shielded withdrawal processing was paused."))
	return c.JSON(fiber.Map{"paused": true})
}

func (s Server) adminResumeShieldedWithdrawalQueue(c *fiber.Ctx) error {
	if err := s.store.SetSystemSetting(c.Context(), shieldedWithdrawalQueuePausedSetting, "false"); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to resume shielded withdrawal queue")
	}
	_, _ = s.store.CreateAdminActivity(c.Context(), adminActivity(c, s.adminActor(c), "resume_shielded_queue", "shielded_withdrawal_queue", "", "Shielded queue resumed", "Automatic shielded withdrawal processing was resumed."))
	return c.JSON(fiber.Map{"paused": false})
}

func (s Server) adminRetryShieldedWithdrawal(c *fiber.Ctx) error {
	withdrawalID := strings.TrimSpace(c.Params("id"))
	if withdrawalID == "" {
		return fiber.NewError(fiber.StatusBadRequest, "withdrawal id is required")
	}
	withdrawal, err := s.store.RetryAnyShieldedWithdrawal(c.Context(), withdrawalID, s.nextShieldedWithdrawalExecutionTime())
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	_, _ = s.store.CreateAdminActivity(c.Context(), adminActivity(c, s.adminActor(c), "retry_shielded_withdrawal", "shielded_withdrawal", withdrawal.ID, "Shielded withdrawal retry queued", "Admin queued a retry for "+shortHashForAdmin(withdrawal.NullifierHash)+"."))
	return c.JSON(fiber.Map{"withdrawal": withdrawal})
}

func (s Server) adminMarkShieldedWithdrawalSuspicious(c *fiber.Ctx) error {
	withdrawalID := strings.TrimSpace(c.Params("id"))
	if withdrawalID == "" {
		return fiber.NewError(fiber.StatusBadRequest, "withdrawal id is required")
	}
	var request AdminShieldedWithdrawalActionRequest
	_ = c.BodyParser(&request)
	withdrawal, err := s.store.MarkShieldedWithdrawalSuspicious(c.Context(), withdrawalID, request.Reason)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	_, _ = s.store.CreateAdminActivity(c.Context(), adminActivity(c, s.adminActor(c), "mark_shielded_withdrawal_suspicious", "shielded_withdrawal", withdrawal.ID, "Shielded withdrawal marked suspicious", strings.TrimSpace(request.Reason)))
	return c.JSON(fiber.Map{"withdrawal": withdrawal})
}

func (s Server) adminProcessShieldedWithdrawal(c *fiber.Ctx) error {
	withdrawalID := strings.TrimSpace(c.Params("id"))
	if withdrawalID == "" {
		return fiber.NewError(fiber.StatusBadRequest, "withdrawal id is required")
	}
	if !s.shieldedPayoutConfigured() {
		return fiber.NewError(fiber.StatusBadGateway, "shielded payout pool is not configured")
	}
	if _, err := s.store.RetryAnyShieldedWithdrawal(c.Context(), withdrawalID, time.Now().UTC()); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	withdrawal, ok, err := s.store.MarkShieldedWithdrawalProcessing(c.Context(), withdrawalID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to lock shielded withdrawal")
	}
	if !ok {
		return fiber.NewError(fiber.StatusConflict, "shielded withdrawal cannot be processed right now")
	}
	completed, err := s.completeShieldedWithdrawalExecution(c.Context(), withdrawal)
	if err != nil {
		_, _ = s.store.CreateAdminActivity(c.Context(), adminActivity(c, s.adminActor(c), "force_process_shielded_withdrawal_failed", "shielded_withdrawal", withdrawal.ID, "Shielded withdrawal force process failed", err.Error()))
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	_, _ = s.store.CreateAdminActivity(c.Context(), adminActivity(c, s.adminActor(c), "force_process_shielded_withdrawal", "shielded_withdrawal", completed.ID, "Shielded withdrawal force processed", "Status: "+completed.Status+"."))
	return c.JSON(fiber.Map{"withdrawal": completed})
}

func (s Server) executeShieldedWithdrawal(ctx context.Context, withdrawal store.ShieldedWithdrawal) (shieldedWithdrawalExecutionResult, error) {
	mode := strings.ToLower(strings.TrimSpace(withdrawal.Mode))
	if mode == "" {
		mode = strings.ToLower(strings.TrimSpace(s.cfg.ShieldedWithdrawalMode))
	}
	relayer := strings.TrimSpace(withdrawal.Relayer)
	if relayer == "" {
		relayer = s.shieldedWithdrawalRelayerAddress()
	}
	relayerFee := strings.TrimSpace(withdrawal.RelayerFee)
	if relayerFee == "" {
		relayerFee = s.shieldedWithdrawalRelayerFee()
	}
	functionName := "withdrawWithZKVerify"
	args := []string{withdrawal.NoteCommitment, withdrawal.NullifierHash, withdrawal.ZKProofSubmissionRef, withdrawal.Recipient, relayer, relayerFee}
	poolAddress := strings.TrimSpace(withdrawal.PoolAddress)
	if poolAddress == "" {
		poolAddress = strings.TrimSpace(s.cfg.ShieldedPayoutPoolAddress)
	}
	if mode == "verified" {
		if !isHexBytes(withdrawal.SolidityProof) {
			return shieldedWithdrawalExecutionResult{}, errors.New("stored solidity proof is invalid")
		}
		if len(strings.TrimSpace(withdrawal.PublicSignals)) == 0 {
			return shieldedWithdrawalExecutionResult{}, errors.New("stored public signals are missing")
		}
		functionName = "withdrawVerified"
		args = []string{withdrawal.NoteCommitment, withdrawal.NullifierHash, withdrawal.Recipient, relayer, relayerFee, withdrawal.SolidityProof, withdrawal.PublicSignals}
	} else {
		submission, err := s.gmrEngine.ZKProofSubmission(ctx, withdrawal.ZKProofSubmissionID)
		if err != nil {
			return shieldedWithdrawalExecutionResult{}, err
		}
		if err := s.validateShieldedWithdrawalProof(submission, withdrawal.NoteCommitment, withdrawal.NullifierHash, withdrawal.Recipient); err != nil {
			return shieldedWithdrawalExecutionResult{}, err
		}
		if strings.TrimSpace(withdrawal.ZKProofSubmissionRef) == "" {
			args[2] = privateClaimTextToBytes32(withdrawal.ZKProofSubmissionID)
		}
	}
	result, err := s.gmrEngine.ContractWrite(ctx, gmrengine.ContractWriteRequest{
		ABI:             shieldedPayoutPoolABI,
		Args:            args,
		ChainID:         s.cfg.ShieldedPayoutPoolChainID,
		ContractAddress: poolAddress,
		FunctionName:    functionName,
	})
	if err != nil {
		return shieldedWithdrawalExecutionResult{}, err
	}
	status := result.Transaction.Status
	if status == "" {
		status = "submitted"
	}
	transactionHash := result.Transaction.TransactionHash
	if s.cfg.ShieldedPayoutRequired {
		transaction, err := s.waitForGMRTransaction(ctx, result.Transaction.ID, s.shieldedPayoutWaitTimeout())
		if err != nil {
			return shieldedWithdrawalExecutionResult{Status: "failed", TransactionID: result.Transaction.ID, TransactionHash: transactionHash}, err
		}
		status = transaction.Status
		transactionHash = transaction.TransactionHash
	}
	return shieldedWithdrawalExecutionResult{
		Status:          status,
		TransactionID:   result.Transaction.ID,
		TransactionHash: transactionHash,
	}, nil
}

func (s Server) shieldedPayoutPools() []fiber.Map {
	pools := []fiber.Map{}
	for _, pool := range s.cfg.ShieldedPayoutPools {
		pools = append(pools, fiber.Map{
			"denomination": pool.Denomination,
			"poolAddress":  strings.ToLower(strings.TrimSpace(pool.PoolAddress)),
		})
	}
	return pools
}

func (s Server) creditShieldedPayout(ctx context.Context, payoutAmount string, noteCommitment string) (shieldedPayoutCreditResult, error) {
	if !s.shieldedPayoutConfigured() {
		return shieldedPayoutCreditResult{}, errors.New("shielded payout pool is not configured")
	}
	noteCommitment = strings.ToLower(strings.TrimSpace(noteCommitment))
	if !isBytes32Hex(noteCommitment) {
		return shieldedPayoutCreditResult{}, errors.New("shieldedNoteCommitment must be a 32-byte hex string")
	}
	payoutQuantity, err := thirdweb.TokenQuantity(payoutAmount, s.cfg.WelcomeTokenDecimals)
	if err != nil {
		return shieldedPayoutCreditResult{}, err
	}
	poolAddress, denomination, ok := s.shieldedPayoutPoolForDenomination(payoutQuantity)
	if !ok {
		return shieldedPayoutCreditResult{}, fmt.Errorf("claim payout %s base units does not match any configured shielded pool denomination", payoutQuantity)
	}

	approveResult, err := s.gmrEngine.ContractWrite(ctx, gmrengine.ContractWriteRequest{
		ABI:             erc20ApproveABI,
		Args:            []string{poolAddress, denomination},
		ChainID:         s.cfg.ShieldedPayoutPoolChainID,
		ContractAddress: s.cfg.WelcomeTokenContract,
		FunctionName:    "approve",
	})
	if err != nil {
		return shieldedPayoutCreditResult{}, err
	}
	transactionIDs := []string{approveResult.Transaction.ID}

	if _, err := s.waitForGMRTransaction(ctx, approveResult.Transaction.ID, s.shieldedPayoutWaitTimeout()); err != nil {
		return shieldedPayoutCreditResult{PayoutError: err.Error(), PayoutStatus: "failed", TransactionIDs: transactionIDs}, err
	}

	depositResult, err := s.gmrEngine.ContractWrite(ctx, gmrengine.ContractWriteRequest{
		ABI:             shieldedPayoutPoolABI,
		Args:            []string{noteCommitment},
		ChainID:         s.cfg.ShieldedPayoutPoolChainID,
		ContractAddress: poolAddress,
		FunctionName:    "depositAndCredit",
	})
	if err != nil {
		return shieldedPayoutCreditResult{PayoutError: err.Error(), PayoutStatus: "failed", TransactionIDs: transactionIDs}, err
	}
	transactionIDs = append(transactionIDs, depositResult.Transaction.ID)
	status := depositResult.Transaction.Status
	if status == "" {
		status = "queued"
	}
	if s.cfg.ShieldedPayoutRequired {
		transaction, err := s.waitForGMRTransaction(ctx, depositResult.Transaction.ID, s.shieldedPayoutWaitTimeout())
		if err != nil {
			return shieldedPayoutCreditResult{PayoutError: err.Error(), PayoutStatus: "failed", TransactionIDs: transactionIDs}, err
		}
		status = transaction.Status
		if strings.EqualFold(status, "confirmed") {
			status = "sent"
		}
	}
	return shieldedPayoutCreditResult{PayoutStatus: status, TransactionIDs: transactionIDs}, nil
}

func (s Server) shieldedPayoutPoolForDenomination(denomination string) (string, string, bool) {
	denomination = strings.TrimSpace(denomination)
	for _, pool := range s.cfg.ShieldedPayoutPools {
		if strings.TrimSpace(pool.Denomination) == denomination {
			return strings.ToLower(strings.TrimSpace(pool.PoolAddress)), strings.TrimSpace(pool.Denomination), true
		}
	}
	if denomination != "" && denomination == strings.TrimSpace(s.cfg.ShieldedPayoutDenomination) && strings.TrimSpace(s.cfg.ShieldedPayoutPoolAddress) != "" {
		return strings.ToLower(strings.TrimSpace(s.cfg.ShieldedPayoutPoolAddress)), strings.TrimSpace(s.cfg.ShieldedPayoutDenomination), true
	}
	return "", "", false
}

func (s Server) validateShieldedWithdrawalProof(submission gmrengine.ZKProofSubmission, noteCommitment string, nullifierHash string, recipient string) error {
	if submission.Status != "submitted" && submission.Status != "finalized" {
		return errors.New("zk proof submission is not verified yet")
	}
	poolAddress, denomination, err := s.validateShieldedWithdrawalPublicSignals(json.RawMessage(submission.PublicSignals), noteCommitment, nullifierHash, recipient)
	if err != nil {
		return err
	}
	expectedRaw, err := s.shieldedWithdrawalProofContext(noteCommitment, nullifierHash, recipient, poolAddress, denomination)
	if err != nil {
		return err
	}
	var expected map[string]any
	var actual map[string]any
	if err := json.Unmarshal(expectedRaw, &expected); err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(submission.Context)), &actual); err != nil {
		return errors.New("zk proof context is missing or invalid")
	}
	for _, key := range []string{"chainId", "circuit", "circuitVersion", "denomination", "kind", "noteCommitment", "nullifierHash", "poolAddress", "recipient", "tokenAddress"} {
		if fmt.Sprint(actual[key]) != fmt.Sprint(expected[key]) {
			return fmt.Errorf("zk proof context %s does not match this withdrawal", key)
		}
	}
	return nil
}

func (s Server) validateShieldedWithdrawalPublicSignals(raw json.RawMessage, noteCommitment string, nullifierHash string, recipient string) (string, string, error) {
	signals, err := parsePrivateClaimPublicSignals(raw)
	if err != nil {
		return "", "", err
	}
	expected := []string{
		bytes32ToFieldString(noteCommitment),
		bytes32ToFieldString(nullifierHash),
		evmAddressToFieldString(recipient),
		fmt.Sprintf("%d", s.cfg.ShieldedPayoutPoolChainID),
		evmAddressToFieldString(s.cfg.WelcomeTokenContract),
	}
	if len(signals) < 7 {
		return "", "", errors.New("zk proof public signals are missing shielded withdrawal fields")
	}
	for index, value := range expected {
		if !sameFieldElement(signals[index], value) {
			return "", "", fmt.Errorf("zk proof public signal %d does not match this withdrawal", index)
		}
	}
	poolAddress, denomination, ok := s.shieldedPayoutPoolFromPublicSignalFields(signals[5], signals[6])
	if !ok {
		return "", "", errors.New("zk proof public signals do not match a configured shielded payout pool")
	}
	return poolAddress, denomination, nil
}

func (s Server) shieldedPayoutPoolFromPublicSignalFields(poolAddressField string, denominationField string) (string, string, bool) {
	for _, pool := range s.cfg.ShieldedPayoutPools {
		if sameFieldElement(poolAddressField, evmAddressToFieldString(pool.PoolAddress)) && sameFieldElement(denominationField, strings.TrimSpace(pool.Denomination)) {
			return strings.ToLower(strings.TrimSpace(pool.PoolAddress)), strings.TrimSpace(pool.Denomination), true
		}
	}
	return "", "", false
}

func (s Server) shieldedWithdrawalProofContext(noteCommitment string, nullifierHash string, recipient string, poolAddress string, denomination string) (json.RawMessage, error) {
	payload := fiber.Map{
		"chainId":        s.cfg.ShieldedPayoutPoolChainID,
		"circuit":        "shielded_withdrawal",
		"circuitVersion": "budol-shielded-withdrawal-v1",
		"denomination":   strings.TrimSpace(denomination),
		"kind":           "budol-shielded-withdrawal",
		"noteCommitment": strings.ToLower(strings.TrimSpace(noteCommitment)),
		"nullifierHash":  strings.ToLower(strings.TrimSpace(nullifierHash)),
		"poolAddress":    strings.ToLower(strings.TrimSpace(poolAddress)),
		"recipient":      strings.ToLower(strings.TrimSpace(recipient)),
		"tokenAddress":   strings.ToLower(strings.TrimSpace(s.cfg.WelcomeTokenContract)),
	}
	return json.Marshal(payload)
}

func (s Server) shieldedPayoutWaitTimeout() time.Duration {
	if s.cfg.ShieldedPayoutConfirmTimeout > 0 {
		return s.cfg.ShieldedPayoutConfirmTimeout
	}
	return 90 * time.Second
}

func (s Server) shieldedWithdrawalRelayerAddress() string {
	relayer := strings.TrimSpace(s.cfg.ShieldedWithdrawalRelayerAddress)
	if relayer == "" {
		return "0x0000000000000000000000000000000000000000"
	}
	return relayer
}

func (s Server) shieldedWithdrawalRelayerFee() string {
	fee := strings.TrimSpace(s.cfg.ShieldedWithdrawalRelayerFee)
	if fee == "" {
		return "0"
	}
	return fee
}

func (s Server) shieldedWithdrawalMaxAttempts() int64 {
	if s.cfg.ShieldedWithdrawalMaxAttempts <= 0 {
		return 3
	}
	return int64(s.cfg.ShieldedWithdrawalMaxAttempts)
}

func (s Server) shieldedWithdrawalBaseBackoff() time.Duration {
	if s.cfg.ShieldedWithdrawalBaseBackoff <= 0 {
		return 2 * time.Minute
	}
	return s.cfg.ShieldedWithdrawalBaseBackoff
}

func (s Server) shieldedWithdrawalStaleProcessing() time.Duration {
	if s.cfg.ShieldedWithdrawalStaleProcessing <= 0 {
		return 10 * time.Minute
	}
	return s.cfg.ShieldedWithdrawalStaleProcessing
}

func (s Server) shieldedWithdrawalRetryTime(attempts int64) time.Time {
	if attempts <= 0 {
		attempts = 1
	}
	delay := s.shieldedWithdrawalBaseBackoff()
	for i := int64(1); i < attempts && delay < time.Hour; i++ {
		delay *= 2
	}
	if delay > time.Hour {
		delay = time.Hour
	}
	return time.Now().UTC().Add(delay)
}

func (s Server) shieldedWithdrawalQueuePaused(ctx context.Context) (bool, error) {
	value, ok, err := s.store.GetSystemSetting(ctx, shieldedWithdrawalQueuePausedSetting)
	if err != nil || !ok {
		return false, err
	}
	return strings.EqualFold(strings.TrimSpace(value), "true"), nil
}

func shortHashForAdmin(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 14 {
		return value
	}
	return value[:8] + "..." + value[len(value)-6:]
}

func (s Server) nextShieldedWithdrawalExecutionTime() time.Time {
	delay := s.cfg.ShieldedWithdrawalDelay
	if delay < 0 {
		delay = 0
	}
	jitter := time.Duration(0)
	if delay > 0 {
		maxJitter := int64(delay / 2)
		if maxJitter < int64(time.Second) {
			maxJitter = int64(time.Second)
		}
		randomValue, err := rand.Int(rand.Reader, big.NewInt(maxJitter+1))
		if err == nil {
			jitter = time.Duration(randomValue.Int64())
		}
	}
	return time.Now().UTC().Add(delay + jitter)
}

func (s Server) startShieldedWithdrawalWorker(ctx context.Context) {
	if !s.shieldedPayoutConfigured() {
		return
	}
	interval := s.cfg.ShieldedWithdrawalPollInterval
	if interval <= 0 {
		interval = 15 * time.Second
	}
	batchLimit := int64(s.cfg.ShieldedWithdrawalBatchLimit)
	if batchLimit <= 0 {
		batchLimit = 5
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			s.processShieldedWithdrawalBatch(ctx, batchLimit)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func (s Server) processShieldedWithdrawalBatch(ctx context.Context, batchLimit int64) {
	paused, err := s.shieldedWithdrawalQueuePaused(ctx)
	if err != nil {
		log.Printf("shielded withdrawal worker pause check error: %v", err)
		return
	}
	if paused {
		return
	}
	staleBefore := time.Now().UTC().Add(-s.shieldedWithdrawalStaleProcessing())
	if recovered, err := s.store.RecoverStaleShieldedWithdrawals(ctx, staleBefore, s.shieldedWithdrawalRetryTime(1), s.shieldedWithdrawalMaxAttempts()); err != nil {
		log.Printf("shielded withdrawal worker stale recovery error: %v", err)
	} else if recovered > 0 {
		log.Printf("shielded withdrawal worker recovered %d stale processing records", recovered)
	}
	withdrawals, err := s.store.ListDueShieldedWithdrawals(ctx, batchLimit, time.Now().UTC())
	if err != nil {
		log.Printf("shielded withdrawal worker list error: %v", err)
		return
	}
	for _, queued := range withdrawals {
		if ctx.Err() != nil {
			return
		}
		withdrawal, ok, err := s.store.MarkShieldedWithdrawalProcessing(ctx, queued.ID)
		if err != nil {
			log.Printf("shielded withdrawal worker lock error: %v", err)
			continue
		}
		if !ok {
			continue
		}
		if _, err := s.completeShieldedWithdrawalExecution(ctx, withdrawal); err != nil {
			log.Printf("shielded withdrawal %s failed: %v", withdrawal.ID, err)
		}
	}
}

func (s Server) completeShieldedWithdrawalExecution(ctx context.Context, withdrawal store.ShieldedWithdrawal) (store.ShieldedWithdrawal, error) {
	result, err := s.executeShieldedWithdrawal(ctx, withdrawal)
	if err != nil {
		if withdrawal.Attempts >= s.shieldedWithdrawalMaxAttempts() {
			completed, recordErr := s.store.CompleteShieldedWithdrawal(ctx, withdrawal.ID, "failed", result.TransactionID, result.TransactionHash, err.Error())
			if recordErr != nil {
				log.Printf("shielded withdrawal worker failure record error: %v", recordErr)
				return store.ShieldedWithdrawal{}, recordErr
			}
			_, _ = s.store.CreateNotification(ctx, withdrawal.UserID, "shielded_withdrawal", "Shielded withdrawal failed", "Your relayed shielded withdrawal failed after max attempts: "+err.Error(), "/portfolio")
			return completed, err
		}
		retryAt := s.shieldedWithdrawalRetryTime(withdrawal.Attempts)
		retry, recordErr := s.store.ScheduleShieldedWithdrawalRetry(ctx, withdrawal.ID, retryAt, err.Error())
		if recordErr != nil {
			log.Printf("shielded withdrawal worker retry record error: %v", recordErr)
			return store.ShieldedWithdrawal{}, recordErr
		}
		_, _ = s.store.CreateNotification(ctx, withdrawal.UserID, "shielded_withdrawal", "Shielded withdrawal retry scheduled", "Your relayed withdrawal hit an error and will retry automatically.", "/portfolio")
		return retry, err
	}
	completed, err := s.store.CompleteShieldedWithdrawal(ctx, withdrawal.ID, result.Status, result.TransactionID, result.TransactionHash, "")
	if err != nil {
		log.Printf("shielded withdrawal worker completion record error: %v", err)
		return store.ShieldedWithdrawal{}, err
	}
	detail := "Your relayed shielded withdrawal status is " + completed.Status + "."
	if completed.TransactionHash != "" {
		detail += " Transaction " + completed.TransactionHash + "."
	}
	_, _ = s.store.CreateNotification(ctx, completed.UserID, "shielded_withdrawal", "Shielded withdrawal processed", detail, "/portfolio")
	return completed, nil
}

func isBytes32Hex(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 66 || !strings.HasPrefix(value, "0x") {
		return false
	}
	for _, char := range value[2:] {
		if (char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F') {
			continue
		}
		return false
	}
	return true
}

func isHexBytes(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) < 2 || !strings.HasPrefix(value, "0x") || len(value)%2 != 0 {
		return false
	}
	for _, char := range value[2:] {
		if (char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F') {
			continue
		}
		return false
	}
	return true
}

func bytes32ToFieldString(value string) string {
	number, ok := fieldElement(value)
	if !ok {
		return ""
	}
	return number.String()
}

func evmAddressToFieldString(value string) string {
	value = strings.TrimSpace(value)
	if !thirdweb.IsEVMAddress(value) {
		return ""
	}
	number, ok := fieldElement(value)
	if !ok {
		return ""
	}
	return number.String()
}

func shieldedWithdrawalPublicArtifactPath(file string) (string, error) {
	candidates := []string{
		filepath.Join("..", "public", "zk", "shielded-withdrawal", file),
		filepath.Join("public", "zk", "shielded-withdrawal", file),
		filepath.Join("..", "..", "public", "zk", "shielded-withdrawal", file),
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Size() > 0 {
			return candidate, nil
		}
	}
	return "", errors.New("shielded withdrawal artifact is not available")
}

func (s Server) verifyShieldedWithdrawalArtifactChecksum(path string) error {
	if err := s.requireProductionCeremonyMetadata(filepath.Dir(path), "shielded_withdrawal"); err != nil {
		return err
	}
	checksumPath := filepath.Join(filepath.Dir(path), "checksums.sha256")
	rawChecksums, err := os.ReadFile(checksumPath)
	if err != nil {
		if s.cfg.IsProduction() {
			return errors.New("shielded withdrawal artifact checksums are required in production")
		}
		return nil
	}
	expected := ""
	fileName := filepath.Base(path)
	for _, line := range strings.Split(string(rawChecksums), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == fileName {
			expected = strings.ToLower(strings.TrimSpace(fields[0]))
			break
		}
	}
	if expected == "" {
		return fmt.Errorf("shielded withdrawal artifact checksum is missing for %s", fileName)
	}
	artifactBytes, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(artifactBytes)
	actual := fmt.Sprintf("%x", sum[:])
	if actual != expected {
		return fmt.Errorf("shielded withdrawal artifact checksum mismatch for %s", fileName)
	}
	return nil
}

func (s Server) requireProductionCeremonyMetadata(dir string, circuit string) error {
	if !s.cfg.IsProduction() {
		return nil
	}
	raw, err := os.ReadFile(filepath.Join(dir, "ceremony.json"))
	if err != nil {
		return fmt.Errorf("%s ceremony metadata is required in production", circuit)
	}
	var metadata struct {
		Circuit         string `json:"circuit"`
		ProductionReady bool   `json:"productionReady"`
		Ceremony        string `json:"ceremony"`
		FinalZKeyHash   string `json:"finalZKeyHash"`
	}
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return fmt.Errorf("%s ceremony metadata is invalid", circuit)
	}
	if metadata.Circuit != circuit {
		return fmt.Errorf("%s ceremony metadata circuit mismatch", circuit)
	}
	if !metadata.ProductionReady {
		return fmt.Errorf("%s proving artifacts are marked as development artifacts", circuit)
	}
	if strings.TrimSpace(metadata.Ceremony) == "" || strings.TrimSpace(metadata.FinalZKeyHash) == "" {
		return fmt.Errorf("%s ceremony metadata must include ceremony and finalZKeyHash", circuit)
	}
	return nil
}
