package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/techbudol1/budol-api/internal/gmrengine"
	"github.com/techbudol1/budol-api/internal/store"
	"github.com/techbudol1/budol-api/internal/walletops"

	"github.com/gofiber/fiber/v2"
)

type PrivateClaimRequest struct {
	NullifierHash            string                        `json:"nullifierHash"`
	PrivacyReceiptCommitment string                        `json:"privacyReceiptCommitment"`
	PrivacyReceiptTxHash     string                        `json:"privacyReceiptTxHash"`
	Root                     string                        `json:"root"`
	ShieldedNoteCommitment   string                        `json:"shieldedNoteCommitment"`
	ShieldedNoteCommitments  []ShieldedNoteCommitmentInput `json:"shieldedNoteCommitments"`
	TradeID                  string                        `json:"tradeId"`
	ZKProofSubmissionID      string                        `json:"zkProofSubmissionId"`
}

type PrivateClaimProofSubmissionRequest struct {
	Context                  json.RawMessage `json:"context"`
	DomainID                 int64           `json:"domainId"`
	NullifierHash            string          `json:"nullifierHash"`
	PrivacyReceiptCommitment string          `json:"privacyReceiptCommitment"`
	PrivacyReceiptTxHash     string          `json:"privacyReceiptTxHash"`
	Proof                    json.RawMessage `json:"proof"`
	ProofSystem              string          `json:"proofSystem"`
	PublicSignals            json.RawMessage `json:"publicSignals"`
	TradeID                  string          `json:"tradeId"`
	VK                       json.RawMessage `json:"vk"`
}

type ManagedPrivacyFeeRequest struct {
	Kind string `json:"kind"`
}

const (
	privacyFeeHidePosition   = "hide_position"
	privacyFeePrivateClaim   = "private_claim"
	privacyFeeShieldedPayout = "shielded_payout"
)

const (
	privacyFeeCollectorSetting      = "zen_privacy_access_fee_collector_address"
	privacyHidePositionFeeSetting   = "zen_hide_position_fee"
	privacyPrivateClaimFeeSetting   = "zen_private_claim_fee"
	privacyShieldedPayoutFeeSetting = "zen_shielded_payout_fee"
)

type privateClaimScriptResponse struct {
	Note store.PrivateClaimNote `json:"note"`
	Tree struct {
		Leaves []string `json:"leaves"`
		Root   string   `json:"root"`
	} `json:"tree"`
}

func (s Server) pollPrivateClaimTree(c *fiber.Ctx) error {
	poll, ok, err := s.store.GetPollBySlug(c.Context(), c.Params("slug"), true)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load poll")
	}
	if !ok {
		return fiber.NewError(fiber.StatusNotFound, "poll not found")
	}
	tree, err := s.store.PrivateClaimTree(c.Context(), poll.ID)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	tree, err = s.privateClaimTreeWithRoot(c.Context(), tree, "tree_requested")
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	return c.JSON(fiber.Map{"tree": tree})
}

func (s Server) privacyAccessConfig(c *fiber.Ctx) error {
	collector := s.privacyFeeCollectorAddress(c.Context())
	return c.JSON(fiber.Map{
		"chainId":          s.cfg.WelcomeTokenChainID,
		"collectorAddress": strings.ToLower(collector),
		"currency":         "tZEN",
		"decimals":         18,
		"mode":             "native",
		"fees": fiber.Map{
			privacyFeeHidePosition:   s.privacyFeeAmount(c.Context(), privacyFeeHidePosition),
			privacyFeePrivateClaim:   s.privacyFeeAmount(c.Context(), privacyFeePrivateClaim),
			privacyFeeShieldedPayout: s.privacyFeeAmount(c.Context(), privacyFeeShieldedPayout),
		},
		"limitations": []string{
			"hide_position_fee is configured but not enforced until public profile/trade feeds exist",
			"private claim fee is enforced before ZK proof submission",
			"shielded payout fee is enforced before a shielded payout pool note is credited",
		},
	})
}

