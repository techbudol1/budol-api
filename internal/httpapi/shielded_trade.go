package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gofiber/fiber/v2"
	"github.com/iden3/go-iden3-crypto/poseidon"
	"github.com/techbudol1/budol-api/internal/config"
	"github.com/techbudol1/budol-api/internal/gmrengine"
	"github.com/techbudol1/budol-api/internal/store"
)

var shieldedTradeVaultABI = json.RawMessage(`[
 {"type":"function","name":"openBatch","stateMutability":"nonpayable","inputs":[{"name":"batchId","type":"bytes32"},{"name":"executeAfter","type":"uint64"},{"name":"expiresAt","type":"uint64"}],"outputs":[]},
 {"type":"function","name":"submitPrivateOrder","stateMutability":"nonpayable","inputs":[{"name":"root","type":"bytes32"},{"name":"nullifierHash","type":"bytes32"},{"name":"orderCommitment","type":"bytes32"},{"name":"batchId","type":"bytes32"},{"name":"pA","type":"uint256[2]"},{"name":"pB","type":"uint256[2][2]"},{"name":"pC","type":"uint256[2]"},{"name":"publicSignals","type":"uint256[9]"}],"outputs":[]},
 {"type":"function","name":"settleBatch","stateMutability":"nonpayable","inputs":[{"name":"batchId","type":"bytes32"}],"outputs":[]}
]`)

var shieldedTradeArtifactChecksums = map[string]string{
	"shielded_trade.wasm":       "6cbc73d689959e35651b6d13e38b45969f5afa49f918a8eb75bd8aa8dc3c0dc9",
	"shielded_trade_final.zkey": "a83a17c1b75c5a1fbd89e6ab704eab01e6e6c4a9d40452a113a0b6932a10d44f",
	"verification_key.json":     "e7df8f217ef5e90e2cc5c2e9c14020443a8e7e1dcd3087d59b4b22259cc1a288",
}

type shieldedTradeProof struct {
	PA [2]string    `json:"pA"`
	PB [2][2]string `json:"pB"`
	PC [2]string    `json:"pC"`
}

type shieldedTradeRequest struct {
	BatchID              string             `json:"batchId"`
	PollID               string             `json:"pollId"`
	Side                 string             `json:"side"`
	Amount               float64            `json:"amount"`
	OrderSalt            string             `json:"orderSalt"`
	NullifierHash        string             `json:"nullifierHash"`
	OrderCommitment      string             `json:"orderCommitment"`
	PrivateClaimLeaf     string             `json:"privateClaimLeaf"`
	PrivacyReceiptTxHash string             `json:"privacyReceiptTxHash"`
	Proof                json.RawMessage    `json:"proof"`
	SolidityProof        shieldedTradeProof `json:"solidityProof"`
	PublicSignals        []string           `json:"publicSignals"`
}

func (s Server) shieldedTradeConfig(c *fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	items := make([]fiber.Map, 0, len(s.cfg.ShieldedTradeVaults))
	feeBps := s.engineTradingFeeBps(c.Context())
	for _, vault := range s.cfg.ShieldedTradeVaults {
		item := fiber.Map{"amount": vault.TradeAmount, "feeBps": vault.FeeBps, "vaultAddress": vault.VaultAddress, "available": vault.FeeBps == feeBps}
		if batch, ok, err := s.store.CurrentShieldedTradeBatch(c.Context(), vault.VaultAddress, time.Now().UTC()); err == nil && ok {
			item["batch"] = batch
		}
		items = append(items, item)
	}
	return c.JSON(fiber.Map{
		"enabled":            s.cfg.ShieldedTradingEnabled,
		"chainId":            s.cfg.WelcomeTokenChainID,
		"tokenAddress":       strings.ToLower(s.activeTokenContract(c.Context())),
		"tokenDecimals":      s.cfg.WelcomeTokenDecimals,
		"treeDepth":          20,
		"zkVerifyDomainId":   s.cfg.ShieldedTradeZKVerifyDomainID,
		"batchWindowSeconds": int(s.cfg.ShieldedTradeBatchWindow / time.Second),
		"operatorPrivacy":    "The testnet relayer sees order details; the public chain sees commitments and batch aggregates.",
		"vaults":             items,
	})
}