func (s Server) createManagedPrivacyAccessFee(c *fiber.Ctx) error {
	user, err := s.authenticatedUser(c)
	if err != nil {
		return err
	}
	if !strings.EqualFold(strings.TrimSpace(user.WalletCustody), "managed") {
		return fiber.NewError(fiber.StatusForbidden, "managed privacy fee is only available for managed wallets")
	}
	if s.gmrEngine == nil || !s.gmrEngine.Configured() {
		return fiber.NewError(fiber.StatusServiceUnavailable, "GMR Engine is not configured")
	}
	var request ManagedPrivacyFeeRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	kind := strings.TrimSpace(request.Kind)
	if kind != privacyFeePrivateClaim && kind != privacyFeeShieldedPayout && kind != privacyFeeHidePosition {
		return fiber.NewError(fiber.StatusBadRequest, "valid privacy fee kind is required")
	}
	amountRaw := s.privacyFeeAmount(c.Context(), kind)
	amount, ok := new(big.Int).SetString(strings.TrimSpace(amountRaw), 10)
	if !ok || amount.Sign() < 0 {
		return fiber.NewError(fiber.StatusInternalServerError, "privacy access fee is misconfigured")
	}
	if amount.Sign() == 0 {
		return c.JSON(fiber.Map{"amountRaw": amountRaw, "kind": kind, "skipped": true, "transactionHash": ""})
	}
	collector := s.privacyFeeCollectorAddress(c.Context())
	if !walletops.IsEVMAddress(collector) {
		return fiber.NewError(fiber.StatusServiceUnavailable, "privacy fee collector is not configured")
	}
	result, err := s.gmrEngine.TransferManagedNative(c.Context(), gmrengine.ManagedNativeTransferRequest{
		AmountRaw: amount.String(),
		ChainID:   s.cfg.WelcomeTokenChainID,
		Owner:     user.WalletAddress,
		Recipient: collector,
	})
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	txHash := strings.TrimSpace(result.TransactionHash)
	if txHash == "" && len(result.TransactionIDs) > 0 {
		txHash = strings.TrimSpace(result.TransactionIDs[len(result.TransactionIDs)-1])
	}
	if txHash == "" {
		return fiber.NewError(fiber.StatusBadGateway, "GMR Engine did not return a managed privacy fee transaction hash")
	}
	if err := s.verifyPrivacyAccessFee(c, user, txHash, kind); err != nil {
		return err
	}
	_, _ = s.store.CreateNotification(c.Context(), user.ID, "privacy_fee", "Privacy fee paid", "Paid tZEN for "+strings.ReplaceAll(kind, "_", " ")+".", "/portfolio")
	return c.JSON(fiber.Map{
		"amountRaw":        amount.String(),
		"collectorAddress": strings.ToLower(collector),
		"currency":         "tZEN",
		"kind":             kind,
		"managed":          true,
		"transactionHash":  txHash,
		"transactionIds":   result.TransactionIDs,
	})
}

func (s Server) privateClaimArtifact(c *fiber.Ctx) error {
	file := strings.TrimSpace(c.Params("file"))
	switch file {
	case "private_winning_claim.wasm", "private_winning_claim_final.zkey", "verification_key.json":
	default:
		return fiber.NewError(fiber.StatusNotFound, "private claim artifact not found")
	}
	path, err := privateClaimPublicArtifactPath(file)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	}
	if err := s.verifyPrivateClaimArtifactChecksum(path); err != nil {
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

func (s Server) claimPublicPayout(c *fiber.Ctx) error {
	user, err := s.authenticatedUser(c)
	if err != nil {
		return err
	}
	tradeID := strings.TrimSpace(c.Params("id"))
	if tradeID == "" {
		return fiber.NewError(fiber.StatusBadRequest, "trade id is required")
	}
	trade, err := s.store.ReserveDirectTradePayout(c.Context(), user.ID, tradeID)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	result, _, payoutErr := s.sendWalletTokensWithRelease(c, []walletRecipientInput{
		{Address: user.WalletAddress, Amount: settlementAmountString(trade.SettlementPayout)},
	}, trade.SettlementPayout)
	if payoutErr != nil {
		_, _ = s.store.CompleteDirectTradePayout(c.Context(), user.ID, trade.ID, nil, "failed", payoutErr.Error())
		return fiber.NewError(fiber.StatusBadGateway, payoutErr.Error())
	}
	payoutStatus, payoutError := s.waitForWalletOpsTransactionStatus(c, result.TransactionIDs)
	trade, err = s.store.CompleteDirectTradePayout(c.Context(), user.ID, trade.ID, result.TransactionIDs, payoutStatus, payoutError)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to record public payout")
	}
	_, _ = s.store.CreateNotification(c.Context(), user.ID, "public_claim", "BUDOL payout claimed", trade.PollTitle+": "+settlementAmountString(trade.SettlementPayout)+" BUDOL direct payout status is "+payoutStatus+".", "/portfolio")
	portfolio, err := s.store.UserPortfolio(c.Context(), user.ID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load portfolio")
	}
	return c.JSON(fiber.Map{
		"portfolio":      portfolio,
		"trade":          trade,
		"payoutStatus":   payoutStatus,
		"payoutError":    payoutError,
		"payoutMode":     "direct",
		"transactionIds": result.TransactionIDs,
	})
}

func (s Server) claimPrivatePayout(c *fiber.Ctx) error {
	user, err := s.authenticatedUser(c)
	if err != nil {
		return err
	}
	var request PrivateClaimRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	if strings.TrimSpace(request.TradeID) == "" {
		return fiber.NewError(fiber.StatusBadRequest, "tradeId is required")
	}
	if strings.TrimSpace(request.ZKProofSubmissionID) == "" {
		return fiber.NewError(fiber.StatusBadRequest, "zkProofSubmissionId is required before a private claim can be paid")
	}
	if _, ok := fieldElement(request.NullifierHash); !ok {
		return fiber.NewError(fiber.StatusBadRequest, "nullifierHash is required")
	}

	trade, err := s.store.GetTradeByID(c.Context(), request.TradeID)
	if err != nil || trade.UserID != user.ID {
		return fiber.NewError(fiber.StatusNotFound, "trade not found")
	}
	if _, ok := fieldElement(trade.PrivateClaimLeaf); !ok {
		return fiber.NewError(fiber.StatusBadRequest, "trade has no private claim commitment")
	}
	expectedOutcome, err := privateClaimOutcomeForSide(trade.Side)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	tree, err := s.store.PrivateClaimTree(c.Context(), trade.PollID)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	tree, err = s.privateClaimTreeWithRoot(c.Context(), tree, "claim_validation")
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	root := tree.Root
	if strings.TrimSpace(request.Root) != "" && strings.TrimSpace(request.Root) != root {
		return fiber.NewError(fiber.StatusBadRequest, "claim root does not match the current market root")
	}
	submission, err := s.gmrEngine.ZKProofSubmission(c.Context(), request.ZKProofSubmissionID)
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	nullifierHash := strings.TrimSpace(request.NullifierHash)
	if err := validatePrivateClaimProof(submission, root, expectedOutcome, nullifierHash); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if err := s.validatePrivateClaimProofContext(c.Context(), submission, trade, root, nullifierHash); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if err := s.requirePrivateClaimRegistryForStrictClaims(); err != nil {
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	if err := s.requireShieldedPayoutForStrictClaims(); err != nil {
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	payoutAmount := settlementAmountString(trade.SettlementPayout)
	shieldedPayout := false
	directPayoutFallback := false
	if s.shieldedPayoutConfigured() {
		_, supported, err := s.shieldedPayoutPlanForAmount(payoutAmount)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		switch {
		case supported:
			shieldedPayout = true
		case s.cfg.ShieldedPayoutDirectFallback:
			directPayoutFallback = true
		default:
			return fiber.NewError(fiber.StatusUnprocessableEntity, "this payout amount is not supported by a configured shielded pool")
		}
	}
	if shieldedPayout && len(request.ShieldedNoteCommitments) == 0 {
		if isBytes32Hex(request.ShieldedNoteCommitment) {
			request.ShieldedNoteCommitments = []ShieldedNoteCommitmentInput{{Commitment: request.ShieldedNoteCommitment}}
		} else {
			return fiber.NewError(fiber.StatusBadRequest, "shieldedNoteCommitments are required for shielded payouts")
		}
	}
	if shieldedPayout {
		if err := s.verifyPrivacyAccessFee(c, user, request.PrivacyReceiptTxHash, privacyFeeShieldedPayout); err != nil {
			return err
		}
	}
	claim, reservedTrade, err := s.store.ReservePrivateClaim(c.Context(), user.ID, request.TradeID, trade.PrivateClaimLeaf, root, nullifierHash, request.ZKProofSubmissionID)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	registryTransactionID := claim.RegistryTransactionID
	registryStatus := claim.RegistryStatus
	if registryTransactionID == "" || strings.EqualFold(registryStatus, "failed") {
		var registryErr error
		registryTransactionID, registryStatus, registryErr = s.registerPrivateClaimOnRegistry(c.Context(), reservedTrade, root, nullifierHash, request.ZKProofSubmissionID)
		if registryErr != nil {
			claim, reservedTrade, _ = s.store.CompletePrivateClaimPayout(c.Context(), user.ID, claim.ID, nil, "failed", registryErr.Error())
			return fiber.NewError(fiber.StatusBadGateway, registryErr.Error())
		}
		if registryTransactionID != "" {
			claim, reservedTrade, _ = s.store.RecordPrivateClaimRegistryTransaction(c.Context(), user.ID, claim.ID, registryTransactionID, registryStatus, "")
		}
	}

	transactionIDs := []string{}
	payoutStatus := ""
	payoutError := ""
	payoutMode := "direct"
	shieldedPayoutCommitments := []string{}
	if shieldedPayout {
		payoutMode = "shielded"
		creditResult, payoutErr := s.creditShieldedPayoutCollateralized(c.Context(), settlementAmountString(reservedTrade.SettlementPayout), request.ShieldedNoteCommitments, reservedTrade.SettlementPayout)
		transactionIDs = creditResult.TransactionIDs
		payoutStatus = creditResult.PayoutStatus
		payoutError = creditResult.PayoutError
		shieldedPayoutCommitments = creditResult.CreditedCommitments
		if payoutErr != nil {
			claim, reservedTrade, _ = s.store.CompletePrivateClaimPayout(c.Context(), user.ID, claim.ID, transactionIDs, "failed", payoutErr.Error())
			return fiber.NewError(fiber.StatusBadGateway, payoutErr.Error())
		}
		if payoutStatus == "" {
			payoutStatus = "submitted"
		}
	} else {
		result, _, payoutErr := s.sendWalletTokensWithRelease(c, []walletRecipientInput{
			{Address: user.WalletAddress, Amount: settlementAmountString(reservedTrade.SettlementPayout)},
		}, reservedTrade.SettlementPayout)
		if payoutErr != nil {
			claim, reservedTrade, _ = s.store.CompletePrivateClaimPayout(c.Context(), user.ID, claim.ID, nil, "failed", payoutErr.Error())
			return fiber.NewError(fiber.StatusBadGateway, payoutErr.Error())
		}
		transactionIDs = result.TransactionIDs
		payoutStatus, payoutError = s.waitForWalletOpsTransactionStatus(c, transactionIDs)
		if directPayoutFallback {
			payoutMode = "direct_fallback"
		}
	}
	claim, reservedTrade, err = s.store.CompletePrivateClaimPayout(c.Context(), user.ID, claim.ID, transactionIDs, payoutStatus, payoutError)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to record private claim payout")
	}
	notificationDetail := reservedTrade.PollTitle + ": " + settlementAmountString(reservedTrade.SettlementPayout) + " BUDOL payout status is " + payoutStatus + "."
	if shieldedPayout {
		notificationDetail = reservedTrade.PollTitle + ": shielded payout note credited. Store your withdrawal note before withdrawing."
	} else if directPayoutFallback {
		notificationDetail = reservedTrade.PollTitle + ": the private ZK claim was paid directly because no shielded pool supports the exact payout amount."
	}
	_, _ = s.store.CreateNotification(c.Context(), user.ID, "private_claim", "Private claim submitted", notificationDetail, "/portfolio")

	portfolio, err := s.store.UserPortfolio(c.Context(), user.ID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load portfolio")
	}
	return c.JSON(fiber.Map{
		"claim":                     claim,
		"portfolio":                 portfolio,
		"trade":                     reservedTrade,
		"payoutStatus":              payoutStatus,
		"payoutError":               payoutError,
		"payoutMode":                payoutMode,
		"registryTransactionId":     registryTransactionID,
		"registryStatus":            registryStatus,
		"shieldedPayout":            shieldedPayout,
		"shieldedPayoutCommitments": shieldedPayoutCommitments,
		"transactionIds":            transactionIDs,
	})
}

func (s Server) submitPrivateClaimProof(c *fiber.Ctx) error {
	user, err := s.authenticatedUser(c)
	if err != nil {
		return err
	}
	var request PrivateClaimProofSubmissionRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	if strings.TrimSpace(request.TradeID) == "" {
		return fiber.NewError(fiber.StatusBadRequest, "tradeId is required")
	}
	if _, ok := fieldElement(request.NullifierHash); !ok {
		return fiber.NewError(fiber.StatusBadRequest, "nullifierHash is required")
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
	if s.gmrEngine == nil || !s.gmrEngine.Configured() {
		return fiber.NewError(fiber.StatusBadGateway, "GMR Engine is not configured")
	}
	if err := s.verifyPrivacyAccessFee(c, user, request.PrivacyReceiptTxHash, privacyFeePrivateClaim); err != nil {
		return err
	}

	trade, err := s.store.GetTradeByID(c.Context(), request.TradeID)
	if err != nil || trade.UserID != user.ID {
		return fiber.NewError(fiber.StatusNotFound, "trade not found")
	}
	if _, ok := fieldElement(trade.PrivateClaimLeaf); !ok {
		return fiber.NewError(fiber.StatusBadRequest, "trade has no private claim commitment")
	}
	expectedOutcome, err := privateClaimOutcomeForSide(trade.Side)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	tree, err := s.store.PrivateClaimTree(c.Context(), trade.PollID)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	tree, err = s.privateClaimTreeWithRoot(c.Context(), tree, "proof_submission")
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	root := tree.Root
	nullifierHash := strings.TrimSpace(request.NullifierHash)
	if err := validatePrivateClaimPublicSignals(request.PublicSignals, root, expectedOutcome, nullifierHash); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	context, _ := s.privateClaimProofContext(c.Context(), trade, root, nullifierHash)
	domainID := request.DomainID
	if domainID == 0 {
		domainID = 1
	}
	proofSystem := strings.TrimSpace(request.ProofSystem)
	if proofSystem == "" {
		proofSystem = "groth16"
	}
	submission, err := s.gmrEngine.SubmitZKProof(c.Context(), gmrengine.ZKProofSubmitRequest{
		Context:       context,
		DomainID:      domainID,
		Proof:         request.Proof,
		ProofSystem:   proofSystem,
		PublicSignals: request.PublicSignals,
		VK:            request.VK,
	})
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
		"root":       root,
		"submission": submission,
	})
}