func (s Server) shieldedTradeArtifact(c *fiber.Ctx) error {
	file := strings.TrimSpace(c.Params("file"))
	if file != "shielded_trade.wasm" && file != "shielded_trade_final.zkey" && file != "verification_key.json" {
		return fiber.NewError(fiber.StatusNotFound, "shielded trade artifact not found")
	}
	path, err := shieldedTradeArtifactPath(file)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "shielded trade artifact is unreadable")
	}
	digest := sha256.Sum256(contents)
	if hex.EncodeToString(digest[:]) != shieldedTradeArtifactChecksums[file] {
		return fiber.NewError(fiber.StatusInternalServerError, "shielded trade artifact checksum mismatch")
	}
	if strings.HasSuffix(file, ".wasm") {
		c.Type("application/wasm")
	} else {
		c.Type("application/octet-stream")
	}
	if strings.HasSuffix(file, ".json") {
		c.Type("application/json")
	}
	return c.SendFile(path)
}

func shieldedTradeArtifactPath(file string) (string, error) {
	candidates := []string{
		filepath.Join("zk", "shielded-trade", file),
		filepath.Join("..", "zk", "shielded-trade", file),
		filepath.Join("internal", "httpapi", "zk", "shielded-trade", file),
	}
	for _, candidate := range candidates {
		if stat, err := os.Stat(candidate); err == nil && !stat.IsDir() {
			return candidate, nil
		}
	}
	return "", errors.New("shielded trade proving artifact is unavailable")
}