func (s Server) privacyFeeAmount(ctx context.Context, kind string) string {
	settingKey := ""
	switch kind {
	case privacyFeeHidePosition:
		settingKey = privacyHidePositionFeeSetting
	case privacyFeePrivateClaim:
		settingKey = privacyPrivateClaimFeeSetting
	case privacyFeeShieldedPayout:
		settingKey = privacyShieldedPayoutFeeSetting
	default:
		return "0"
	}
	if settingKey != "" {
		if value, ok, err := s.store.GetSystemSetting(ctx, settingKey); err == nil && ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	switch kind {
	case privacyFeeHidePosition:
		return strings.TrimSpace(s.cfg.ZENHidePositionFee)
	case privacyFeePrivateClaim:
		return strings.TrimSpace(s.cfg.ZENPrivateClaimFee)
	case privacyFeeShieldedPayout:
		return strings.TrimSpace(s.cfg.ZENShieldedPayoutFee)
	default:
		return "0"
	}
}

func (s Server) privacyFeeCollectorAddress(ctx context.Context) string {
	if value, ok, err := s.store.GetSystemSetting(ctx, privacyFeeCollectorSetting); err == nil && ok && strings.TrimSpace(value) != "" {
		return strings.ToLower(strings.TrimSpace(value))
	}
	if strings.TrimSpace(s.cfg.ZENPrivacyAccessFeeCollectorAddress) != "" {
		return strings.ToLower(strings.TrimSpace(s.cfg.ZENPrivacyAccessFeeCollectorAddress))
	}
	if wallet, err := s.projectWalletAddress(ctx); err == nil {
		return strings.ToLower(strings.TrimSpace(wallet))
	}
	return ""
}

func (s Server) verifyPrivacyAccessFee(c *fiber.Ctx, user store.User, txHash string, kind string) error {
	amount := s.privacyFeeAmount(c.Context(), kind)
	value, ok := new(big.Int).SetString(strings.TrimSpace(amount), 10)
	if !ok || value.Sign() < 0 {
		return fiber.NewError(fiber.StatusInternalServerError, "privacy access fee is misconfigured")
	}
	if value.Sign() == 0 {
		return nil
	}
	collector := s.privacyFeeCollectorAddress(c.Context())
	if !walletops.IsEVMAddress(collector) {
		return fiber.NewError(fiber.StatusServiceUnavailable, "privacy fee collector is not configured")
	}
	if !walletops.IsEVMAddress(user.WalletAddress) {
		return fiber.NewError(fiber.StatusBadRequest, "user wallet address is invalid")
	}
	txHash = strings.TrimSpace(txHash)
	if txHash == "" {
		return fiber.NewError(fiber.StatusPaymentRequired, kind+" tZEN fee transaction is required")
	}
	found, err := s.evm.WaitForNativeTransfer(c.Context(), txHash, user.WalletAddress, collector, value.String())
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, "failed to verify "+kind+" tZEN fee: "+err.Error())
	}
	if !found {
		return fiber.NewError(fiber.StatusPaymentRequired, kind+" tZEN fee was not found in the submitted transaction")
	}
	return nil
}

func (s Server) privateClaimProofContext(ctx context.Context, trade store.Trade, root string, nullifierHash string) (json.RawMessage, error) {
	payload := fiber.Map{
		"chainId":        s.cfg.WelcomeTokenChainID,
		"circuit":        "private_winning_claim",
		"circuitVersion": "budol-private-claim-v1",
		"kind":           "budol-private-claim",
		"nullifierHash":  strings.TrimSpace(nullifierHash),
		"pollId":         trade.PollID,
		"pollSlug":       trade.PollSlug,
		"registry":       strings.ToLower(strings.TrimSpace(s.cfg.PrivateClaimRegistryAddress)),
		"root":           strings.TrimSpace(root),
		"tokenAddress":   strings.ToLower(strings.TrimSpace(s.activeTokenContract(ctx))),
		"tradeId":        trade.ID,
	}
	return json.Marshal(payload)
}

func (s Server) validatePrivateClaimProofContext(ctx context.Context, submission gmrengine.ZKProofSubmission, trade store.Trade, root string, nullifierHash string) error {
	expectedRaw, err := s.privateClaimProofContext(ctx, trade, root, nullifierHash)
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
	for _, key := range []string{"chainId", "circuit", "circuitVersion", "kind", "nullifierHash", "pollId", "pollSlug", "registry", "root", "tokenAddress", "tradeId"} {
		if fmt.Sprint(actual[key]) != fmt.Sprint(expected[key]) {
			return fmt.Errorf("zk proof context %s does not match this claim", key)
		}
	}
	return nil
}