func (s Server) createShieldedTrade(c *fiber.Ctx) error {
	if !s.cfg.ShieldedTradingEnabled {
		return fiber.NewError(fiber.StatusServiceUnavailable, "shielded trading is not enabled")
	}
	user, err := s.authenticatedUser(c)
	if err != nil {
		return err
	}
	var request shieldedTradeRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	request.Side = strings.ToLower(strings.TrimSpace(request.Side))
	if request.Side != "yes" && request.Side != "no" {
		return fiber.NewError(fiber.StatusBadRequest, "side must be yes or no")
	}
	if err := validateFixedTradeAmount(request.Amount); err != nil {
		return err
	}
	if len(request.PublicSignals) != 9 || len(request.Proof) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "complete shielded trade proof is required")
	}
	if _, ok := fieldElement(request.PrivateClaimLeaf); !ok {
		return fiber.NewError(fiber.StatusBadRequest, "privateClaimLeaf is required")
	}

	vault, ok := s.shieldedTradeVaultForAmount(request.Amount, s.engineTradingFeeBps(c.Context()))
	if !ok {
		return fiber.NewError(fiber.StatusConflict, "no shielded vault supports this amount and active fee schedule")
	}
	batch, found, err := s.store.CurrentShieldedTradeBatch(c.Context(), vault.VaultAddress, time.Now().UTC())
	if err != nil || !found || !strings.EqualFold(batch.BatchID, request.BatchID) {
		return fiber.NewError(fiber.StatusConflict, "shielded batch changed; refresh the quote and prove again")
	}
	if err := s.validateShieldedTradeRequest(c.Context(), request, vault, batch); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if err := s.verifyPrivacyAccessFee(c, user, request.PrivacyReceiptTxHash, privacyFeeHidePosition); err != nil {
		return err
	}
	poll, exists, err := s.store.GetPollByID(c.Context(), request.PollID)
	if err != nil || !exists {
		return fiber.NewError(fiber.StatusBadRequest, "poll not found")
	}
	if endsAt, parseErr := time.Parse(time.RFC3339, strings.TrimSpace(poll.EndsAt)); parseErr == nil {
		executeAfter, _ := time.Parse(time.RFC3339, batch.ExecuteAfter)
		if !endsAt.After(executeAfter) {
			return fiber.NewError(fiber.StatusConflict, "market closes before this private batch settles")
		}
	}
	s.collateralMu.Lock()
	quote, quoteErr := s.store.QuoteTrade(c.Context(), store.TradeInput{PollID: request.PollID, Side: request.Side, Amount: request.Amount})
	if quoteErr == nil {
		_, _, quoteErr = s.collateralizeQuote(c.Context(), quote, request.Amount)
	}
	s.collateralMu.Unlock()
	if quoteErr != nil {
		return fiber.NewError(fiber.StatusBadRequest, quoteErr.Error())
	}
	if s.gmrEngine == nil || !s.gmrEngine.Configured() {
		return fiber.NewError(fiber.StatusServiceUnavailable, "GMR Engine relayer is not configured")
	}

	vk, err := os.ReadFile(s.cfg.ShieldedTradeVerificationKeyPath)
	if err != nil {
		if path, pathErr := shieldedTradeArtifactPath("verification_key.json"); pathErr == nil {
			vk, err = os.ReadFile(path)
		}
	}
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "pinned shielded trade verification key is unavailable")
	}
	vkDigest := sha256.Sum256(vk)
	if hex.EncodeToString(vkDigest[:]) != shieldedTradeArtifactChecksums["verification_key.json"] {
		return fiber.NewError(fiber.StatusInternalServerError, "pinned shielded trade verification key checksum mismatch")
	}
	contextPayload, _ := json.Marshal(fiber.Map{"kind": "budol-shielded-trade", "version": "v1", "chainId": s.cfg.WelcomeTokenChainID, "vault": vault.VaultAddress, "batchId": batch.BatchID, "orderCommitment": request.OrderCommitment, "nullifierHash": request.NullifierHash})
	publicSignals, _ := json.Marshal(request.PublicSignals)
	submission, err := s.gmrEngine.SubmitZKProof(c.Context(), gmrengine.ZKProofSubmitRequest{Context: contextPayload, DomainID: s.cfg.ShieldedTradeZKVerifyDomainID, Proof: request.Proof, ProofSystem: "groth16", PublicSignals: publicSignals, VK: vk})
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, "zkVerify submission failed: "+err.Error())
	}

	root, _ := privateClaimFieldToBytes32(request.PublicSignals[0])
	nullifier, _ := privateClaimFieldToBytes32(request.NullifierHash)
	commitment, _ := privateClaimFieldToBytes32(request.OrderCommitment)
	args := []string{root, nullifier, commitment, batch.BatchID, jsonString(request.SolidityProof.PA), jsonString(request.SolidityProof.PB), jsonString(request.SolidityProof.PC), jsonString(request.PublicSignals)}
	write, err := s.gmrEngine.ContractWrite(c.Context(), gmrengine.ContractWriteRequest{ABI: shieldedTradeVaultABI, Args: args, ChainID: s.cfg.WelcomeTokenChainID, ContractAddress: vault.VaultAddress, FunctionName: "submitPrivateOrder"})
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, "relayer submission failed: "+err.Error())
	}
	tx, err := s.waitForGMRTransaction(c.Context(), write.Transaction.ID, 2*time.Minute)
	if err != nil || !strings.EqualFold(tx.Status, "confirmed") {
		return fiber.NewError(fiber.StatusBadGateway, "private order was not confirmed on Horizen")
	}
	order, err := s.store.CreateShieldedTradeOrder(c.Context(), store.ShieldedTradeOrderInput{BatchID: batch.BatchID, UserID: user.ID, PollID: request.PollID, Side: request.Side, Amount: request.Amount, NullifierHash: request.NullifierHash, OrderCommitment: request.OrderCommitment, ZKProofSubmissionID: submission.ID, TransactionID: tx.ID, TransactionHash: tx.TransactionHash, PrivateClaimLeaf: request.PrivateClaimLeaf})
	if err != nil {
		return fiber.NewError(fiber.StatusConflict, err.Error())
	}
	_, _ = s.store.CreateNotification(c.Context(), user.ID, "shielded_trade", "Private order accepted", "Your private order will be included in the next aggregate market update.", "/portfolio")
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"order": order, "batch": batch, "zkVerifySubmission": submission})
}

func (s Server) validateShieldedTradeRequest(ctx context.Context, request shieldedTradeRequest, vault config.ShieldedTradeVault, batch store.ShieldedTradeBatch) error {
	for _, signal := range request.PublicSignals {
		if number, ok := fieldElement(signal); !ok || number.Sign() < 0 {
			return errors.New("public signals contain an invalid field element")
		}
	}
	checks := []struct {
		index    int
		expected string
		name     string
	}{
		{1, request.NullifierHash, "nullifier"}, {2, request.OrderCommitment, "order commitment"},
		{3, strconv.Itoa(s.cfg.WelcomeTokenChainID), "chain"}, {4, addressField(s.activeTokenContract(ctx)), "token"},
		{5, addressField(vault.VaultAddress), "vault"}, {6, shieldedTradeDenominationRaw(vault.TradeAmount, vault.FeeBps, s.cfg.WelcomeTokenDecimals), "denomination"},
		{7, strconv.FormatInt(vault.FeeBps, 10), "fee"}, {8, bytes32Field(batch.BatchID), "batch"},
	}
	for _, check := range checks {
		if !sameFieldElement(request.PublicSignals[check.index], check.expected) {
			return fmt.Errorf("proof %s does not match this order", check.name)
		}
	}
	market := new(big.Int).SetBytes(crypto.Keccak256([]byte(strings.TrimSpace(request.PollID))))
	modulus, _ := new(big.Int).SetString(privateClaimFieldModulusText, 10)
	market.Mod(market, modulus)
	outcome := big.NewInt(1)
	if request.Side == "no" {
		outcome = big.NewInt(2)
	}
	amountRaw := tokenDecimalRaw(strconv.FormatFloat(request.Amount, 'f', -1, 64), s.cfg.WelcomeTokenDecimals)
	salt, ok := fieldElement(request.OrderSalt)
	if !ok {
		return errors.New("orderSalt is invalid")
	}
	batchField, _ := fieldElement(bytes32Field(batch.BatchID))
	expected, err := poseidon.Hash([]*big.Int{market, outcome, amountRaw, salt, batchField})
	if err != nil || !sameFieldElement(expected.String(), request.OrderCommitment) {
		return errors.New("order commitment does not match market, side, amount, salt, and batch")
	}
	return nil
}