func privateClaimPublicArtifactPath(file string) (string, error) {
	candidates := []string{
		filepath.Join("..", "public", "zk", "private-claim", file),
		filepath.Join("public", "zk", "private-claim", file),
		filepath.Join("..", "..", "public", "zk", "private-claim", file),
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Size() > 0 {
			return candidate, nil
		}
	}
	return "", errors.New("private claim artifact is not available")
}

func (s Server) verifyPrivateClaimArtifactChecksum(path string) error {
	if err := s.requireProductionCeremonyMetadata(filepath.Dir(path), "private_winning_claim"); err != nil {
		return err
	}
	checksumPath := filepath.Join(filepath.Dir(path), "checksums.sha256")
	rawChecksums, err := os.ReadFile(checksumPath)
	if err != nil {
		if s.cfg.IsProduction() {
			return errors.New("private claim artifact checksums are required in production")
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
		return fmt.Errorf("private claim artifact checksum is missing for %s", fileName)
	}
	artifactBytes, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(artifactBytes)
	actual := hex.EncodeToString(sum[:])
	if actual != expected {
		return fmt.Errorf("private claim artifact checksum mismatch for %s", fileName)
	}
	return nil
}

func privateClaimOutcomeForSide(side string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(side)) {
	case "yes":
		return "1", nil
	case "no":
		return "2", nil
	default:
		return "", errors.New("trade side is not eligible for private claim proof")
	}
}

func validatePrivateClaimProof(submission gmrengine.ZKProofSubmission, root string, outcome string, nullifierHash string) error {
	if submission.Status != "submitted" && submission.Status != "finalized" {
		return errors.New("zk proof submission is not verified yet")
	}
	return validatePrivateClaimPublicSignals(json.RawMessage(submission.PublicSignals), root, outcome, nullifierHash)
}

func validatePrivateClaimPublicSignals(raw json.RawMessage, root string, outcome string, nullifierHash string) error {
	signals, err := parsePrivateClaimPublicSignals(raw)
	if err != nil {
		return err
	}
	if len(signals) < 3 {
		return errors.New("zk proof public signals must include root, outcome, and nullifier")
	}
	if !sameFieldElement(signals[0], root) {
		return errors.New("zk proof root does not match the current market root")
	}
	if !sameFieldElement(signals[1], outcome) {
		return errors.New("zk proof outcome does not match the trade outcome")
	}
	if !sameFieldElement(signals[2], nullifierHash) {
		return errors.New("zk proof nullifier does not match the submitted nullifier")
	}
	return nil
}

func parsePrivateClaimPublicSignals(raw json.RawMessage) ([]string, error) {
	rawText := strings.TrimSpace(string(raw))
	if rawText == "" || rawText == "null" {
		return nil, errors.New("zk proof submission has no public signals")
	}
	var list []any
	if err := json.Unmarshal([]byte(rawText), &list); err == nil {
		signals := make([]string, 0, len(list))
		for _, value := range list {
			signals = append(signals, fmt.Sprint(value))
		}
		return signals, nil
	}
	var named struct {
		Root            any `json:"root"`
		ResolvedOutcome any `json:"resolvedOutcome"`
		NullifierHash   any `json:"nullifierHash"`
	}
	if err := json.Unmarshal([]byte(rawText), &named); err != nil {
		return nil, errors.New("zk proof public signals must be a JSON array or object")
	}
	return []string{fmt.Sprint(named.Root), fmt.Sprint(named.ResolvedOutcome), fmt.Sprint(named.NullifierHash)}, nil
}

func sameFieldElement(left string, right string) bool {
	a, okA := fieldElement(left)
	b, okB := fieldElement(right)
	return okA && okB && a.Cmp(b) == 0
}

func fieldElement(value string) (*big.Int, bool) {
	value = strings.Trim(strings.TrimSpace(value), `"`)
	if value == "" || value == "<nil>" {
		return nil, false
	}
	number := new(big.Int)
	if strings.HasPrefix(value, "0x") || strings.HasPrefix(value, "0X") {
		_, ok := number.SetString(value[2:], 16)
		return number, ok
	}
	_, ok := number.SetString(value, 10)
	return number, ok
}

func (s Server) privateClaimMarketRoot(ctx context.Context, leaves []string) (string, error) {
	response, err := runPrivateClaimScript(ctx, map[string]any{
		"action":     "marketRoot",
		"leaves":     leaves,
		"sortLeaves": false,
	})
	if err != nil {
		return "", err
	}
	return response.Tree.Root, nil
}

func (s Server) privateClaimTreeWithRoot(ctx context.Context, tree store.PrivateClaimTree, reason string) (store.PrivateClaimTree, error) {
	root, err := s.privateClaimMarketRoot(ctx, tree.Leaves)
	if err != nil {
		return tree, err
	}
	tree.Root = root
	if snapshot, err := s.store.RecordPrivateClaimRoot(ctx, tree.PollID, root, tree.LeafCount, reason); err == nil && snapshot.ID != "" {
		tree.RootHistory = []store.PrivateClaimRootSnapshot{snapshot}
	}
	if history, err := s.store.PrivateClaimRootHistory(ctx, tree.PollID, 10); err == nil {
		tree.RootHistory = history
	}
	return tree, nil
}

func (s Server) recordPrivateClaimRootForPoll(ctx context.Context, pollID string, reason string) {
	tree, err := s.store.PrivateClaimTree(ctx, pollID)
	if err != nil {
		return
	}
	_, _ = s.privateClaimTreeWithRoot(ctx, tree, reason)
}

func runPrivateClaimScript(ctx context.Context, payload map[string]any) (privateClaimScriptResponse, error) {
	script, err := privateClaimScriptPath()
	if err != nil {
		return privateClaimScriptResponse{}, err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return privateClaimScriptResponse{}, err
	}
	commandCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, "bun", script)
	cmd.Stdin = bytes.NewReader(body)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return privateClaimScriptResponse{}, fmt.Errorf("private claim helper failed: %s", strings.TrimSpace(string(output)))
	}
	trimmed := bytes.TrimSpace(output)
	if len(trimmed) == 0 {
		return privateClaimScriptResponse{}, errors.New("private claim helper returned empty output")
	}
	var response privateClaimScriptResponse
	if err := json.Unmarshal(trimmed, &response); err != nil {
		return privateClaimScriptResponse{}, err
	}
	return response, nil
}

func privateClaimScriptPath() (string, error) {
	candidates := []string{
		filepath.Join("scripts", "private-claim-note.ts"),
		filepath.Join("..", "scripts", "private-claim-note.ts"),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", errors.New("private claim helper script not found")
}