func (s Server) shieldedTradeVaultForAmount(amount float64, feeBps int64) (config.ShieldedTradeVault, bool) {
	for _, vault := range s.cfg.ShieldedTradeVaults {
		parsed, _ := strconv.ParseFloat(vault.TradeAmount, 64)
		if math.Abs(parsed-amount) < 0.0000001 && vault.FeeBps == feeBps {
			return vault, true
		}
	}
	return config.ShieldedTradeVault{}, false
}

func (s Server) startShieldedTradeWorker(ctx context.Context) {
	if !s.cfg.ShieldedTradingEnabled {
		return
	}
	go func() {
		s.processShieldedTradeBatches(ctx)
		ticker := time.NewTicker(s.cfg.ShieldedTradePollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.processShieldedTradeBatches(ctx)
			}
		}
	}()
}

func (s Server) processShieldedTradeBatches(ctx context.Context) {
	if !s.shieldedTradeMu.TryLock() {
		return
	}
	defer s.shieldedTradeMu.Unlock()
	for _, vault := range s.cfg.ShieldedTradeVaults {
		if vault.FeeBps != s.engineTradingFeeBps(ctx) {
			continue
		}
		if _, ok, _ := s.store.CurrentShieldedTradeBatch(ctx, vault.VaultAddress, time.Now().UTC()); !ok {
			_, _ = s.openShieldedTradeBatch(ctx, vault)
		}
	}
	due, err := s.store.ListDueShieldedTradeBatches(ctx, time.Now().UTC())
	if err != nil {
		return
	}
	for _, batch := range due {
		s.settleShieldedTradeBatch(ctx, batch)
	}
}

func (s Server) openShieldedTradeBatch(ctx context.Context, vault config.ShieldedTradeVault) (store.ShieldedTradeBatch, error) {
	batchID, err := randomFieldBytes32()
	if err != nil {
		return store.ShieldedTradeBatch{}, err
	}
	now := time.Now().UTC()
	executeAfter := now.Add(s.cfg.ShieldedTradeBatchWindow)
	expiresAt := now.Add(s.cfg.ShieldedTradeBatchExpiry)
	write, err := s.gmrEngine.ContractWrite(ctx, gmrengine.ContractWriteRequest{ABI: shieldedTradeVaultABI, Args: []string{batchID, strconv.FormatInt(executeAfter.Unix(), 10), strconv.FormatInt(expiresAt.Unix(), 10)}, ChainID: s.cfg.WelcomeTokenChainID, ContractAddress: vault.VaultAddress, FunctionName: "openBatch"})
	if err != nil {
		return store.ShieldedTradeBatch{}, err
	}
	tx, err := s.waitForGMRTransaction(ctx, write.Transaction.ID, 2*time.Minute)
	if err != nil || !strings.EqualFold(tx.Status, "confirmed") {
		return store.ShieldedTradeBatch{}, errors.New("opening shielded trade batch was not confirmed")
	}
	amount, _ := strconv.ParseFloat(vault.TradeAmount, 64)
	return s.store.CreateShieldedTradeBatch(ctx, store.ShieldedTradeBatchInput{BatchID: batchID, VaultAddress: vault.VaultAddress, TradeAmount: amount, FeeBps: vault.FeeBps, ExecuteAfter: executeAfter, ExpiresAt: expiresAt, TransactionID: tx.ID, TransactionHash: tx.TransactionHash})
}

func (s Server) settleShieldedTradeBatch(ctx context.Context, batch store.ShieldedTradeBatch) {
	orders, err := s.store.ListShieldedTradeOrders(ctx, batch.BatchID)
	if err != nil {
		return
	}
	if len(orders) == 0 {
		_, _ = s.store.UpdateShieldedTradeBatch(ctx, batch.BatchID, "empty", "", "", "")
		return
	}
	write, err := s.gmrEngine.ContractWrite(ctx, gmrengine.ContractWriteRequest{ABI: shieldedTradeVaultABI, Args: []string{batch.BatchID}, ChainID: s.cfg.WelcomeTokenChainID, ContractAddress: batch.VaultAddress, FunctionName: "settleBatch"})
	if err != nil {
		_, _ = s.store.UpdateShieldedTradeBatch(ctx, batch.BatchID, "failed", "", "", err.Error())
		return
	}
	tx, err := s.waitForGMRTransaction(ctx, write.Transaction.ID, 2*time.Minute)
	if err != nil || !strings.EqualFold(tx.Status, "confirmed") {
		_, _ = s.store.UpdateShieldedTradeBatch(ctx, batch.BatchID, "failed", write.Transaction.ID, tx.TransactionHash, "batch settlement was not confirmed")
		return
	}
	_, _ = s.store.UpdateShieldedTradeBatch(ctx, batch.BatchID, "settled", tx.ID, tx.TransactionHash, "")
	s.shieldedPublishMu.Lock()
	defer s.shieldedPublishMu.Unlock()
	projectWallet, projectWalletErr := s.projectWalletAddress(ctx)
	for _, order := range orders {
		if projectWalletErr != nil {
			_ = s.store.CompleteShieldedTradeOrder(ctx, order.ID, "", "failed", projectWalletErr.Error())
			continue
		}
		user, ok, err := s.store.GetByID(ctx, order.UserID)
		if err != nil || !ok {
			_ = s.store.CompleteShieldedTradeOrder(ctx, order.ID, "", "failed", "user not found")
			continue
		}
		trade, err := s.store.CreateTrade(ctx, user, store.TradeInput{PollID: order.PollID, Side: order.Side, Amount: order.Amount, EscrowTxHash: tx.TransactionHash, EscrowStatus: "verified", EscrowVerifiedAt: time.Now().UTC().Format(time.RFC3339), EscrowFrom: batch.VaultAddress, EscrowTo: projectWallet, EscrowAmount: tradeEscrowTotal(order.Amount, batch.FeeBps), PrivateClaimLeaf: order.PrivateClaimLeaf, PrivacyMode: "shielded", ShieldedBatchID: batch.BatchID})
		if err != nil {
			_ = s.store.CompleteShieldedTradeOrder(ctx, order.ID, "", "failed", err.Error())
			continue
		}
		_ = s.store.CompleteShieldedTradeOrder(ctx, order.ID, trade.ID, "settled", "")
		_, _ = s.store.CreateNotification(ctx, user.ID, "shielded_trade", "Private trade settled", trade.PollTitle+" was included in an aggregate private batch.", "/portfolio")
	}
}

func randomFieldBytes32() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	n := new(big.Int).SetBytes(bytes)
	m, _ := new(big.Int).SetString(privateClaimFieldModulusText, 10)
	n.Mod(n, m)
	padded := make([]byte, 32)
	copy(padded[32-len(n.Bytes()):], n.Bytes())
	return "0x" + hex.EncodeToString(padded), nil
}
func bytes32Field(value string) string {
	n, ok := fieldElement(value)
	if !ok {
		return ""
	}
	return n.String()
}
func addressField(value string) string {
	return new(big.Int).SetBytes(common.HexToAddress(value).Bytes()).String()
}
func jsonString(value any) string { encoded, _ := json.Marshal(value); return string(encoded) }
func tokenDecimalRaw(value string, decimals int) *big.Int {
	r, ok := new(big.Rat).SetString(value)
	if !ok {
		return new(big.Int)
	}
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	r.Mul(r, new(big.Rat).SetInt(scale))
	return new(big.Int).Quo(r.Num(), r.Denom())
}
func shieldedTradeDenominationRaw(amount string, feeBps int64, decimals int) string {
	raw := tokenDecimalRaw(amount, decimals)
	raw.Mul(raw, big.NewInt(10000+feeBps))
	raw.Quo(raw, big.NewInt(10000))
	return raw.String()
}
