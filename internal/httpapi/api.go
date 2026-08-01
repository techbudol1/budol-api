package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/techbudol1/budol-api/internal/config"
	"github.com/techbudol1/budol-api/internal/evm"
	"github.com/techbudol1/budol-api/internal/gmrengine"
	"github.com/techbudol1/budol-api/internal/newsagent"
	"github.com/techbudol1/budol-api/internal/session"
	"github.com/techbudol1/budol-api/internal/store"
	"github.com/techbudol1/budol-api/internal/walletops"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/helmet"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

type Server struct {
	cfg                  config.Config
	collateralMu         *sync.Mutex
	engineLimiter        *engineRateLimiter
	evm                  *evm.Client
	gmrEngine            *gmrengine.Client
	newsAgent            *newsagent.Agent
	store                store.AdminStore
	walletops            *walletops.Client
	sessions             session.Manager
	walletBalanceCache   map[string]walletBalanceCacheEntry
	walletBalanceCacheMu *sync.Mutex
	shieldedTradeMu      *sync.Mutex
	shieldedPublishMu    *sync.Mutex
}

const walletBalanceCacheTTL = 10 * time.Second

type walletBalanceCacheEntry struct {
	Balance  fiber.Map
	Balances []fiber.Map
	Warning  string
	Expires  time.Time
}

type AdminLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type WalletTransferRequest struct {
	To          string `json:"to"`
	Amount      string `json:"amount"`
	ConfirmText string `json:"confirmText"`
}

type WalletAirdropRecipient struct {
	Address string `json:"address"`
	Amount  string `json:"amount"`
}

type WalletAirdropRequest struct {
	Recipients  []WalletAirdropRecipient `json:"recipients"`
	ConfirmText string                   `json:"confirmText"`
}

type WalletBurnRequest struct {
	Amount      string `json:"amount"`
	ConfirmText string `json:"confirmText"`
}

type TokenContractUpdateRequest struct {
	TokenAddress string `json:"tokenAddress"`
}

type GasFreeTradingUpdateRequest struct {
	Enabled bool `json:"enabled"`
}

type TradingFeeUpdateRequest struct {
	TradingFeeBps int64 `json:"tradingFeeBps"`
}

type SponsorPolicyUpdateRequest struct {
	MaxSponsoredTradesPerWalletDay int64  `json:"maxSponsoredTradesPerWalletDay"`
	MaxSponsoredTradesGlobalDay    int64  `json:"maxSponsoredTradesGlobalDay"`
	MinSponsorBalanceRaw           string `json:"minSponsorBalanceRaw"`
}

type PrivacyAccessSettingsUpdateRequest struct {
	CollectorAddress  string `json:"collectorAddress"`
	HidePositionFee   string `json:"hidePositionFee"`
	PrivateClaimFee   string `json:"privateClaimFee"`
	ShieldedPayoutFee string `json:"shieldedPayoutFee"`
}

type TradeRequest struct {
	PollID            string  `json:"pollId"`
	Side              string  `json:"side"`
	Amount            float64 `json:"amount"`
	EscrowTxHash      string  `json:"escrowTxHash"`
	EscrowFromAddress string  `json:"escrowFromAddress"`
	PrivateClaimLeaf  string  `json:"privateClaimLeaf"`
}

type GaslessEscrowRequest struct {
	Amount   string `json:"amount"`
	Deadline string `json:"deadline"`
	Owner    string `json:"owner"`
	PollID   string `json:"pollId"`
	R        string `json:"r"`
	S        string `json:"s"`
	Side     string `json:"side"`
	V        int    `json:"v"`
}

type ManagedEscrowRequest struct {
	Amount string `json:"amount"`
	PollID string `json:"pollId"`
	Side   string `json:"side"`
}

type TradeConfig struct {
	ChainID                    int              `json:"chainId"`
	Collateral                 CollateralStatus `json:"collateral"`
	CollateralGuaranteeEnabled bool             `json:"collateralGuaranteeEnabled"`
	EngineGasFreeEnabled       bool             `json:"engineGasFreeEnabled"`
	GasPayerBalance            string           `json:"gasPayerBalance"`
	GasPayerBalanceRaw         string           `json:"gasPayerBalanceRaw"`
	GaslessSpenderAddress      string           `json:"gaslessSpenderAddress"`
	NetworkName                string           `json:"networkName"`
	TokenAddress               string           `json:"tokenAddress"`
	TokenDecimals              int              `json:"tokenDecimals"`
	TokenSymbol                string           `json:"tokenSymbol"`
	EscrowWalletAddress        string           `json:"escrowWalletAddress"`
	TradingFeeBps              int64            `json:"tradingFeeBps"`
	TradingFeeRate             float64          `json:"tradingFeeRate"`
}

type SmartWalletConfig struct {
	Enabled           bool   `json:"enabled"`
	ChainID           int    `json:"chainId"`
	NetworkName       string `json:"networkName"`
	RPCURL            string `json:"rpcUrl"`
	EntryPointAddress string `json:"entryPointAddress"`
	EntryPointVersion string `json:"entryPointVersion"`
	FactoryAddress    string `json:"factoryAddress"`
	BundlerURL        string `json:"bundlerUrl"`
	PaymasterAddress  string `json:"paymasterAddress"`
	PaymasterURL      string `json:"paymasterUrl"`
	GasSponsored      bool   `json:"gasSponsored"`
	AccountType       string `json:"accountType"`
	Mode              string `json:"mode"`
}

const defaultTradingFeeBps int64 = 50

const sponsorPolicySetting = "gas_free_sponsor_policy"

var allowedTradeAmounts = map[float64]struct{}{
	10:  {},
	25:  {},
	50:  {},
	100: {},
	250: {},
	500: {},
}

type SponsorPolicy struct {
	MaxSponsoredTradesPerWalletDay int64  `json:"maxSponsoredTradesPerWalletDay"`
	MaxSponsoredTradesGlobalDay    int64  `json:"maxSponsoredTradesGlobalDay"`
	MinSponsorBalanceRaw           string `json:"minSponsorBalanceRaw"`
}

type CashoutRequest struct {
	PollID string  `json:"pollId"`
	Side   string  `json:"side"`
	Amount float64 `json:"amount"`
}

type MarketCommentRequest struct {
	Body string `json:"body"`
}

type CommentReportRequest struct {
	Reason string `json:"reason"`
}

type WatchlistRequest struct {
	Slug string `json:"slug"`
}

type CommentModerationRequest struct {
	Status string `json:"status"`
}

type NewsCandidateReviewRequest struct {
	Notes string `json:"notes"`
}

type PollResolutionRequest struct {
	Status                string `json:"status"`
	Outcome               string `json:"outcome"`
	ResolutionSource      string `json:"resolutionSource"`
	ResolutionEvidenceURL string `json:"resolutionEvidenceUrl"`
	ResolutionNotes       string `json:"resolutionNotes"`
	ConfirmText           string `json:"confirmText"`
}

type PayoutRetryRequest struct {
	ConfirmText string `json:"confirmText"`
}

type tradeEscrowVerification struct {
	Status     string
	From       string
	To         string
	Amount     float64
	VerifiedAt string
}

type walletRecipientInput struct {
	Address string
	Amount  string
}

const deadBurnAddress = "0x000000000000000000000000000000000000dEaD"
const settlementConfirmText = "CONFIRM SETTLEMENT"
const retryPayoutConfirmText = "RETRY PAYOUT"
const retryPrivateClaimPayoutConfirmText = "RETRY PRIVATE CLAIM PAYOUT"
const reconcilePrivateClaimPayoutConfirmText = "RECONCILE PRIVATE CLAIM PAYOUT"
const transferConfirmText = "CONFIRM TRANSFER"
const airdropConfirmText = "CONFIRM AIRDROP"
const burnConfirmText = "CONFIRM BURN"
const budolTokenContractSetting = "budol_token_contract"

func New(cfg config.Config, userStore store.AdminStore, walletopsClient *walletops.Client, gmrEngineClient *gmrengine.Client, newsAgent *newsagent.Agent, sessions session.Manager) *fiber.App {
	server := Server{
		cfg:                  cfg,
		collateralMu:         &sync.Mutex{},
		engineLimiter:        newEngineRateLimiter(),
		evm:                  evm.NewClient(cfg.WelcomeTokenRPCURL),
		gmrEngine:            gmrEngineClient,
		newsAgent:            newsAgent,
		store:                userStore,
		walletops:            walletopsClient,
		sessions:             sessions,
		walletBalanceCache:   map[string]walletBalanceCacheEntry{},
		walletBalanceCacheMu: &sync.Mutex{},
		shieldedTradeMu:      &sync.Mutex{},
		shieldedPublishMu:    &sync.Mutex{},
	}

	app := fiber.New(fiber.Config{
		AppName: "BudolPH API",
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			applyAllowedOriginHeaders(c, cfg.AllowedOrigins)
			return errorHandler(c, err)
		},
	})

	app.Use(recover.New())
	app.Use(logger.New())
	app.Use(helmet.New())
	app.Use(cacheControlHeaders)
	app.Use(cors.New(cors.Config{
		AllowCredentials: true,
		AllowHeaders:     "Origin, Content-Type, Accept, Authorization, X-Budol-Admin-Key",
		AllowMethods:     "GET,POST,PUT,PATCH,DELETE,OPTIONS",
		AllowOrigins:     strings.Join(cfg.AllowedOrigins, ","),
	}))

	app.Get("/healthz", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"ok": true})
	})

	workerCtx, stopWorkers := context.WithCancel(context.Background())
	app.Hooks().OnShutdown(func() error {
		stopWorkers()
		return nil
	})
	server.startShieldedWithdrawalWorker(workerCtx)
	server.startShieldedTradeWorker(workerCtx)
	server.startMarketAlertWorker(workerCtx)

	api := app.Group("/api")
	authRateLimit := limiter.New(limiter.Config{
		Max:        20,
		Expiration: 5 * time.Minute,
	})
	tradeRateLimit := limiter.New(limiter.Config{
		Max:        30,
		Expiration: time.Minute,
	})
	adminMutationRateLimit := limiter.New(limiter.Config{
		Max:        60,
		Expiration: time.Minute,
	})
	api.Get("/auth/google/start", authRateLimit, server.startGoogleOAuth)
	api.Get("/auth/google/callback", authRateLimit, server.googleOAuthCallback)
	api.Post("/auth/wallet/nonce", authRateLimit, server.walletLoginNonce)
	api.Post("/auth/wallet/verify", authRateLimit, server.walletLoginVerify)
	api.Get("/auth/me", server.me)
	api.Patch("/account/profile", authRateLimit, server.updateAccountProfile)
	api.Post("/auth/logout", server.logout)
	api.Get("/polls", server.publicPolls)
	api.Get("/polls/:slug/activity", server.publicPollActivity)
	api.Get("/polls/:slug/stats", server.publicPollStats)
	api.Get("/polls/:slug/comments", server.publicPollComments)
	api.Get("/polls/:slug/private-claim-tree", server.pollPrivateClaimTree)
	api.Get("/zk/private-claim/:file", server.privateClaimArtifact)
	api.Get("/zk/shielded-withdrawal/:file", server.shieldedWithdrawalArtifact)
	api.Get("/zk/shielded-trade/:file", server.shieldedTradeArtifact)
	api.Post("/polls/:slug/comments", server.createPollComment)
	api.Post("/comments/:id/report", server.reportPollComment)
	api.Get("/polls/:slug", server.publicPollDetail)
	api.Get("/portfolio", server.portfolio)
	api.Get("/trade-config", server.tradeConfig)
	api.Get("/smart-wallet/config", server.smartWalletConfig)
	api.Get("/collateral-status", server.publicCollateralStatus)
	api.Get("/trade-quote", server.tradeQuote)
	api.Post("/trades/gasless-escrow", tradeRateLimit, server.createGaslessTradeEscrow)
	api.Post("/trades/managed-escrow", tradeRateLimit, server.createManagedTradeEscrow)
	api.Post("/trades", tradeRateLimit, server.createTrade)
	api.Get("/shielded-trades/config", server.shieldedTradeConfig)
	api.Post("/shielded-trades", tradeRateLimit, server.createShieldedTrade)
	api.Post("/trades/:id/public-claim", tradeRateLimit, server.claimPublicPayout)
	api.Get("/cashout-quote", server.cashoutQuote)
	api.Post("/cashouts", server.cashoutPosition)
	api.Get("/private-claims/shielded-config", server.privateClaimShieldedPayoutConfig)
	api.Get("/privacy-access/config", server.privacyAccessConfig)
	api.Post("/privacy-access/managed-fee", tradeRateLimit, server.createManagedPrivacyAccessFee)
	api.Post("/private-claims/shielded-withdrawal-proofs", tradeRateLimit, server.submitShieldedWithdrawalProof)
	api.Get("/private-claims/shielded-withdrawals", server.listShieldedWithdrawals)
	api.Post("/private-claims/shielded-withdrawals", tradeRateLimit, server.withdrawShieldedPayout)
	api.Post("/private-claims/shielded-withdrawals/:id/retry", tradeRateLimit, server.retryShieldedWithdrawal)
	api.Post("/private-claims/proof-submissions", tradeRateLimit, server.submitPrivateClaimProof)
	api.Post("/private-claims", tradeRateLimit, server.claimPrivatePayout)
	api.Get("/notifications", server.notifications)
	api.Post("/notifications/read-all", server.markAllNotificationsRead)
	api.Post("/notifications/:id/read", server.markNotificationRead)
	api.Get("/watchlist", server.watchlist)
	api.Post("/watchlist", server.addWatchlist)
	api.Delete("/watchlist/:slug", server.removeWatchlist)
	api.Get("/market-alerts/:slug", server.marketAlert)
	api.Put("/market-alerts/:slug", server.updateMarketAlert)
	api.Delete("/market-alerts/:slug", server.deleteMarketAlert)
	api.Get("/wallet/balance", server.walletBalance)
	api.Get("/wallet/history", server.walletHistory)

	api.Post("/admin/auth/login", authRateLimit, server.adminLogin)
	api.Get("/admin/auth/me", server.adminMe)
	api.Post("/admin/auth/logout", server.adminLogout)

	admin := api.Group("/admin", server.requireAdminSession)
	admin.Get("/users", server.adminUsers)
	admin.Get("/users/:id", server.adminUserDetail)
	admin.Patch("/users/:id", server.adminUpdateUser)
	admin.Get("/poll-classifications", server.adminPollClassifications)
	admin.Post("/poll-classifications", server.adminUpsertPollClassification)
	admin.Get("/polls", server.adminPolls)
	admin.Post("/polls", server.adminCreatePoll)
	admin.Put("/polls/:id", server.adminUpdatePoll)
	admin.Get("/news-agent/status", server.adminNewsAgentStatus)
	admin.Post("/news-agent/scan", adminMutationRateLimit, server.adminRunNewsAgent)
	admin.Get("/news-candidates", server.adminNewsCandidates)
	admin.Put("/news-candidates/:id", adminMutationRateLimit, server.adminUpdateNewsCandidate)
	admin.Post("/news-candidates/:id/approve", adminMutationRateLimit, server.adminApproveNewsCandidate)
	admin.Post("/news-candidates/:id/reject", adminMutationRateLimit, server.adminRejectNewsCandidate)
	admin.Get("/polls/:id/settlement-preview", server.adminSettlementPreview)
	admin.Get("/polls/:id/settlement-detail", server.adminSettlementDetail)
	admin.Post("/polls/:id/resolution", adminMutationRateLimit, server.adminResolvePoll)
	admin.Post("/polls/:id/payout-retry", adminMutationRateLimit, server.adminRetrySettlementPayout)
	admin.Post("/polls/:id/payout-reconcile", adminMutationRateLimit, server.adminReconcileSettlementPayout)
	admin.Get("/trades", server.adminTrades)
	admin.Post("/trades/:id/escrow-reconcile", adminMutationRateLimit, server.adminReconcileTradeEscrow)
	admin.Post("/trades/:id/private-claim-payout-retry", adminMutationRateLimit, server.adminRetryPrivateClaimPayout)
	admin.Post("/trades/:id/private-claim-payout-reconcile", adminMutationRateLimit, server.adminReconcilePrivateClaimPayout)
	admin.Get("/activity", server.adminActivity)
	admin.Get("/comments", server.adminComments)
	admin.Patch("/comments/:id", server.adminModerateComment)
	admin.Get("/shielded-withdrawals", server.adminShieldedWithdrawals)
	admin.Post("/shielded-withdrawals/:id/retry", adminMutationRateLimit, server.adminRetryShieldedWithdrawal)
	admin.Post("/shielded-withdrawals/:id/process", adminMutationRateLimit, server.adminProcessShieldedWithdrawal)
	admin.Post("/shielded-withdrawals/:id/suspicious", adminMutationRateLimit, server.adminMarkShieldedWithdrawalSuspicious)
	admin.Get("/shielded-withdrawals/queue", server.adminShieldedWithdrawalQueue)
	admin.Post("/shielded-withdrawals/queue/pause", adminMutationRateLimit, server.adminPauseShieldedWithdrawalQueue)
	admin.Post("/shielded-withdrawals/queue/resume", adminMutationRateLimit, server.adminResumeShieldedWithdrawalQueue)
	admin.Get("/wallet", server.adminWalletConfig)
	admin.Patch("/wallet/token-contract", adminMutationRateLimit, server.adminUpdateWalletTokenContract)
	admin.Patch("/wallet/gas-free-trading", adminMutationRateLimit, server.adminUpdateGasFreeTrading)
	admin.Patch("/wallet/trading-fee", adminMutationRateLimit, server.adminUpdateTradingFee)
	admin.Patch("/wallet/sponsor-policy", adminMutationRateLimit, server.adminUpdateSponsorPolicy)
	admin.Patch("/wallet/privacy-access", adminMutationRateLimit, server.adminUpdatePrivacyAccessSettings)
	admin.Post("/wallet/transfer", adminMutationRateLimit, server.adminWalletTransfer)
	admin.Post("/wallet/airdrop", adminMutationRateLimit, server.adminWalletAirdrop)
	admin.Post("/wallet/burn", adminMutationRateLimit, server.adminWalletBurn)
	admin.Get("/welcome-grants", server.adminWelcomeGrants)
	admin.Post("/welcome-grants/retry", adminMutationRateLimit, server.adminRetryWelcomeGrants)
	admin.Get("/engine/apps", server.adminEngineApps)
	admin.Post("/engine/apps", adminMutationRateLimit, server.adminCreateEngineApp)
	admin.Get("/engine/apps/:id", server.adminEngineApp)
	admin.Post("/engine/apps/:id/api-keys", adminMutationRateLimit, server.adminCreateEngineAPIKey)
	admin.Get("/engine/apps/:id/api-keys", server.adminEngineAPIKeys)
	admin.Get("/engine/apps/:id/usage", server.adminEngineUsage)
	admin.Post("/engine/api-keys/:id/revoke", adminMutationRateLimit, server.adminRevokeEngineAPIKey)
	admin.Post("/engine/api-keys/:id/rotate", adminMutationRateLimit, server.adminRotateEngineAPIKey)

	engine := api.Group("/engine", server.requireEngineScope("transactions:read"))
	engine.Get("/auth/me", server.engineAuthMe)

	return app
}

func (s Server) me(c *fiber.Ctx) error {
	user, err := s.authenticatedUser(c)
	if err != nil {
		return err
	}
	user, repaired, err := s.repairManagedOAuthWallet(c, user)
	if err != nil {
		return err
	}
	if repaired {
		if err := s.issueUserSession(c, user); err != nil {
			return err
		}
		s.grantWelcomeTokens(c, user)
		_, _ = s.store.CreateNotification(c.Context(), user.ID, "wallet_repair", "Managed wallet repaired", "BudolPH created a fresh managed wallet because the previous Engine wallet was removed.", "/wallet")
	}

	return c.JSON(fiber.Map{
		"user": user,
	})
}

func (s Server) shouldCreateLoginNotification(c *fiber.Ctx, userID string) bool {
	cookie := c.Cookies(s.cfg.SessionCookieName)
	if cookie == "" {
		return true
	}
	claims, err := s.sessions.Verify(cookie)
	if err != nil {
		return true
	}
	return claims.Subject != userID
}

func (s Server) authenticatedUser(c *fiber.Ctx) (store.User, error) {
	cookie := c.Cookies(s.cfg.SessionCookieName)
	if cookie == "" {
		return store.User{}, fiber.NewError(fiber.StatusUnauthorized, "not logged in")
	}

	claims, err := s.sessions.Verify(cookie)
	if err != nil {
		return store.User{}, fiber.NewError(fiber.StatusUnauthorized, "invalid session")
	}

	user, ok, err := s.store.GetByID(c.Context(), claims.Subject)
	if err != nil {
		return store.User{}, fiber.NewError(fiber.StatusInternalServerError, "failed to load user")
	}
	if !ok {
		return store.User{}, fiber.NewError(fiber.StatusUnauthorized, "user not found")
	}
	return user, nil
}

func (s Server) repairManagedOAuthWallet(c *fiber.Ctx, user store.User) (store.User, bool, error) {
	if s.gmrEngine == nil || !s.gmrEngine.Configured() || !strings.EqualFold(user.WalletCustody, "managed") {
		return user, false, nil
	}
	authProvider, ok := managedOAuthProvider(user.AuthType)
	if !ok {
		return user, false, nil
	}
	providerUserID := strings.TrimSpace(user.ProviderUserID)
	if providerUserID == "" {
		return user, false, nil
	}
	managedWallet, err := s.gmrEngine.CreateManagedUserWallet(c.Context(), gmrengine.UserWalletRequest{
		AuthProvider: authProvider,
		Email:        user.Email,
		Metadata:     `{"source":"auth_me_repair"}`,
		UserID:       providerUserID,
	})
	if err != nil {
		return user, false, nil
	}
	if strings.EqualFold(managedWallet.Address, user.WalletAddress) {
		return user, false, nil
	}
	updatedUser, err := s.store.UpdateUserWalletAddress(c.Context(), user.ID, managedWallet.Address)
	if err != nil {
		return user, false, fiber.NewError(fiber.StatusInternalServerError, "failed to repair managed wallet")
	}
	return updatedUser, true, nil
}

func managedOAuthProvider(authType string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(authType)) {
	case "google_oauth":
		return "google", true
	case "x_oauth":
		return "x", true
	default:
		return "", false
	}
}

func (s Server) logout(c *fiber.Ctx) error {
	c.Cookie(&fiber.Cookie{
		Name:     s.cfg.SessionCookieName,
		Value:    "",
		Expires:  time.Now().UTC().Add(-time.Hour),
		HTTPOnly: true,
		SameSite: fiber.CookieSameSiteLaxMode,
		Secure:   s.cfg.IsProduction(),
		Path:     "/",
	})

	return c.JSON(fiber.Map{"ok": true})
}

func (s Server) grantWelcomeTokens(c *fiber.Ctx, user store.User) {
	tokenContract := s.activeTokenContract(c.Context())
	if tokenContract == "" || s.cfg.WelcomeTokenAmount == "" {
		return
	}

	quantity, err := walletops.TokenQuantity(s.cfg.WelcomeTokenAmount, s.cfg.WelcomeTokenDecimals)
	if err != nil {
		return
	}

	grant, created, err := s.store.CreateTokenGrantIfMissing(c.Context(), user, store.TokenGrantInput{
		GrantType:    "welcome",
		TokenAddress: tokenContract,
		ChainID:      int64(s.cfg.WelcomeTokenChainID),
		Amount:       s.cfg.WelcomeTokenAmount,
		Quantity:     quantity,
	})
	if err != nil {
		return
	}
	if !created && !shouldRetryTokenGrant(c, s.walletops, grant) {
		return
	}

	_ = s.attemptWelcomeTokenGrant(c, user, grant, quantity)
}

func (s Server) attemptWelcomeTokenGrant(c *fiber.Ctx, user store.User, grant store.TokenGrant, quantity string) store.TokenGrant {
	tokenContract := s.activeTokenContract(c.Context())
	grantDetails := &store.TokenGrantInput{
		GrantType:    "welcome",
		TokenAddress: tokenContract,
		ChainID:      int64(s.cfg.WelcomeTokenChainID),
		Amount:       s.cfg.WelcomeTokenAmount,
		Quantity:     quantity,
	}
	if s.gmrEngine == nil || !s.gmrEngine.Configured() {
		updated, _ := s.store.UpdateTokenGrantStatus(c.Context(), grant.ID, "pending", nil, "GMR Engine is not configured", grantDetails)
		_, _ = s.store.CreateNotification(c.Context(), user.ID, "welcome_tokens", "Welcome token grant pending", "BudolPH could not send the welcome tokens because GMR Engine is not configured.", "/wallet")
		return updated
	}

	projectWallet, ok, err := s.gmrEngine.DefaultAdminWallet(c.Context())
	if err != nil || !ok {
		message := "GMR Engine default project wallet is not configured"
		if err != nil {
			message = err.Error()
		}
		updated, _ := s.store.UpdateTokenGrantStatus(c.Context(), grant.ID, "pending", nil, message, grantDetails)
		_, _ = s.store.CreateNotification(c.Context(), user.ID, "welcome_tokens", "Welcome token grant pending", "BudolPH is waiting for the project wallet to be configured before sending your welcome tokens.", "/wallet")
		return updated
	}
	nativeBalance, err := s.evm.NativeBalance(c.Context(), projectWallet.Address)
	if err != nil || isZeroDecimalString(nativeBalance.Raw) {
		message := "project wallet needs " + networkName(s.cfg.WelcomeTokenChainID) + " ETH for gas"
		if err != nil {
			message = err.Error()
		}
		updated, _ := s.store.UpdateTokenGrantStatus(c.Context(), grant.ID, "pending", nil, message, grantDetails)
		_, _ = s.store.CreateNotification(c.Context(), user.ID, "welcome_tokens", "Welcome token grant pending", "BudolPH is waiting for the project wallet to be funded with "+networkName(s.cfg.WelcomeTokenChainID)+" ETH for gas.", "/wallet")
		return updated
	}
	tokenBalance, err := s.evm.ERC20Balance(c.Context(), tokenContract, projectWallet.Address, s.cfg.WelcomeTokenDecimals)
	if err != nil || !hasAtLeastRawTokenBalance(tokenBalance.Raw, quantity) {
		message := "project wallet needs BUDOL tokens"
		if err != nil {
			message = err.Error()
		}
		updated, _ := s.store.UpdateTokenGrantStatus(c.Context(), grant.ID, "pending", nil, message, grantDetails)
		_, _ = s.store.CreateNotification(c.Context(), user.ID, "welcome_tokens", "Welcome token grant pending", "BudolPH is waiting for the project wallet to be funded with BUDOL tokens.", "/wallet")
		return updated
	}

	grantAmount, err := strconv.ParseFloat(strings.TrimSpace(s.cfg.WelcomeTokenAmount), 64)
	if err != nil || grantAmount <= 0 {
		updated, _ := s.store.UpdateTokenGrantStatus(c.Context(), grant.ID, "failed", nil, "welcome token amount is invalid", grantDetails)
		return updated
	}
	s.collateralMu.Lock()
	if err := s.ensureCollateralOutflow(c.Context(), grantAmount, 0); err != nil {
		s.collateralMu.Unlock()
		updated, _ := s.store.UpdateTokenGrantStatus(c.Context(), grant.ID, "pending", nil, "BUDOL is reserved for guaranteed market payouts", grantDetails)
		_, _ = s.store.CreateNotification(c.Context(), user.ID, "welcome_tokens", "Welcome token grant pending", "BudolPH is waiting for free treasury tokens; market payout reserves cannot be spent.", "/wallet")
		return updated
	}
	result, err := s.gmrEngine.TransferERC20(c.Context(), gmrengine.TransferRequest{
		Amount:          s.cfg.WelcomeTokenAmount,
		ChainID:         s.cfg.WelcomeTokenChainID,
		ContractAddress: tokenContract,
		Decimals:        s.cfg.WelcomeTokenDecimals,
		Recipient:       user.WalletAddress,
	})
	s.collateralMu.Unlock()
	if err != nil {
		// A gateway or RPC error can happen after the transaction was broadcast.
		// Keep the grant pending until wallet history confirms whether it landed.
		updated, _ := s.store.UpdateTokenGrantStatus(c.Context(), grant.ID, "pending", nil, err.Error(), grantDetails)
		return updated
	}

	updated, _ := s.store.UpdateTokenGrantStatus(c.Context(), grant.ID, "sent", result.TransactionIDs, "", grantDetails)
	notificationLink := "/wallet"
	if len(result.TransactionIDs) > 0 && strings.HasPrefix(strings.ToLower(result.TransactionIDs[0]), "0x") {
		notificationLink = walletTransferLink(result.TransactionIDs[0])
	}
	_, _ = s.store.CreateNotification(c.Context(), user.ID, "welcome_tokens", "Welcome tokens received", s.cfg.WelcomeTokenAmount+" BUDOL welcome tokens were sent to your wallet.", notificationLink)
	return updated
}

func isZeroDecimalString(value string) bool {
	parsed, ok := new(big.Int).SetString(strings.TrimSpace(value), 10)
	return !ok || parsed.Sign() == 0
}

func isUnsignedIntegerString(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func hasAtLeastRawTokenBalance(balance string, required string) bool {
	balanceValue, balanceOK := new(big.Int).SetString(strings.TrimSpace(balance), 10)
	requiredValue, requiredOK := new(big.Int).SetString(strings.TrimSpace(required), 10)
	if !balanceOK || !requiredOK {
		return false
	}
	return balanceValue.Cmp(requiredValue) >= 0
}

func (s Server) adminLogin(c *fiber.Ctx) error {
	var request AdminLoginRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}

	if !s.validAdminCredentials(request.Username, request.Password) {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid admin credentials")
	}

	token, err := s.sessions.Issue(store.User{
		ID:            "admin:" + s.cfg.AdminUsername,
		WalletAddress: "admin:" + s.cfg.AdminUsername,
		Role:          "admin",
		Status:        "active",
	})
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to issue admin session")
	}

	s.setAdminCookie(c, token, time.Now().UTC().Add(s.sessions.TTL()))
	_, _ = s.store.CreateAdminActivity(c.Context(), store.AdminActivityInput{
		Actor:      s.cfg.AdminUsername,
		Action:     "login",
		TargetType: "session",
		Title:      "Admin signed in",
		Detail:     s.cfg.AdminUsername + " opened the control room.",
	})
	return c.JSON(fiber.Map{
		"admin": fiber.Map{
			"username": s.cfg.AdminUsername,
		},
	})
}

func (s Server) adminMe(c *fiber.Ctx) error {
	claims, err := s.verifyAdminCookie(c)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "not logged in")
	}
	return c.JSON(fiber.Map{
		"admin": fiber.Map{
			"username": strings.TrimPrefix(claims.Subject, "admin:"),
		},
	})
}

func (s Server) adminLogout(c *fiber.Ctx) error {
	actor := s.adminActor(c)
	s.setAdminCookie(c, "", time.Now().UTC().Add(-time.Hour))
	_, _ = s.store.CreateAdminActivity(c.Context(), store.AdminActivityInput{
		Actor:      actor,
		Action:     "logout",
		TargetType: "session",
		Title:      "Admin signed out",
		Detail:     actor + " ended the dashboard session.",
	})
	return c.JSON(fiber.Map{"ok": true})
}

func (s Server) publicPolls(c *fiber.Ctx) error {
	s.shieldedPublishMu.Lock()
	defer s.shieldedPublishMu.Unlock()
	polls, err := s.store.ListPolls(c.Context(), true)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load polls")
	}
	return c.JSON(fiber.Map{"polls": polls})
}

func (s Server) publicPollDetail(c *fiber.Ctx) error {
	s.shieldedPublishMu.Lock()
	defer s.shieldedPublishMu.Unlock()
	poll, ok, err := s.store.GetPollBySlug(c.Context(), c.Params("slug"), true)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load poll")
	}
	if !ok {
		return fiber.NewError(fiber.StatusNotFound, "poll not found")
	}
	return c.JSON(fiber.Map{"poll": poll})
}

func (s Server) publicPollActivity(c *fiber.Ctx) error {
	s.shieldedPublishMu.Lock()
	defer s.shieldedPublishMu.Unlock()
	activities, err := s.store.MarketActivity(c.Context(), c.Params("slug"))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load market activity")
	}
	return c.JSON(fiber.Map{"activity": activities})
}

func (s Server) publicPollStats(c *fiber.Ctx) error {
	s.shieldedPublishMu.Lock()
	defer s.shieldedPublishMu.Unlock()
	stats, err := s.store.MarketStats(c.Context(), c.Params("slug"))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load market stats")
	}
	return c.JSON(fiber.Map{"stats": stats})
}

func (s Server) publicPollComments(c *fiber.Ctx) error {
	comments, err := s.store.ListMarketComments(c.Context(), c.Params("slug"))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load market comments")
	}
	return c.JSON(fiber.Map{"comments": comments})
}

func (s Server) createPollComment(c *fiber.Ctx) error {
	user, err := s.authenticatedUser(c)
	if err != nil {
		return err
	}

	var request MarketCommentRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}

	comment, err := s.store.CreateMarketComment(c.Context(), user, c.Params("slug"), request.Body)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	_, _ = s.store.CreateNotification(c.Context(), user.ID, "comment", "Marites note posted", "Your comment is visible on "+comment.PollSlug+".", "/markets/"+comment.PollSlug)
	_, _ = s.store.NotifyWatchers(c.Context(), comment.PollSlug, user.ID, "watchlist_comment", "New comment on watched market", comment.Actor+" posted a Marites note.", "/markets/"+comment.PollSlug)
	return c.JSON(fiber.Map{"comment": comment})
}

func (s Server) reportPollComment(c *fiber.Ctx) error {
	user, err := s.authenticatedUser(c)
	if err != nil {
		return err
	}

	var request CommentReportRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}

	comment, err := s.store.ReportMarketComment(c.Context(), user, c.Params("id"), request.Reason)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	_, _ = s.store.CreateNotification(c.Context(), user.ID, "comment_report", "Comment reported", "Thanks. BudolPH admins will review that Marites note.", "/markets/"+comment.PollSlug)
	_, _ = s.store.CreateAdminActivity(c.Context(), store.AdminActivityInput{
		Actor:      shortAddressForAdmin(user.WalletAddress),
		Action:     "report_comment",
		TargetType: "comment",
		TargetID:   comment.ID,
		Title:      "Comment reported",
		Detail:     comment.PollSlug + " report: " + comment.LatestReportReason,
	})
	return c.JSON(fiber.Map{"comment": comment})
}

func (s Server) createTrade(c *fiber.Ctx) error {
	user, err := s.authenticatedUser(c)
	if err != nil {
		return err
	}

	var request TradeRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	if _, ok := fieldElement(request.PrivateClaimLeaf); !ok {
		return fiber.NewError(fiber.StatusBadRequest, "privateClaimLeaf is required")
	}
	if err := validateFixedTradeAmount(request.Amount); err != nil {
		return err
	}
	escrow, err := s.verifyTradeEscrow(c, user, request)
	if err != nil {
		return err
	}

	s.collateralMu.Lock()
	defer s.collateralMu.Unlock()
	quote, err := s.store.QuoteTrade(c.Context(), store.TradeInput{
		PollID: request.PollID,
		Side:   request.Side,
		Amount: request.Amount,
	})
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if _, _, err := s.collateralizeQuote(c.Context(), quote, 0); err != nil {
		if errors.Is(err, errUnderCollateralized) {
			return fiber.NewError(fiber.StatusConflict, "trade rejected: payout reserves would not remain fully collateralized")
		}
		return fiber.NewError(fiber.StatusBadGateway, "failed to verify payout collateral")
	}

	trade, err := s.store.CreateTrade(c.Context(), user, store.TradeInput{
		PollID:           request.PollID,
		Side:             request.Side,
		Amount:           request.Amount,
		EscrowTxHash:     request.EscrowTxHash,
		EscrowStatus:     escrow.Status,
		EscrowVerifiedAt: escrow.VerifiedAt,
		EscrowFrom:       escrow.From,
		EscrowTo:         escrow.To,
		EscrowAmount:     escrow.Amount,
		PrivateClaimLeaf: strings.TrimSpace(request.PrivateClaimLeaf),
	})
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	s.recordPrivateClaimRootForPoll(c.Context(), trade.PollID, "trade_created")
	_, _ = s.store.CreateNotification(c.Context(), user.ID, "trade", "Trade placed", trade.PollTitle+": bought "+trade.OutcomeLabel+" for "+settlementAmountString(trade.Amount)+" BUDOL.", "/markets/"+trade.PollSlug)
	oldYesPercent := quote.SpotPriceCents
	if strings.EqualFold(quote.Side, "no") {
		oldYesPercent = 100 - quote.SpotPriceCents
	}
	_, _ = s.store.NotifyMarketPriceAlerts(c.Context(), trade.PollID, user.ID, oldYesPercent, quote.NewYesPercent)

	portfolio, err := s.store.UserPortfolio(c.Context(), user.ID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load portfolio")
	}
	poll, _, err := s.store.GetPollBySlug(c.Context(), trade.PollSlug, true)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load updated poll")
	}

	return c.JSON(fiber.Map{
		"trade":     trade,
		"portfolio": portfolio,
		"poll":      poll,
	})
}

func (s Server) createGaslessTradeEscrow(c *fiber.Ctx) error {
	user, err := s.authenticatedUser(c)
	if err != nil {
		return err
	}
	if s.gmrEngine == nil || !s.gmrEngine.Configured() {
		return fiber.NewError(fiber.StatusServiceUnavailable, "GMR Engine is not configured")
	}
	engineGasFreeEnabled, gaslessSpenderAddress := s.engineGasFreeConfig(c.Context())
	if !engineGasFreeEnabled {
		return fiber.NewError(fiber.StatusForbidden, "GMR Engine gas-free trading is disabled for this project")
	}

	var request GaslessEscrowRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	owner := strings.ToLower(strings.TrimSpace(request.Owner))
	if owner == "" || !walletops.IsEVMAddress(owner) {
		return fiber.NewError(fiber.StatusBadRequest, "valid owner is required")
	}
	if owner != strings.ToLower(strings.TrimSpace(user.WalletAddress)) {
		return fiber.NewError(fiber.StatusForbidden, "permit owner must match the logged-in wallet")
	}
	if strings.TrimSpace(request.Amount) == "" {
		return fiber.NewError(fiber.StatusBadRequest, "amount is required")
	}
	amount, err := strconv.ParseFloat(strings.TrimSpace(request.Amount), 64)
	if err != nil || amount <= 0 {
		return fiber.NewError(fiber.StatusBadRequest, "valid amount is required")
	}
	if err := validateFixedTradeAmount(amount); err != nil {
		return err
	}
	tradingFeeBps := s.engineTradingFeeBps(c.Context())
	escrowTotal := tradeEscrowTotal(amount, tradingFeeBps)
	if err := s.ensureTradingBalance(c, user, escrowTotal); err != nil {
		return err
	}
	if err := s.enforceSponsorPolicy(c, owner, gaslessSpenderAddress); err != nil {
		return err
	}
	if strings.TrimSpace(request.PollID) == "" {
		return fiber.NewError(fiber.StatusBadRequest, "pollId is required")
	}
	side := strings.ToLower(strings.TrimSpace(request.Side))
	if side != "yes" && side != "no" {
		return fiber.NewError(fiber.StatusBadRequest, "side must be yes or no")
	}
	if !isDecimalString(request.Deadline) || !isBytes32String(request.R) || !isBytes32String(request.S) {
		return fiber.NewError(fiber.StatusBadRequest, "valid permit signature is required")
	}
	tokenContract := s.activeTokenContract(c.Context())
	projectWallet, err := s.projectWalletAddress(c.Context())
	if err != nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, err.Error())
	}

	s.collateralMu.Lock()
	defer s.collateralMu.Unlock()
	quote, err := s.store.QuoteTrade(c.Context(), store.TradeInput{
		PollID: request.PollID,
		Side:   side,
		Amount: amount,
	})
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if _, _, err := s.collateralizeQuote(c.Context(), quote, amount); err != nil {
		if errors.Is(err, errUnderCollateralized) {
			return fiber.NewError(fiber.StatusConflict, "trade rejected: payout reserves would not remain fully collateralized")
		}
		return fiber.NewError(fiber.StatusBadGateway, "failed to verify payout collateral")
	}
	result, err := s.gmrEngine.TransferERC20WithPermit(c.Context(), gmrengine.TransferWithPermitRequest{
		Amount:          settlementAmountString(escrowTotal),
		ChainID:         s.cfg.WelcomeTokenChainID,
		ContractAddress: tokenContract,
		Deadline:        strings.TrimSpace(request.Deadline),
		Decimals:        s.cfg.WelcomeTokenDecimals,
		Owner:           owner,
		Recipient:       projectWallet,
		R:               strings.TrimSpace(request.R),
		S:               strings.TrimSpace(request.S),
		V:               request.V,
	})
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	txHash := strings.TrimSpace(result.TransferTransactionHash)
	if txHash == "" && len(result.TransactionIDs) > 0 {
		txHash = strings.TrimSpace(result.TransactionIDs[len(result.TransactionIDs)-1])
	}
	if txHash == "" {
		return fiber.NewError(fiber.StatusBadGateway, "GMR Engine did not return an escrow transaction hash")
	}
	_ = s.store.CreateSponsoredEscrow(c.Context(), owner, request.PollID, side, amount, tradeFeeAmount(amount, tradingFeeBps), escrowTotal, txHash)
	return c.JSON(fiber.Map{
		"escrowTxHash":            txHash,
		"escrowAmount":            escrowTotal,
		"gasFree":                 true,
		"permitTransactionHash":   result.PermitTransactionHash,
		"tradingFee":              tradeFeeAmount(amount, tradingFeeBps),
		"tradingFeeBps":           tradingFeeBps,
		"transactionIds":          result.TransactionIDs,
		"transferTransactionHash": result.TransferTransactionHash,
	})
}

func (s Server) createManagedTradeEscrow(c *fiber.Ctx) error {
	user, err := s.authenticatedUser(c)
	if err != nil {
		return err
	}
	if !strings.EqualFold(strings.TrimSpace(user.WalletCustody), "managed") {
		return fiber.NewError(fiber.StatusForbidden, "managed escrow is only available for social-login wallets")
	}
	if s.gmrEngine == nil || !s.gmrEngine.Configured() {
		return fiber.NewError(fiber.StatusServiceUnavailable, "GMR Engine is not configured")
	}

	var request ManagedEscrowRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	amount, err := strconv.ParseFloat(strings.TrimSpace(request.Amount), 64)
	if err != nil || amount <= 0 {
		return fiber.NewError(fiber.StatusBadRequest, "valid amount is required")
	}
	if err := validateFixedTradeAmount(amount); err != nil {
		return err
	}
	tradingFeeBps := s.engineTradingFeeBps(c.Context())
	escrowTotal := tradeEscrowTotal(amount, tradingFeeBps)
	if err := s.ensureTradingBalance(c, user, escrowTotal); err != nil {
		return err
	}
	if strings.TrimSpace(request.PollID) == "" {
		return fiber.NewError(fiber.StatusBadRequest, "pollId is required")
	}
	side := strings.ToLower(strings.TrimSpace(request.Side))
	if side != "yes" && side != "no" {
		return fiber.NewError(fiber.StatusBadRequest, "side must be yes or no")
	}
	tokenContract := s.activeTokenContract(c.Context())
	projectWallet, err := s.projectWalletAddress(c.Context())
	if err != nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, err.Error())
	}

	s.collateralMu.Lock()
	defer s.collateralMu.Unlock()
	quote, err := s.store.QuoteTrade(c.Context(), store.TradeInput{
		PollID: request.PollID,
		Side:   side,
		Amount: amount,
	})
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if _, _, err := s.collateralizeQuote(c.Context(), quote, amount); err != nil {
		if errors.Is(err, errUnderCollateralized) {
			return fiber.NewError(fiber.StatusConflict, "trade rejected: payout reserves would not remain fully collateralized")
		}
		return fiber.NewError(fiber.StatusBadGateway, "failed to verify payout collateral")
	}

	deadline := strconv.FormatInt(time.Now().UTC().Add(15*time.Minute).Unix(), 10)
	result, err := s.gmrEngine.TransferManagedERC20WithPermit(c.Context(), gmrengine.TransferWithPermitRequest{
		Amount:          settlementAmountString(escrowTotal),
		ChainID:         s.cfg.WelcomeTokenChainID,
		ContractAddress: tokenContract,
		Deadline:        deadline,
		Decimals:        s.cfg.WelcomeTokenDecimals,
		Owner:           user.WalletAddress,
		Recipient:       projectWallet,
	})
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	txHash := strings.TrimSpace(result.TransferTransactionHash)
	if txHash == "" && len(result.TransactionIDs) > 0 {
		txHash = strings.TrimSpace(result.TransactionIDs[len(result.TransactionIDs)-1])
	}
	if txHash == "" {
		return fiber.NewError(fiber.StatusBadGateway, "GMR Engine did not return a managed escrow transaction hash")
	}
	return c.JSON(fiber.Map{
		"escrowTxHash":            txHash,
		"escrowAmount":            escrowTotal,
		"gasFree":                 true,
		"managed":                 true,
		"permitTransactionHash":   result.PermitTransactionHash,
		"tradingFee":              tradeFeeAmount(amount, tradingFeeBps),
		"tradingFeeBps":           tradingFeeBps,
		"transactionIds":          result.TransactionIDs,
		"transferTransactionHash": result.TransferTransactionHash,
	})
}

func (s Server) tradeQuote(c *fiber.Ctx) error {
	amount, err := strconv.ParseFloat(c.Query("amount"), 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid amount")
	}
	if err := validateFixedTradeAmount(amount); err != nil {
		return err
	}
	quote, err := s.store.QuoteTrade(c.Context(), store.TradeInput{
		PollID: c.Query("pollId"),
		Side:   c.Query("side"),
		Amount: amount,
	})
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	quote, _, err = s.collateralizeQuote(c.Context(), quote, amount)
	if err != nil {
		if errors.Is(err, errUnderCollateralized) {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error": "trade rejected: payout reserves would not remain fully collateralized",
				"quote": quote,
			})
		}
		return fiber.NewError(fiber.StatusBadGateway, "failed to verify payout collateral")
	}
	return c.JSON(fiber.Map{"quote": quote})
}

func (s Server) publicCollateralStatus(c *fiber.Ctx) error {
	status, err := s.currentCollateralStatus(c.Context())
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, "failed to load payout collateral")
	}
	return c.JSON(fiber.Map{"collateral": status})
}

func (s Server) smartWalletConfig(c *fiber.Ctx) error {
	configured := s.cfg.SmartWalletEnabled &&
		walletops.IsEVMAddress(s.cfg.SmartWalletEntryPointAddress) &&
		walletops.IsEVMAddress(s.cfg.SmartWalletFactoryAddress) &&
		strings.TrimSpace(s.cfg.SmartWalletBundlerURL) != ""

	return c.JSON(fiber.Map{
		"config": SmartWalletConfig{
			Enabled:           configured,
			ChainID:           s.cfg.SmartWalletChainID,
			NetworkName:       networkName(s.cfg.SmartWalletChainID),
			RPCURL:            s.cfg.SmartWalletRPCURL,
			EntryPointAddress: strings.ToLower(s.cfg.SmartWalletEntryPointAddress),
			EntryPointVersion: s.cfg.SmartWalletEntryPointVersion,
			FactoryAddress:    strings.ToLower(s.cfg.SmartWalletFactoryAddress),
			BundlerURL:        s.cfg.SmartWalletBundlerURL,
			PaymasterAddress:  strings.ToLower(s.cfg.SmartWalletPaymasterAddress),
			PaymasterURL:      s.cfg.SmartWalletPaymasterURL,
			GasSponsored:      s.cfg.SmartWalletGasSponsored && walletops.IsEVMAddress(s.cfg.SmartWalletPaymasterAddress) && strings.TrimSpace(s.cfg.SmartWalletPaymasterURL) != "",
			AccountType:       "SimpleAccount",
			Mode:              "erc4337",
		},
	})
}

func (s Server) tradeConfig(c *fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	tokenContract := s.activeTokenContract(c.Context())
	if strings.TrimSpace(tokenContract) == "" {
		return fiber.NewError(fiber.StatusServiceUnavailable, "BUDOL token contract is not configured")
	}
	if !walletops.IsEVMAddress(tokenContract) {
		return fiber.NewError(fiber.StatusServiceUnavailable, "BUDOL token contract is invalid")
	}
	projectWallet, err := s.projectWalletAddress(c.Context())
	if err != nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, err.Error())
	}
	collateral, err := s.currentCollateralStatus(c.Context())
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, "failed to load payout collateral")
	}

	engineGasFreeEnabled, gaslessSpenderAddress := s.engineGasFreeConfig(c.Context())
	tradingFeeBps := s.engineTradingFeeBps(c.Context())
	gasPayerBalance := ""
	gasPayerBalanceRaw := ""
	if engineGasFreeEnabled && gaslessSpenderAddress != "" {
		if balance, err := s.evm.NativeBalance(c.Context(), gaslessSpenderAddress); err == nil {
			gasPayerBalance = balance.Formatted
			gasPayerBalanceRaw = balance.Raw
		}
	}
	return c.JSON(fiber.Map{
		"config": TradeConfig{
			ChainID:                    s.cfg.WelcomeTokenChainID,
			Collateral:                 collateral,
			CollateralGuaranteeEnabled: s.cfg.CollateralGuaranteeEnabled,
			EngineGasFreeEnabled:       engineGasFreeEnabled,
			GasPayerBalance:            gasPayerBalance,
			GasPayerBalanceRaw:         gasPayerBalanceRaw,
			GaslessSpenderAddress:      strings.ToLower(gaslessSpenderAddress),
			NetworkName:                networkName(s.cfg.WelcomeTokenChainID),
			TokenAddress:               strings.ToLower(tokenContract),
			TokenDecimals:              s.cfg.WelcomeTokenDecimals,
			TokenSymbol:                s.cfg.WelcomeTokenSymbol,
			EscrowWalletAddress:        projectWallet,
			TradingFeeBps:              tradingFeeBps,
			TradingFeeRate:             float64(tradingFeeBps) / 10000,
		},
	})
}

func (s Server) engineGasFreeConfig(ctx context.Context) (bool, string) {
	if s.gmrEngine == nil || !s.gmrEngine.Configured() {
		return false, ""
	}
	auth, err := s.gmrEngine.AuthMe(ctx)
	if err != nil || !auth.App.GasFreeEnabled {
		return false, ""
	}
	wallet, ok, err := s.gmrEngine.DefaultAdminWallet(ctx)
	if err != nil || !ok || !walletops.IsEVMAddress(wallet.Address) {
		return false, ""
	}
	return true, wallet.Address
}

func (s Server) engineTradingFeeBps(ctx context.Context) int64 {
	if s.gmrEngine == nil || !s.gmrEngine.Configured() {
		return defaultTradingFeeBps
	}
	auth, err := s.gmrEngine.AuthMe(ctx)
	if err != nil {
		return defaultTradingFeeBps
	}
	return normalizeTradingFeeBps(auth.App.TradingFeeBps)
}

func (s Server) sponsorPolicy(ctx context.Context) SponsorPolicy {
	defaultPolicy := SponsorPolicy{
		MaxSponsoredTradesPerWalletDay: 20,
		MaxSponsoredTradesGlobalDay:    1000,
		MinSponsorBalanceRaw:           "10000000000000000",
	}
	value, ok, err := s.store.GetSystemSetting(ctx, sponsorPolicySetting)
	if err != nil || !ok || strings.TrimSpace(value) == "" {
		return defaultPolicy
	}
	var policy SponsorPolicy
	if err := json.Unmarshal([]byte(value), &policy); err != nil {
		return defaultPolicy
	}
	return normalizeSponsorPolicy(policy)
}

func normalizeSponsorPolicy(policy SponsorPolicy) SponsorPolicy {
	if policy.MaxSponsoredTradesPerWalletDay <= 0 {
		policy.MaxSponsoredTradesPerWalletDay = 20
	}
	if policy.MaxSponsoredTradesPerWalletDay > 10000 {
		policy.MaxSponsoredTradesPerWalletDay = 10000
	}
	if policy.MaxSponsoredTradesGlobalDay <= 0 {
		policy.MaxSponsoredTradesGlobalDay = 1000
	}
	if policy.MaxSponsoredTradesGlobalDay > 1000000 {
		policy.MaxSponsoredTradesGlobalDay = 1000000
	}
	if strings.TrimSpace(policy.MinSponsorBalanceRaw) == "" {
		policy.MinSponsorBalanceRaw = "10000000000000000"
	}
	if _, ok := new(big.Int).SetString(strings.TrimSpace(policy.MinSponsorBalanceRaw), 10); !ok {
		policy.MinSponsorBalanceRaw = "10000000000000000"
	}
	return policy
}

func (s Server) enforceSponsorPolicy(c *fiber.Ctx, owner string, sponsorAddress string) error {
	policy := s.sponsorPolicy(c.Context())
	since := time.Now().UTC().Truncate(24 * time.Hour).Format(time.RFC3339)
	walletCount, err := s.store.CountSponsoredEscrows(c.Context(), owner, since)
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, "failed to check wallet sponsor limits")
	}
	if walletCount >= policy.MaxSponsoredTradesPerWalletDay {
		return fiber.NewError(fiber.StatusTooManyRequests, "daily gas-free trade limit reached for this wallet")
	}
	globalCount, err := s.store.CountGlobalSponsoredEscrows(c.Context(), since)
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, "failed to check global sponsor limits")
	}
	if globalCount >= policy.MaxSponsoredTradesGlobalDay {
		return fiber.NewError(fiber.StatusTooManyRequests, "daily gas-free trading limit reached")
	}
	if strings.TrimSpace(sponsorAddress) == "" {
		return fiber.NewError(fiber.StatusServiceUnavailable, "gas sponsor wallet is not configured")
	}
	balance, err := s.evm.NativeBalance(c.Context(), sponsorAddress)
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, "failed to check sponsor balance")
	}
	minBalance, _ := new(big.Int).SetString(policy.MinSponsorBalanceRaw, 10)
	currentBalance, ok := new(big.Int).SetString(balance.Raw, 10)
	if !ok || currentBalance.Cmp(minBalance) < 0 {
		return fiber.NewError(fiber.StatusServiceUnavailable, "gas-free trading is paused because sponsor balance is below threshold")
	}
	return nil
}

func normalizeTradingFeeBps(value int64) int64 {
	if value < 0 {
		return 0
	}
	if value > 1000 {
		return 1000
	}
	return value
}

func tradeFeeAmount(amount float64, feeBps int64) float64 {
	return roundTradeMoney(amount * float64(feeBps) / 10000)
}

func tradeEscrowTotal(amount float64, feeBps int64) float64 {
	return roundTradeMoney(amount + tradeFeeAmount(amount, feeBps))
}

func roundTradeMoney(value float64) float64 {
	return math.Round(value*100) / 100
}

func validateFixedTradeAmount(amount float64) error {
	if _, ok := allowedTradeAmounts[roundTradeMoney(amount)]; !ok {
		return fiber.NewError(fiber.StatusBadRequest, "choose one of the fixed BUDOL amounts: 10, 25, 50, 100, 250, 500")
	}
	return nil
}

func isDecimalString(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}
	for _, char := range trimmed {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func isBytes32String(value string) bool {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) != 66 || !strings.HasPrefix(trimmed, "0x") {
		return false
	}
	for _, char := range trimmed[2:] {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') && (char < 'A' || char > 'F') {
			return false
		}
	}
	return true
}

func (s Server) cashoutQuote(c *fiber.Ctx) error {
	user, err := s.authenticatedUser(c)
	if err != nil {
		return err
	}
	quote, err := s.store.QuoteCashout(c.Context(), user.ID, store.CashoutInput{
		PollID: c.Query("pollId"),
		Side:   c.Query("side"),
		Amount: parseOptionalFloat(c.Query("amount")),
	})
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	return c.JSON(fiber.Map{"quote": quote})
}

func (s Server) cashoutPosition(c *fiber.Ctx) error {
	user, err := s.authenticatedUser(c)
	if err != nil {
		return err
	}

	var request CashoutRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}

	s.collateralMu.Lock()
	defer s.collateralMu.Unlock()
	quote, err := s.store.QuoteCashout(c.Context(), user.ID, store.CashoutInput{
		PollID: request.PollID,
		Side:   request.Side,
		Amount: request.Amount,
	})
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	currentRequirement, err := s.store.CollateralRequirement(c.Context())
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, "failed to verify payout collateral")
	}
	projectedRequirement := store.ProjectedCollateralAfterCashout(currentRequirement, quote)
	collateralRelease := store.CollateralRelease(currentRequirement, projectedRequirement)

	result, _, err := s.sendWalletTokensLocked(c, []walletRecipientInput{
		{Address: user.WalletAddress, Amount: settlementAmountString(quote.Proceeds)},
	}, collateralRelease)
	if err != nil {
		return err
	}
	payoutStatus, payoutError := s.waitForWalletOpsTransactionStatus(c, result.TransactionIDs)

	cashout, err := s.store.CashoutPosition(c.Context(), user.ID, store.CashoutInput{
		PollID: request.PollID,
		Side:   request.Side,
		Amount: request.Amount,
	}, result.TransactionIDs, payoutStatus, payoutError)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to record cashout")
	}
	_, _ = s.store.CreateNotification(c.Context(), user.ID, "cashout", "Cashout submitted", cashout.PollTitle+": "+settlementAmountString(cashout.Proceeds)+" BUDOL payout status is "+payoutStatus+".", "/portfolio")

	portfolio, err := s.store.UserPortfolio(c.Context(), user.ID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load portfolio")
	}
	poll, _, err := s.store.GetPollBySlug(c.Context(), cashout.PollSlug, true)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load updated poll")
	}

	return c.JSON(fiber.Map{
		"cashout":        cashout,
		"portfolio":      portfolio,
		"poll":           poll,
		"payoutStatus":   payoutStatus,
		"payoutError":    payoutError,
		"transactionIds": result.TransactionIDs,
	})
}

func (s Server) verifyTradeEscrow(c *fiber.Ctx, user store.User, request TradeRequest) (tradeEscrowVerification, error) {
	projectWallet, err := s.projectWalletAddress(c.Context())
	if err != nil {
		return tradeEscrowVerification{}, fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	txHash := strings.TrimSpace(request.EscrowTxHash)
	if txHash == "" {
		return tradeEscrowVerification{}, fiber.NewError(fiber.StatusBadRequest, "escrow transaction hash is required")
	}
	tradingFeeBps := s.engineTradingFeeBps(c.Context())
	escrowAmount := tradeEscrowTotal(request.Amount, tradingFeeBps)
	quantity, err := walletops.TokenQuantity(settlementAmountString(escrowAmount), s.cfg.WelcomeTokenDecimals)
	if err != nil {
		return tradeEscrowVerification{}, fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	tokenContract := s.activeTokenContract(c.Context())
	escrowFrom, err := s.expectedEscrowFromAddress(c.Context(), user, request)
	if err != nil {
		return tradeEscrowVerification{}, err
	}
	ok, err := s.evm.WaitForERC20Transfer(c.Context(), tokenContract, txHash, escrowFrom, projectWallet, quantity)
	if err != nil {
		return tradeEscrowVerification{}, fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	if !ok {
		return tradeEscrowVerification{}, fiber.NewError(fiber.StatusBadRequest, "escrow transfer was not found or is still pending")
	}
	return tradeEscrowVerification{
		Status:     "verified",
		From:       strings.ToLower(escrowFrom),
		To:         projectWallet,
		Amount:     escrowAmount,
		VerifiedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func (s Server) expectedEscrowFromAddress(ctx context.Context, user store.User, request TradeRequest) (string, error) {
	requestedFrom := strings.ToLower(strings.TrimSpace(request.EscrowFromAddress))
	userWallet := strings.ToLower(strings.TrimSpace(user.WalletAddress))
	if requestedFrom == "" || strings.EqualFold(requestedFrom, userWallet) {
		return userWallet, nil
	}
	if !walletops.IsEVMAddress(requestedFrom) {
		return "", fiber.NewError(fiber.StatusBadRequest, "invalid escrowFromAddress")
	}
	if !s.cfg.SmartWalletEnabled {
		return "", fiber.NewError(fiber.StatusBadRequest, "smart wallet escrow is not enabled")
	}
	if !walletops.IsEVMAddress(s.cfg.SmartWalletFactoryAddress) {
		return "", fiber.NewError(fiber.StatusServiceUnavailable, "smart account factory is not configured")
	}
	expected, err := s.evm.SimpleAccountAddress(ctx, s.cfg.SmartWalletFactoryAddress, userWallet, 0)
	if err != nil {
		return "", fiber.NewError(fiber.StatusBadGateway, "failed to derive smart account address")
	}
	if !strings.EqualFold(requestedFrom, expected) {
		return "", fiber.NewError(fiber.StatusBadRequest, "escrowFromAddress does not match the configured smart wallet")
	}
	return strings.ToLower(expected), nil
}

func (s Server) ensureTradingBalance(c *fiber.Ctx, user store.User, amount float64) error {
	if amount <= 0 {
		return fiber.NewError(fiber.StatusBadRequest, "amount must be greater than zero")
	}
	balance, err := s.evm.ERC20Balance(c.Context(), s.activeTokenContract(c.Context()), user.WalletAddress, s.cfg.WelcomeTokenDecimals)
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, "failed to verify token balance")
	}
	tokenBalance, err := strconv.ParseFloat(balance.Formatted, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, "failed to parse token balance")
	}
	if amount > tokenBalance {
		return fiber.NewError(fiber.StatusBadRequest, "order exceeds the BUDOL balance in this wallet")
	}
	return nil
}

func (s Server) portfolio(c *fiber.Ctx) error {
	user, err := s.authenticatedUser(c)
	if err != nil {
		return err
	}
	portfolio, err := s.store.UserPortfolio(c.Context(), user.ID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load portfolio")
	}
	return c.JSON(fiber.Map{"portfolio": portfolio})
}

func (s Server) adminUsers(c *fiber.Ctx) error {
	users, err := s.store.ListUsers(c.Context())
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load users")
	}
	return c.JSON(fiber.Map{"users": users})
}

func (s Server) adminUserDetail(c *fiber.Ctx) error {
	detail, err := s.store.AdminUserDetail(c.Context(), c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	return c.JSON(fiber.Map{"detail": detail})
}

func (s Server) notifications(c *fiber.Ctx) error {
	user, err := s.authenticatedUser(c)
	if err != nil {
		return err
	}
	_ = s.syncWalletTransferNotifications(c, user)
	notifications, err := s.store.ListNotifications(c.Context(), user.ID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load notifications")
	}
	return c.JSON(fiber.Map{"notifications": notifications})
}

func (s Server) markNotificationRead(c *fiber.Ctx) error {
	user, err := s.authenticatedUser(c)
	if err != nil {
		return err
	}
	notification, ok, err := s.store.MarkNotificationRead(c.Context(), user.ID, c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to update notification")
	}
	if !ok {
		return fiber.NewError(fiber.StatusNotFound, "notification not found")
	}
	return c.JSON(fiber.Map{"notification": notification})
}

func (s Server) markAllNotificationsRead(c *fiber.Ctx) error {
	user, err := s.authenticatedUser(c)
	if err != nil {
		return err
	}
	count, err := s.store.MarkAllNotificationsRead(c.Context(), user.ID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to update notifications")
	}
	return c.JSON(fiber.Map{"updated": count})
}

func (s Server) watchlist(c *fiber.Ctx) error {
	user, err := s.authenticatedUser(c)
	if err != nil {
		return err
	}
	items, err := s.store.ListWatchlist(c.Context(), user.ID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load watchlist")
	}
	return c.JSON(fiber.Map{"watchlist": items})
}

func (s Server) addWatchlist(c *fiber.Ctx) error {
	user, err := s.authenticatedUser(c)
	if err != nil {
		return err
	}
	var request WatchlistRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	items, created, err := s.store.AddWatchlist(c.Context(), user.ID, request.Slug)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "failed to update watchlist")
	}
	if created {
		_, _ = s.store.CreateNotification(c.Context(), user.ID, "watchlist", "Added to watchlist", "This market is saved to your watchlist. Configure threshold alerts from its market page.", "/markets/"+slugifyForPath(request.Slug))
	}
	return c.JSON(fiber.Map{"watchlist": items})
}

func (s Server) removeWatchlist(c *fiber.Ctx) error {
	user, err := s.authenticatedUser(c)
	if err != nil {
		return err
	}
	items, err := s.store.RemoveWatchlist(c.Context(), user.ID, c.Params("slug"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "failed to update watchlist")
	}
	return c.JSON(fiber.Map{"watchlist": items})
}

func (s Server) walletBalance(c *fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	user, err := s.authenticatedUser(c)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	privacyToken := strings.ToLower(strings.TrimSpace("0xb06EC4ce262D8dbDc24Fac87479A49A7DC4cFb87"))
	tokenContract := s.activeTokenContract(c.Context())
	cacheKey := walletBalanceCacheKey(user.WalletAddress, tokenContract, privacyToken)
	if !truthyQuery(c.Query("refresh")) {
		if cached, ok := s.cachedWalletBalance(cacheKey); ok {
			return c.JSON(cached)
		}
	}

	type nativeResult struct {
		balance evm.NativeBalance
		err     error
	}
	type tokenResult struct {
		balance evm.TokenBalance
		err     error
	}

	var wg sync.WaitGroup
	nativeCh := make(chan nativeResult, 1)
	privacyCh := make(chan tokenResult, 1)
	budolCh := make(chan tokenResult, 1)

	wg.Add(1)
	go func() {
		defer wg.Done()
		balance, err := s.evm.NativeBalance(c.Context(), user.WalletAddress)
		nativeCh <- nativeResult{balance: balance, err: err}
	}()

	if walletops.IsEVMAddress(privacyToken) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			balance, err := s.evm.ERC20Balance(c.Context(), privacyToken, user.WalletAddress, 18)
			privacyCh <- tokenResult{balance: balance, err: err}
		}()
	} else {
		close(privacyCh)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		balance, err := s.evm.ERC20Balance(c.Context(), tokenContract, user.WalletAddress, s.cfg.WelcomeTokenDecimals)
		if err != nil && s.gmrEngine != nil && s.gmrEngine.Configured() {
			engineBalance, engineErr := s.gmrEngine.ERC20Balance(c.Context(), s.cfg.WelcomeTokenChainID, tokenContract, user.WalletAddress)
			if engineErr == nil {
				balance = evm.TokenBalance{
					Raw:           engineBalance.OwnedBalanceRaw,
					Formatted:     engineBalance.OwnedBalance,
					Decimals:      int(engineBalance.Decimals),
					WalletAddress: engineBalance.WalletAddress,
					TokenAddress:  engineBalance.ContractAddress,
					FetchedAt:     now,
				}
				err = nil
			}
		}
		budolCh <- tokenResult{balance: balance, err: err}
	}()

	wg.Wait()
	close(nativeCh)
	close(budolCh)
	if walletops.IsEVMAddress(privacyToken) {
		close(privacyCh)
	}

	balances := []fiber.Map{}
	native := <-nativeCh
	if native.err == nil {
		balances = append(balances, fiber.Map{
			"raw":           native.balance.Raw,
			"formatted":     native.balance.Formatted,
			"decimals":      native.balance.Decimals,
			"kind":          "native",
			"label":         "Gas token",
			"symbol":        "ETH",
			"walletAddress": native.balance.WalletAddress,
			"fetchedAt":     native.balance.FetchedAt,
		})
	} else {
		balances = append(balances, zeroWalletBalance("native", "Gas token", "ETH", user.WalletAddress, "", 18, now))
	}
	if privacy, ok := <-privacyCh; ok {
		if privacy.err == nil {
			balances = append(balances, walletTokenBalanceMap(privacy.balance, "Privacy token", "tZEN"))
		} else {
			balances = append(balances, zeroWalletBalance("erc20", "Privacy token", "tZEN", user.WalletAddress, privacyToken, 18, now))
		}
	}
	budol := <-budolCh
	warning := ""
	budolBalance := fiber.Map{}
	if budol.err == nil {
		budolBalance = walletTokenBalanceMap(budol.balance, "Trading token", "BUDOL")
	} else {
		budolBalance = zeroWalletBalance("erc20", "Trading token", "BUDOL", user.WalletAddress, tokenContract, s.cfg.WelcomeTokenDecimals, now)
		warning = "BUDOL balance is temporarily unavailable"
	}
	balances = append(balances, budolBalance)
	response := fiber.Map{
		"balance":  budolBalance,
		"balances": balances,
		"cached":   false,
	}
	if warning != "" {
		response["warning"] = warning
	}
	s.setCachedWalletBalance(cacheKey, response)
	return c.JSON(response)
}

func walletBalanceCacheKey(walletAddress string, tokenAddress string, privacyTokenAddress string) string {
	return strings.ToLower(strings.TrimSpace(walletAddress)) + ":" + strings.ToLower(strings.TrimSpace(tokenAddress)) + ":" + strings.ToLower(strings.TrimSpace(privacyTokenAddress))
}

func truthyQuery(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func (s Server) cachedWalletBalance(key string) (fiber.Map, bool) {
	if s.walletBalanceCacheMu == nil {
		return nil, false
	}
	s.walletBalanceCacheMu.Lock()
	defer s.walletBalanceCacheMu.Unlock()
	entry, ok := s.walletBalanceCache[key]
	if !ok || time.Now().UTC().After(entry.Expires) {
		if ok {
			delete(s.walletBalanceCache, key)
		}
		return nil, false
	}
	return walletBalanceResponseMap(entry, true), true
}

func (s Server) setCachedWalletBalance(key string, response fiber.Map) {
	if s.walletBalanceCacheMu == nil {
		return
	}
	balance, _ := response["balance"].(fiber.Map)
	balances, _ := response["balances"].([]fiber.Map)
	warning, _ := response["warning"].(string)
	s.walletBalanceCacheMu.Lock()
	defer s.walletBalanceCacheMu.Unlock()
	s.walletBalanceCache[key] = walletBalanceCacheEntry{
		Balance:  cloneFiberMap(balance),
		Balances: cloneFiberMaps(balances),
		Warning:  warning,
		Expires:  time.Now().UTC().Add(walletBalanceCacheTTL),
	}
}

func walletBalanceResponseMap(entry walletBalanceCacheEntry, cached bool) fiber.Map {
	response := fiber.Map{
		"balance":  cloneFiberMap(entry.Balance),
		"balances": cloneFiberMaps(entry.Balances),
		"cached":   cached,
	}
	if strings.TrimSpace(entry.Warning) != "" {
		response["warning"] = entry.Warning
	}
	return response
}

func cloneFiberMaps(items []fiber.Map) []fiber.Map {
	cloned := make([]fiber.Map, 0, len(items))
	for _, item := range items {
		cloned = append(cloned, cloneFiberMap(item))
	}
	return cloned
}

func cloneFiberMap(item fiber.Map) fiber.Map {
	cloned := fiber.Map{}
	for key, value := range item {
		cloned[key] = value
	}
	return cloned
}

func walletTokenBalanceMap(balance evm.TokenBalance, label string, symbol string) fiber.Map {
	return fiber.Map{
		"raw":           balance.Raw,
		"formatted":     balance.Formatted,
		"decimals":      balance.Decimals,
		"kind":          "erc20",
		"label":         label,
		"symbol":        strings.TrimSpace(symbol),
		"walletAddress": balance.WalletAddress,
		"tokenAddress":  balance.TokenAddress,
		"fetchedAt":     balance.FetchedAt,
	}
}

func zeroWalletBalance(kind string, label string, symbol string, walletAddress string, tokenAddress string, decimals int, fetchedAt string) fiber.Map {
	return fiber.Map{
		"raw":           "0",
		"formatted":     "0",
		"decimals":      decimals,
		"kind":          strings.TrimSpace(kind),
		"label":         strings.TrimSpace(label),
		"symbol":        strings.TrimSpace(symbol),
		"walletAddress": strings.ToLower(strings.TrimSpace(walletAddress)),
		"tokenAddress":  strings.ToLower(strings.TrimSpace(tokenAddress)),
		"fetchedAt":     fetchedAt,
	}
}

func (s Server) walletHistory(c *fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	user, err := s.authenticatedUser(c)
	if err != nil {
		return err
	}
	tokenRequests := []struct {
		address  string
		decimals int
		label    string
		symbol   string
	}{
		{
			address:  strings.ToLower(strings.TrimSpace("0xb06EC4ce262D8dbDc24Fac87479A49A7DC4cFb87")),
			decimals: 18,
			label:    "Privacy token",
			symbol:   "tZEN",
		},
		{
			address:  strings.ToLower(strings.TrimSpace(s.activeTokenContract(c.Context()))),
			decimals: s.cfg.WelcomeTokenDecimals,
			label:    "Trading token",
			symbol:   "BUDOL",
		},
	}

	transfers := []evm.TokenTransfer{}
	warnings := []string{}
	seenTokens := map[string]struct{}{}
	for _, token := range tokenRequests {
		if !walletops.IsEVMAddress(token.address) {
			continue
		}
		if _, seen := seenTokens[token.address]; seen {
			continue
		}
		seenTokens[token.address] = struct{}{}
		tokenTransfers, err := s.evm.ERC20TransferHistory(c.Context(), token.address, user.WalletAddress, token.decimals)
		if err != nil {
			warnings = append(warnings, token.symbol+" transfer history is temporarily unavailable")
			continue
		}
		for _, transfer := range tokenTransfers {
			transfer.TokenAddress = token.address
			transfer.TokenSymbol = token.symbol
			transfer.TokenLabel = token.label
			transfers = append(transfers, transfer)
		}
	}
	sort.Slice(transfers, func(i int, j int) bool {
		if transfers[i].BlockNumber == transfers[j].BlockNumber {
			return transfers[i].LogIndex > transfers[j].LogIndex
		}
		return transfers[i].BlockNumber > transfers[j].BlockNumber
	})
	if len(transfers) > 100 {
		transfers = transfers[:100]
	}
	_ = s.createReceivedTransferNotifications(c, user, transfers)
	response := fiber.Map{
		"transfers": transfers,
	}
	if len(warnings) > 0 {
		response["warning"] = strings.Join(warnings, "; ")
	}
	return c.JSON(response)
}

func (s Server) syncWalletTransferNotifications(c *fiber.Ctx, user store.User) error {
	transfers, err := s.evm.ERC20TransferHistory(c.Context(), s.activeTokenContract(c.Context()), user.WalletAddress, s.cfg.WelcomeTokenDecimals)
	if err != nil {
		return err
	}
	return s.createReceivedTransferNotifications(c, user, transfers)
}

func (s Server) createReceivedTransferNotifications(c *fiber.Ctx, user store.User, transfers []evm.TokenTransfer) error {
	existing, err := s.store.ListNotifications(c.Context(), user.ID)
	if err != nil {
		return err
	}
	welcomeGrantSender := ""
	if s.gmrEngine != nil && s.gmrEngine.Configured() {
		if wallet, ok, walletErr := s.gmrEngine.DefaultAdminWallet(c.Context()); walletErr == nil && ok {
			welcomeGrantSender = wallet.Address
		}
	}
	seen := map[string]bool{}
	for _, notification := range existing {
		if notification.Kind == "wallet_received" {
			seen[walletTransferNotificationLink(notification.Link)] = true
		}
	}
	for _, transfer := range transfers {
		if transfer.Direction != "received" || strings.TrimSpace(transfer.TransactionHash) == "" {
			continue
		}
		tokenSymbol := firstNonEmpty(strings.TrimSpace(transfer.TokenSymbol), "BUDOL")
		link := walletTransferLink(transfer.TransactionHash)
		if seen[link] {
			continue
		}
		grant := store.TokenGrant{}
		isWelcomeGrant := false
		var reconcileErr error
		if strings.EqualFold(tokenSymbol, "BUDOL") && welcomeGrantSender != "" && strings.EqualFold(transfer.Counterparty, welcomeGrantSender) {
			grant, isWelcomeGrant, reconcileErr = s.store.ReconcileWelcomeTokenGrantTransfer(
				c.Context(),
				user.ID,
				user.WalletAddress,
				transfer.Amount,
				transfer.TransactionHash,
				transfer.Timestamp,
			)
		}
		if reconcileErr == nil && isWelcomeGrant {
			// A successful submission already creates this notification. When an
			// ambiguous submission error is reconciled, create the single success
			// notification here instead of a second generic receive notification.
			hasWelcomeSuccess := false
			for _, notification := range existing {
				if notification.Kind == "welcome_tokens" && strings.EqualFold(walletTransferNotificationLink(notification.Link), link) {
					hasWelcomeSuccess = true
					break
				}
			}
			if !hasWelcomeSuccess {
				_, _ = s.store.CreateNotification(
					c.Context(),
					user.ID,
					"welcome_tokens",
					"Welcome tokens received",
					grant.Amount+" "+tokenSymbol+" welcome tokens were sent to your wallet.",
					link,
				)
			}
			seen[link] = true
			continue
		}
		_, _ = s.store.CreateNotification(
			c.Context(),
			user.ID,
			"wallet_received",
			tokenSymbol+" received",
			transfer.Amount+" "+tokenSymbol+" arrived in your wallet from "+shortAddressForAdmin(transfer.Counterparty)+".",
			link,
		)
		seen[link] = true
	}
	return nil
}

func (s Server) adminUpdateUser(c *fiber.Ctx) error {
	var input store.UserUpdateInput
	if err := c.BodyParser(&input); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	user, err := s.store.UpdateUser(c.Context(), c.Params("id"), input)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	_, _ = s.store.CreateAdminActivity(c.Context(), store.AdminActivityInput{
		Actor:      s.adminActor(c),
		Action:     "update_user",
		TargetType: "user",
		TargetID:   user.ID,
		Title:      "User updated",
		Detail:     shortAddressForAdmin(user.WalletAddress) + " set to " + user.Role + " / " + user.Status + ".",
	})
	return c.JSON(fiber.Map{"user": user})
}

func (s Server) adminTrades(c *fiber.Ctx) error {
	limit := int64(c.QueryInt("limit", 100))
	trades, err := s.store.ListTrades(c.Context(), c.Query("userId"), c.Query("pollId"), limit)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load trades")
	}
	return c.JSON(fiber.Map{"trades": trades})
}

func (s Server) adminReconcileTradeEscrow(c *fiber.Ctx) error {
	trade, err := s.store.GetTradeByID(c.Context(), c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	}
	if strings.TrimSpace(trade.EscrowTxHash) == "" {
		updated, updateErr := s.store.UpdateTradeEscrowReconciliation(c.Context(), trade.ID, "missing", "escrow transaction hash is missing", "", trade.EscrowFrom, trade.EscrowTo, trade.EscrowAmount)
		if updateErr != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to update escrow reconciliation")
		}
		return c.JSON(fiber.Map{"trade": updated})
	}

	from := firstNonEmpty(trade.EscrowFrom, trade.UserWalletAddress)
	to := firstNonEmpty(trade.EscrowTo, s.cfg.ProjectWallet)
	amount := trade.EscrowAmount
	if amount <= 0 {
		amount = trade.Amount
	}
	quantity, err := walletops.TokenQuantity(settlementAmountString(amount), s.cfg.WelcomeTokenDecimals)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	found, pending, err := s.evm.ERC20TransferInTransaction(c.Context(), s.activeTokenContract(c.Context()), trade.EscrowTxHash, from, to, quantity)
	status := "mismatch"
	errorMessage := "expected escrow transfer was not found in this transaction"
	verifiedAt := ""
	if err != nil {
		status = "error"
		errorMessage = err.Error()
	} else if pending {
		status = "pending"
		errorMessage = "transaction receipt is still pending"
	} else if found {
		status = "verified"
		errorMessage = ""
		verifiedAt = time.Now().UTC().Format(time.RFC3339)
	}

	updated, err := s.store.UpdateTradeEscrowReconciliation(c.Context(), trade.ID, status, errorMessage, verifiedAt, from, to, amount)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to update escrow reconciliation")
	}
	_, _ = s.store.CreateAdminActivity(c.Context(), adminActivity(c, s.adminActor(c), "reconcile_escrow", "trade", trade.ID, "Trade escrow reconciled", trade.PollTitle+" escrow rechecked as "+status+"."))
	return c.JSON(fiber.Map{"trade": updated})
}

func (s Server) adminRetryPrivateClaimPayout(c *fiber.Ctx) error {
	var request PayoutRetryRequest
	if err := c.BodyParser(&request); err != nil && len(c.Body()) > 0 {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	if !matchesConfirmation(request.ConfirmText, retryPrivateClaimPayoutConfirmText) {
		return fiber.NewError(fiber.StatusBadRequest, "type "+retryPrivateClaimPayoutConfirmText+" to retry the private claim payout")
	}
	if !s.cfg.ShieldedPayoutDirectFallback {
		return fiber.NewError(fiber.StatusConflict, "direct fallback for unsupported shielded payout amounts is disabled")
	}
	trade, err := s.store.GetTradeByID(c.Context(), c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "trade not found")
	}
	if trade.PayoutStatus != "claim_failed" || trade.PrivateClaimID == "" || trade.SettlementPayout <= 0 {
		return fiber.NewError(fiber.StatusBadRequest, "trade does not have a failed private claim payout")
	}
	if len(trade.PayoutTransactionIDs) > 0 {
		return fiber.NewError(fiber.StatusConflict, "failed claim has payout transaction IDs and must be reconciled before retry")
	}
	if !walletops.IsEVMAddress(trade.UserWalletAddress) {
		return fiber.NewError(fiber.StatusBadRequest, "trade owner wallet is invalid")
	}

	result, _, err := s.sendWalletTokensWithRelease(c, []walletRecipientInput{{
		Address: trade.UserWalletAddress,
		Amount:  settlementAmountString(trade.SettlementPayout),
	}}, trade.SettlementPayout)
	if err != nil {
		return err
	}
	payoutStatus, payoutError := s.waitForWalletOpsTransactionStatus(c, result.TransactionIDs)
	claim, updatedTrade, err := s.store.CompletePrivateClaimPayout(c.Context(), trade.UserID, trade.PrivateClaimID, result.TransactionIDs, payoutStatus, payoutError)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "payout was submitted but its claim record could not be updated")
	}
	_, _ = s.store.CreateNotification(c.Context(), trade.UserID, "private_claim", "Private claim payout retried", trade.PollTitle+": "+settlementAmountString(trade.SettlementPayout)+" BUDOL was paid directly after the shielded denomination mismatch.", "/portfolio")
	_, _ = s.store.CreateAdminActivity(c.Context(), adminActivity(c, s.adminActor(c), "retry_private_claim_payout", "trade", trade.ID, "Private claim payout retried", trade.PollTitle+" paid "+settlementAmountString(trade.SettlementPayout)+" BUDOL directly."))
	return c.JSON(fiber.Map{
		"claim":          claim,
		"trade":          updatedTrade,
		"payoutMode":     "direct_fallback",
		"payoutStatus":   payoutStatus,
		"payoutError":    payoutError,
		"transactionIds": result.TransactionIDs,
	})
}

func (s Server) adminReconcilePrivateClaimPayout(c *fiber.Ctx) error {
	var request PayoutRetryRequest
	if err := c.BodyParser(&request); err != nil && len(c.Body()) > 0 {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	if !matchesConfirmation(request.ConfirmText, reconcilePrivateClaimPayoutConfirmText) {
		return fiber.NewError(fiber.StatusBadRequest, "type "+reconcilePrivateClaimPayoutConfirmText+" to reconcile the private claim payout")
	}
	trade, err := s.store.GetTradeByID(c.Context(), c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "trade not found")
	}
	if trade.PrivateClaimID == "" || len(trade.PayoutTransactionIDs) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "trade has no private claim payout transaction to reconcile")
	}
	payoutStatus, payoutError, transactionHash := s.walletopsTransactionStatus(c, trade.PayoutTransactionIDs[0])
	claim, updatedTrade, err := s.store.CompletePrivateClaimPayout(c.Context(), trade.UserID, trade.PrivateClaimID, trade.PayoutTransactionIDs, payoutStatus, payoutError)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to update the private claim payout status")
	}
	_, _ = s.store.CreateAdminActivity(c.Context(), adminActivity(c, s.adminActor(c), "reconcile_private_claim_payout", "trade", trade.ID, "Private claim payout reconciled", trade.PollTitle+" payout rechecked as "+payoutStatus+"."))
	return c.JSON(fiber.Map{
		"claim":           claim,
		"trade":           updatedTrade,
		"payoutStatus":    payoutStatus,
		"payoutError":     payoutError,
		"transactionHash": transactionHash,
		"transactionIds":  trade.PayoutTransactionIDs,
	})
}

func (s Server) adminPollClassifications(c *fiber.Ctx) error {
	classifications, err := s.store.ListPollClassifications(c.Context())
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load poll classifications")
	}
	return c.JSON(fiber.Map{"classifications": classifications})
}

func (s Server) adminUpsertPollClassification(c *fiber.Ctx) error {
	var input store.PollClassificationInput
	if err := c.BodyParser(&input); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	classification, err := s.store.UpsertPollClassification(c.Context(), input)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	_, _ = s.store.CreateAdminActivity(c.Context(), store.AdminActivityInput{
		Actor:      s.adminActor(c),
		Action:     "save_classification",
		TargetType: "classification",
		TargetID:   classification.ID,
		Title:      "Classification saved",
		Detail:     classification.DisplayName + " is available as " + classification.CallName + ".",
	})
	return c.JSON(fiber.Map{"classification": classification})
}

func (s Server) adminPolls(c *fiber.Ctx) error {
	polls, err := s.store.ListPolls(c.Context(), false)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load polls")
	}
	return c.JSON(fiber.Map{"polls": polls})
}

func (s Server) adminCreatePoll(c *fiber.Ctx) error {
	var input store.PollInput
	if err := c.BodyParser(&input); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	if strings.TrimSpace(input.PollType) == "multiple_choice_yes_no" {
		createdPolls, err := s.createMultiChoicePolls(c, input)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		firstPoll := createdPolls[0]
		return c.JSON(fiber.Map{"poll": firstPoll, "polls": createdPolls})
	}
	poll, err := s.store.UpsertPoll(c.Context(), input)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	_, _ = s.store.CreateAdminActivity(c.Context(), store.AdminActivityInput{
		Actor:      s.adminActor(c),
		Action:     "create_poll",
		TargetType: "poll",
		TargetID:   poll.ID,
		Title:      "Poll saved",
		Detail:     adminPollDetail(poll, input.AuditReason),
	})
	return c.JSON(fiber.Map{"poll": poll})
}

func (s Server) createMultiChoicePolls(c *fiber.Ctx, input store.PollInput) ([]store.Poll, error) {
	choices := normalizedChoiceLabels(input.ChoiceLabels)
	if len(choices) < 2 {
		return nil, errors.New("multiple-choice yes/no polls need at least two choices")
	}
	groupSlug := slugifyForPath(firstNonEmpty(input.Slug, input.Title))
	if groupSlug == "" {
		return nil, errors.New("multiple-choice yes/no polls need a question or slug")
	}
	groupTitle := strings.TrimSpace(input.Title)
	if groupTitle == "" {
		groupTitle = groupSlug
	}
	choiceSlugs := make([]string, 0, len(choices))
	seenChoiceSlugs := map[string]bool{}
	for _, choice := range choices {
		choiceSlug := slugifyForPath(choice)
		if choiceSlug == "" {
			return nil, errors.New("multiple-choice yes/no choices need URL-safe labels")
		}
		if seenChoiceSlugs[choiceSlug] {
			return nil, errors.New("multiple-choice yes/no choices produce duplicate URL slugs")
		}
		seenChoiceSlugs[choiceSlug] = true
		choiceSlugs = append(choiceSlugs, choiceSlug)
	}

	created := make([]store.Poll, 0, len(choices))
	for index, choice := range choices {
		choiceInput := input
		choiceInput.ChoiceLabels = nil
		choiceInput.Slug = groupSlug + "-" + choiceSlugs[index]
		choiceInput.Title = groupTitle + ": " + choice
		choiceInput.OutcomeA = "Yes"
		choiceInput.OutcomeB = "No"
		choiceInput.MarketGroupID = groupSlug
		choiceInput.MarketGroupTitle = groupTitle
		choiceInput.MarketChoiceLabel = choice
		choiceInput.MarketChoiceIndex = int64(index + 1)
		choiceInput.SortOrder = input.SortOrder + int64(index)
		poll, err := s.store.UpsertPoll(c.Context(), choiceInput)
		if err != nil {
			return nil, err
		}
		created = append(created, poll)
		_, _ = s.store.CreateAdminActivity(c.Context(), store.AdminActivityInput{
			Actor:      s.adminActor(c),
			Action:     "create_poll",
			TargetType: "poll",
			TargetID:   poll.ID,
			Title:      "Choice market saved",
			Detail:     adminPollDetail(poll, input.AuditReason),
		})
	}
	return created, nil
}

func (s Server) adminUpdatePoll(c *fiber.Ctx) error {
	var input store.PollInput
	if err := c.BodyParser(&input); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	currentPoll, ok, err := s.store.GetPollByID(c.Context(), c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load poll")
	}
	if !ok {
		return fiber.NewError(fiber.StatusNotFound, "poll not found")
	}
	nextStatus := strings.ToLower(strings.TrimSpace(input.Status))
	if nextStatus == "" {
		nextStatus = "draft"
	}
	if isSettledPoll(currentPoll) && nextStatus != currentPoll.Status {
		return fiber.NewError(fiber.StatusBadRequest, "settled markets cannot be reopened or moved to another status")
	}
	poll, err := s.store.UpdatePoll(c.Context(), c.Params("id"), input)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	_, _ = s.store.CreateAdminActivity(c.Context(), store.AdminActivityInput{
		Actor:      s.adminActor(c),
		Action:     "update_poll",
		TargetType: "poll",
		TargetID:   poll.ID,
		Title:      "Poll updated",
		Detail:     adminPollDetail(poll, input.AuditReason),
	})
	_, _ = s.store.NotifyWatchers(c.Context(), poll.Slug, "", "watchlist_poll_update", "Watched market updated", poll.Title+" is now "+poll.Status+" / "+poll.Visibility+".", "/markets/"+poll.Slug)
	return c.JSON(fiber.Map{"poll": poll})
}

func (s Server) adminResolvePoll(c *fiber.Ctx) error {
	var request PollResolutionRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	request.Status = strings.ToLower(strings.TrimSpace(request.Status))
	request.Outcome = strings.ToLower(strings.TrimSpace(request.Outcome))

	currentPoll, ok, err := s.store.GetPollByID(c.Context(), c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load poll")
	}
	if !ok {
		return fiber.NewError(fiber.StatusNotFound, "poll not found")
	}
	if isSettledPoll(currentPoll) {
		if request.Status == "resolved" || request.Status == "cancelled" {
			return fiber.NewError(fiber.StatusBadRequest, "market settlement is already recorded")
		}
		if request.Status != currentPoll.Status {
			return fiber.NewError(fiber.StatusBadRequest, "settled markets cannot be reopened or moved to another status")
		}
	}

	if request.Status == "resolved" || request.Status == "cancelled" {
		if !matchesConfirmation(request.ConfirmText, settlementConfirmText) {
			return fiber.NewError(fiber.StatusBadRequest, "type "+settlementConfirmText+" to resolve or cancel a poll with settlement")
		}
		preview, err := s.store.PreviewPollSettlement(c.Context(), c.Params("id"), request.Status, request.Outcome)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}

		transactionIDs := []string{}
		payoutStatus := "none"
		payoutError := ""
		if preview.PayoutRequired > 0 {
			payoutStatus = "claimable"
		}

		poll, err := s.store.UpdatePollResolution(c.Context(), c.Params("id"), request.Status, request.Outcome, s.adminActor(c), request.ResolutionSource, request.ResolutionEvidenceURL, request.ResolutionNotes)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		settlement, err := s.store.SettlePollTrades(c.Context(), c.Params("id"), request.Status, request.Outcome, transactionIDs, payoutStatus, payoutError)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to record settlement")
		}
		s.acceptPrivateClaimRegistryRoot(c.Context(), poll.ID, request.Status, request.Outcome)
		s.notifySettlementRecipients(c, settlement, payoutStatus)
		_, _ = s.store.CreateAdminActivity(c.Context(), store.AdminActivityInput{
			Actor:      s.adminActor(c),
			Action:     request.Status,
			TargetType: "poll",
			TargetID:   poll.ID,
			Title:      "Poll " + request.Status,
			Detail:     poll.Title + " affected " + strconv.FormatInt(settlement.RecipientCount, 10) + " users.",
			IPAddress:  c.IP(),
			UserAgent:  c.Get("User-Agent"),
		})
		_, _ = s.store.NotifyMarketResolutionAlerts(c.Context(), poll.Slug, request.Status)
		return c.JSON(fiber.Map{
			"poll":           poll,
			"settlement":     settlement,
			"payoutStatus":   payoutStatus,
			"payoutError":    payoutError,
			"transactionIds": transactionIDs,
		})
	}

	poll, err := s.store.UpdatePollResolution(c.Context(), c.Params("id"), request.Status, request.Outcome, s.adminActor(c), request.ResolutionSource, request.ResolutionEvidenceURL, request.ResolutionNotes)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	_, _ = s.store.CreateAdminActivity(c.Context(), store.AdminActivityInput{
		Actor:      s.adminActor(c),
		Action:     "update_resolution_state",
		TargetType: "poll",
		TargetID:   poll.ID,
		Title:      "Poll state changed",
		Detail:     poll.Title + " moved to " + poll.Status + ".",
	})
	_, _ = s.store.NotifyWatchers(c.Context(), poll.Slug, "", "watchlist_poll_update", "Watched market state changed", poll.Title+" moved to "+poll.Status+".", "/markets/"+poll.Slug)
	return c.JSON(fiber.Map{"poll": poll})
}

func isSettledPoll(poll store.Poll) bool {
	if poll.Status != "resolved" && poll.Status != "cancelled" {
		return false
	}
	return poll.SettlementStatus != "" || poll.ResolvedAt != "" || poll.ResolutionOutcome != ""
}

func (s Server) adminRetrySettlementPayout(c *fiber.Ctx) error {
	var request PayoutRetryRequest
	if err := c.BodyParser(&request); err != nil && len(c.Body()) > 0 {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	if !matchesConfirmation(request.ConfirmText, retryPayoutConfirmText) {
		return fiber.NewError(fiber.StatusBadRequest, "type "+retryPayoutConfirmText+" to retry settlement payout")
	}
	s.collateralMu.Lock()
	defer s.collateralMu.Unlock()
	preview, err := s.store.SettlementPayoutRetryPreview(c.Context(), c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if preview.PayoutRequired <= 0 || len(preview.Recipients) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "no payable settlement recipients to retry")
	}

	result, err := s.sendSettlementPayoutsLocked(c, preview)
	if err != nil {
		return err
	}
	payoutStatus, payoutError := s.waitForWalletOpsTransactionStatus(c, result.TransactionIDs)
	settlement, err := s.store.RecordSettlementPayoutRetry(c.Context(), c.Params("id"), result.TransactionIDs, payoutStatus, payoutError)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to record payout retry")
	}
	s.notifySettlementRecipients(c, settlement, payoutStatus)
	_, _ = s.store.CreateAdminActivity(c.Context(), store.AdminActivityInput{
		Actor:      s.adminActor(c),
		Action:     "retry_payout",
		TargetType: "poll",
		TargetID:   c.Params("id"),
		Title:      "Settlement payout retried",
		Detail:     preview.PollTitle + " retried " + strconv.FormatInt(settlement.RecipientCount, 10) + " recipients.",
		IPAddress:  c.IP(),
		UserAgent:  c.Get("User-Agent"),
	})
	return c.JSON(fiber.Map{
		"settlement":     settlement,
		"payoutStatus":   payoutStatus,
		"payoutError":    payoutError,
		"transactionIds": result.TransactionIDs,
	})
}

func (s Server) adminReconcileSettlementPayout(c *fiber.Ctx) error {
	detail, err := s.store.AdminSettlementDetail(c.Context(), c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	transactionIDs := detail.Poll.SettlementPayoutTransactionIDs
	if len(transactionIDs) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "no settlement payout transaction IDs to reconcile")
	}

	payoutStatus, payoutError, transactionHash := s.walletopsTransactionStatus(c, transactionIDs[0])
	poll, err := s.store.UpdateSettlementPayoutReconciliation(c.Context(), c.Params("id"), payoutStatus, payoutError, transactionHash)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to update payout reconciliation")
	}
	_, _ = s.store.CreateAdminActivity(c.Context(), adminActivity(c, s.adminActor(c), "reconcile_payout", "poll", poll.ID, "Settlement payout reconciled", poll.Title+" payout rechecked as "+payoutStatus+"."))
	return c.JSON(fiber.Map{
		"poll":            poll,
		"payoutStatus":    payoutStatus,
		"payoutError":     payoutError,
		"transactionHash": transactionHash,
	})
}

func (s Server) adminActivity(c *fiber.Ctx) error {
	activity, err := s.store.ListAdminActivity(c.Context())
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load admin activity")
	}
	return c.JSON(fiber.Map{"activity": activity})
}

func (s Server) adminComments(c *fiber.Ctx) error {
	comments, err := s.store.ListAllMarketComments(c.Context())
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load comments")
	}
	return c.JSON(fiber.Map{"comments": comments})
}

func (s Server) adminModerateComment(c *fiber.Ctx) error {
	var request CommentModerationRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	comment, err := s.store.UpdateMarketCommentStatus(c.Context(), c.Params("id"), request.Status)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	_, _ = s.store.CreateAdminActivity(c.Context(), store.AdminActivityInput{
		Actor:      s.adminActor(c),
		Action:     "moderate_comment",
		TargetType: "comment",
		TargetID:   comment.ID,
		Title:      "Comment moderated",
		Detail:     comment.Actor + " comment on " + comment.PollSlug + " set to " + comment.Status + ".",
	})
	if comment.Status != "visible" {
		_, _ = s.store.CreateNotification(c.Context(), comment.UserID, "comment_moderation", "Comment moderated", "Your comment on "+comment.PollSlug+" was set to "+comment.Status+".", "/markets/"+comment.PollSlug)
	}
	return c.JSON(fiber.Map{"comment": comment})
}

func (s Server) adminSettlementPreview(c *fiber.Ctx) error {
	status := c.Query("status", "resolved")
	outcome := c.Query("outcome")
	preview, err := s.store.PreviewPollSettlement(c.Context(), c.Params("id"), status, outcome)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	balance := s.projectWalletTokenBalance(c)
	collateral, err := s.currentCollateralStatus(c.Context())
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, "failed to load payout collateral")
	}
	projectWallet, err := s.projectWalletAddress(c.Context())
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	return c.JSON(fiber.Map{
		"settlement": preview,
		"wallet": fiber.Map{
			"chainId":              s.cfg.WelcomeTokenChainID,
			"chainName":            networkName(s.cfg.WelcomeTokenChainID),
			"tokenAddress":         s.activeTokenContract(c.Context()),
			"projectWallet":        projectWallet,
			"projectWalletBalance": balance,
			"collateral":           collateral,
		},
	})
}

func (s Server) adminSettlementDetail(c *fiber.Ctx) error {
	detail, err := s.store.AdminSettlementDetail(c.Context(), c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	return c.JSON(fiber.Map{"detail": detail})
}

func (s Server) adminWalletConfig(c *fiber.Ctx) error {
	tokenContract, err := s.requireWalletConfig(c.Context())
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	balance := s.projectWalletTokenBalance(c)
	projectWallet, err := s.projectWalletAddress(c.Context())
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	collateral, err := s.currentCollateralStatus(c.Context())
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, "failed to load payout collateral")
	}
	gasFreeEnabled, gaslessSpenderAddress := s.engineGasFreeConfig(c.Context())
	tradingFeeBps := s.engineTradingFeeBps(c.Context())
	sponsorPolicy := s.sponsorPolicy(c.Context())
	gasPayerBalance := ""
	gasPayerBalanceRaw := ""
	if gaslessSpenderAddress != "" {
		if balance, err := s.evm.NativeBalance(c.Context(), gaslessSpenderAddress); err == nil {
			gasPayerBalance = balance.Formatted
			gasPayerBalanceRaw = balance.Raw
		}
	}
	return c.JSON(fiber.Map{
		"wallet": fiber.Map{
			"chainId":              s.cfg.WelcomeTokenChainID,
			"chainName":            networkName(s.cfg.WelcomeTokenChainID),
			"tokenAddress":         tokenContract,
			"tokenDecimals":        s.cfg.WelcomeTokenDecimals,
			"projectWallet":        projectWallet,
			"projectWalletBalance": balance,
			"burnAddress":          deadBurnAddress,
			"collateral":           collateral,
			"tradingFee": fiber.Map{
				"bps":  tradingFeeBps,
				"rate": float64(tradingFeeBps) / 10000,
			},
			"sponsorPolicy": sponsorPolicy,
			"privacyAccess": fiber.Map{
				"collectorAddress":  s.privacyFeeCollectorAddress(c.Context()),
				"currency":          "tZEN",
				"decimals":          18,
				"hidePositionFee":   s.privacyFeeAmount(c.Context(), privacyFeeHidePosition),
				"privateClaimFee":   s.privacyFeeAmount(c.Context(), privacyFeePrivateClaim),
				"shieldedPayoutFee": s.privacyFeeAmount(c.Context(), privacyFeeShieldedPayout),
			},
			"gasFreeTrading": fiber.Map{
				"enabled":               gasFreeEnabled,
				"scope":                 "self_custody_wallets",
				"gaslessSpenderAddress": strings.ToLower(gaslessSpenderAddress),
				"gasPayerBalance":       gasPayerBalance,
				"gasPayerBalanceRaw":    gasPayerBalanceRaw,
			},
		},
	})
}

func (s Server) adminUpdateGasFreeTrading(c *fiber.Ctx) error {
	if s.gmrEngine == nil || !s.gmrEngine.Configured() {
		return fiber.NewError(fiber.StatusServiceUnavailable, "GMR Engine is not configured")
	}
	var request GasFreeTradingUpdateRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	result, err := s.gmrEngine.UpdateGasFree(c.Context(), request.Enabled)
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	_, _ = s.store.CreateAdminActivity(c.Context(), store.AdminActivityInput{
		Actor:      s.adminActor(c),
		Action:     "update_gas_free_trading",
		TargetType: "wallet",
		TargetID:   result.App.ID,
		Detail:     "Self-custody gas-free trading set to " + strconv.FormatBool(result.App.GasFreeEnabled) + ".",
		IPAddress:  c.IP(),
		UserAgent:  c.Get("User-Agent"),
	})
	return s.adminWalletConfig(c)
}

func (s Server) adminUpdateTradingFee(c *fiber.Ctx) error {
	if s.gmrEngine == nil || !s.gmrEngine.Configured() {
		return fiber.NewError(fiber.StatusServiceUnavailable, "GMR Engine is not configured")
	}
	var request TradingFeeUpdateRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	if request.TradingFeeBps < 0 || request.TradingFeeBps > 1000 {
		return fiber.NewError(fiber.StatusBadRequest, "tradingFeeBps must be between 0 and 1000")
	}
	result, err := s.gmrEngine.UpdateTradingFee(c.Context(), request.TradingFeeBps)
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	_, _ = s.store.CreateAdminActivity(c.Context(), store.AdminActivityInput{
		Actor:      s.adminActor(c),
		Action:     "update_trading_fee",
		TargetType: "wallet",
		TargetID:   result.App.ID,
		Detail:     "Trading fee set to " + strconv.FormatInt(result.App.TradingFeeBps, 10) + " bps.",
		IPAddress:  c.IP(),
		UserAgent:  c.Get("User-Agent"),
	})
	return s.adminWalletConfig(c)
}

func (s Server) adminUpdateSponsorPolicy(c *fiber.Ctx) error {
	var request SponsorPolicyUpdateRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	policy := normalizeSponsorPolicy(SponsorPolicy{
		MaxSponsoredTradesPerWalletDay: request.MaxSponsoredTradesPerWalletDay,
		MaxSponsoredTradesGlobalDay:    request.MaxSponsoredTradesGlobalDay,
		MinSponsorBalanceRaw:           request.MinSponsorBalanceRaw,
	})
	payload, err := json.Marshal(policy)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to encode sponsor policy")
	}
	if err := s.store.SetSystemSetting(c.Context(), sponsorPolicySetting, string(payload)); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to update sponsor policy")
	}
	_, _ = s.store.CreateAdminActivity(c.Context(), store.AdminActivityInput{
		Actor:      s.adminActor(c),
		Action:     "update_sponsor_policy",
		TargetType: "wallet",
		TargetID:   "gas_free_sponsor_policy",
		Detail:     "Gas-free sponsor limits updated.",
		IPAddress:  c.IP(),
		UserAgent:  c.Get("User-Agent"),
	})
	return s.adminWalletConfig(c)
}

func (s Server) adminUpdatePrivacyAccessSettings(c *fiber.Ctx) error {
	var request PrivacyAccessSettingsUpdateRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	collector := strings.ToLower(strings.TrimSpace(request.CollectorAddress))
	if collector != "" && !walletops.IsEVMAddress(collector) {
		return fiber.NewError(fiber.StatusBadRequest, "collectorAddress must be a valid EVM address")
	}
	fees := map[string]string{
		privacyHidePositionFeeSetting:   strings.TrimSpace(request.HidePositionFee),
		privacyPrivateClaimFeeSetting:   strings.TrimSpace(request.PrivateClaimFee),
		privacyShieldedPayoutFeeSetting: strings.TrimSpace(request.ShieldedPayoutFee),
	}
	for key, value := range fees {
		if value == "" {
			value = "0"
		}
		if !isUnsignedIntegerString(value) {
			return fiber.NewError(fiber.StatusBadRequest, key+" must be raw native tZEN base units")
		}
		fees[key] = value
	}
	if err := s.store.SetSystemSetting(c.Context(), privacyFeeCollectorSetting, collector); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to update privacy fee collector")
	}
	for key, value := range fees {
		if err := s.store.SetSystemSetting(c.Context(), key, value); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to update privacy fee settings")
		}
	}
	_, _ = s.store.CreateAdminActivity(c.Context(), store.AdminActivityInput{
		Actor:      s.adminActor(c),
		Action:     "update_privacy_access_settings",
		TargetType: "wallet",
		TargetID:   "privacy_access",
		Detail:     "Privacy access fee settings updated.",
		IPAddress:  c.IP(),
		UserAgent:  c.Get("User-Agent"),
	})
	return s.adminWalletConfig(c)
}

func (s Server) adminUpdateWalletTokenContract(c *fiber.Ctx) error {
	var request TokenContractUpdateRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	tokenAddress := strings.ToLower(strings.TrimSpace(request.TokenAddress))
	if !walletops.IsEVMAddress(tokenAddress) {
		return fiber.NewError(fiber.StatusBadRequest, "token contract must be a valid EVM address")
	}
	s.collateralMu.Lock()
	defer s.collateralMu.Unlock()
	if err := s.ensureTokenContractCollateralized(c.Context(), tokenAddress); err != nil {
		if errors.Is(err, errUnderCollateralized) {
			return fiber.NewError(fiber.StatusConflict, "token contract change rejected: the replacement token does not cover reserved market payouts")
		}
		return fiber.NewError(fiber.StatusBadGateway, "failed to verify replacement token collateral")
	}
	oldAddress := s.activeTokenContract(c.Context())
	if err := s.store.SetSystemSetting(c.Context(), budolTokenContractSetting, tokenAddress); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to update token contract")
	}
	_, _ = s.store.CreateAdminActivity(c.Context(), store.AdminActivityInput{
		Actor:      s.adminActor(c),
		Action:     "update_token_contract",
		TargetType: "wallet",
		TargetID:   tokenAddress,
		Title:      "BUDOL token contract updated",
		Detail:     shortAddressForAdmin(oldAddress) + " -> " + shortAddressForAdmin(tokenAddress),
		IPAddress:  c.IP(),
		UserAgent:  c.Get("User-Agent"),
	})
	return s.adminWalletConfig(c)
}

func (s Server) adminWelcomeGrants(c *fiber.Ctx) error {
	overview, err := s.welcomeGrantOverview(c)
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	return c.JSON(overview)
}

func (s Server) adminRetryWelcomeGrants(c *fiber.Ctx) error {
	if s.activeTokenContract(c.Context()) == "" || s.cfg.WelcomeTokenAmount == "" {
		return fiber.NewError(fiber.StatusBadRequest, "welcome token grant is not configured")
	}
	quantity, err := walletops.TokenQuantity(s.cfg.WelcomeTokenAmount, s.cfg.WelcomeTokenDecimals)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	grants, err := s.store.ListTokenGrants(c.Context(), "welcome", []string{"pending", "failed"}, 100)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load token grants")
	}
	results := []store.TokenGrant{}
	for _, grant := range grants {
		user, ok, err := s.store.GetByID(c.Context(), grant.UserID)
		if err != nil || !ok {
			updated, _ := s.store.UpdateTokenGrantStatus(c.Context(), grant.ID, "failed", nil, "grant user no longer exists", nil)
			results = append(results, updated)
			continue
		}
		results = append(results, s.attemptWelcomeTokenGrant(c, user, grant, quantity))
	}
	_, _ = s.store.CreateAdminActivity(c.Context(), store.AdminActivityInput{
		Actor:      s.adminActor(c),
		Action:     "retry_welcome_grants",
		TargetType: "wallet",
		Title:      "Welcome grants retried",
		Detail:     strconv.Itoa(len(grants)) + " pending or failed welcome grants were retried.",
		IPAddress:  c.IP(),
		UserAgent:  c.Get("User-Agent"),
	})
	overview, err := s.welcomeGrantOverview(c)
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	overview["retried"] = len(grants)
	overview["results"] = results
	return c.JSON(overview)
}

func (s Server) adminWalletTransfer(c *fiber.Ctx) error {
	var request WalletTransferRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	if !matchesConfirmation(request.ConfirmText, transferConfirmText) {
		return fiber.NewError(fiber.StatusBadRequest, "type "+transferConfirmText+" to transfer admin wallet funds")
	}
	result, quantity, err := s.sendWalletTokens(c, []walletRecipientInput{
		{Address: request.To, Amount: request.Amount},
	})
	if err != nil {
		return err
	}
	_, _ = s.store.CreateAdminActivity(c.Context(), store.AdminActivityInput{
		Actor:      s.adminActor(c),
		Action:     "wallet_transfer",
		TargetType: "wallet",
		TargetID:   request.To,
		Title:      "Wallet transfer submitted",
		Detail:     request.Amount + " BUDOL sent to " + shortAddressForAdmin(request.To) + ".",
		IPAddress:  c.IP(),
		UserAgent:  c.Get("User-Agent"),
	})
	return c.JSON(walletActionResponse("transfer", 1, quantity, result))
}

func (s Server) adminWalletAirdrop(c *fiber.Ctx) error {
	var request WalletAirdropRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	if !matchesConfirmation(request.ConfirmText, airdropConfirmText) {
		return fiber.NewError(fiber.StatusBadRequest, "type "+airdropConfirmText+" to airdrop admin wallet funds")
	}
	if len(request.Recipients) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "add at least one airdrop recipient")
	}
	if len(request.Recipients) > 100 {
		return fiber.NewError(fiber.StatusBadRequest, "airdrop is limited to 100 recipients per request")
	}

	recipients := make([]walletRecipientInput, 0, len(request.Recipients))
	for _, recipient := range request.Recipients {
		recipients = append(recipients, walletRecipientInput{
			Address: recipient.Address,
			Amount:  recipient.Amount,
		})
	}

	result, quantity, err := s.sendWalletTokens(c, recipients)
	if err != nil {
		return err
	}
	_, _ = s.store.CreateAdminActivity(c.Context(), store.AdminActivityInput{
		Actor:      s.adminActor(c),
		Action:     "wallet_airdrop",
		TargetType: "wallet",
		Title:      "Airdrop submitted",
		Detail:     strconv.Itoa(len(recipients)) + " recipients queued from the admin wallet.",
		IPAddress:  c.IP(),
		UserAgent:  c.Get("User-Agent"),
	})
	return c.JSON(walletActionResponse("airdrop", len(recipients), quantity, result))
}

func (s Server) adminWalletBurn(c *fiber.Ctx) error {
	var request WalletBurnRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	if !matchesConfirmation(request.ConfirmText, burnConfirmText) {
		return fiber.NewError(fiber.StatusBadRequest, "type "+burnConfirmText+" to burn admin wallet funds")
	}
	result, quantity, err := s.sendWalletTokens(c, []walletRecipientInput{
		{Address: deadBurnAddress, Amount: request.Amount},
	})
	if err != nil {
		return err
	}
	_, _ = s.store.CreateAdminActivity(c.Context(), store.AdminActivityInput{
		Actor:      s.adminActor(c),
		Action:     "wallet_burn",
		TargetType: "wallet",
		TargetID:   deadBurnAddress,
		Title:      "Burn submitted",
		Detail:     request.Amount + " BUDOL sent to the burn address.",
		IPAddress:  c.IP(),
		UserAgent:  c.Get("User-Agent"),
	})
	response := walletActionResponse("burn", 1, quantity, result)
	response["burnAddress"] = deadBurnAddress
	return c.JSON(response)
}

func (s Server) activeTokenContract(ctx context.Context) string {
	if s.store != nil {
		value, ok, err := s.store.GetSystemSetting(ctx, budolTokenContractSetting)
		if err == nil && ok && strings.TrimSpace(value) != "" {
			return strings.ToLower(strings.TrimSpace(value))
		}
	}
	return strings.ToLower(strings.TrimSpace(s.cfg.WelcomeTokenContract))
}

func (s Server) requireWalletConfig(ctx context.Context) (string, error) {
	tokenContract := s.activeTokenContract(ctx)
	if s.cfg.WelcomeTokenChainID <= 0 {
		return "", errors.New("wallet chain is not configured")
	}
	if strings.TrimSpace(tokenContract) == "" {
		return "", errors.New("token contract is not configured")
	}
	if !walletops.IsEVMAddress(tokenContract) {
		return "", errors.New("token contract is invalid")
	}
	if s.cfg.WelcomeTokenDecimals < 0 {
		return "", errors.New("token decimals are invalid")
	}
	if s.gmrEngine == nil || !s.gmrEngine.Configured() {
		return "", errors.New("GMR Engine is not configured")
	}
	if _, err := s.projectWalletAddress(ctx); err != nil {
		return "", err
	}
	return tokenContract, nil
}

func (s Server) sendWalletTokens(c *fiber.Ctx, inputs []walletRecipientInput) (walletops.SendTokenResult, string, error) {
	return s.sendWalletTokensWithRelease(c, inputs, 0)
}

func (s Server) sendWalletTokensWithRelease(c *fiber.Ctx, inputs []walletRecipientInput, liabilityRelease float64) (walletops.SendTokenResult, string, error) {
	s.collateralMu.Lock()
	defer s.collateralMu.Unlock()
	return s.sendWalletTokensLocked(c, inputs, liabilityRelease)
}

func (s Server) sendWalletTokensLocked(c *fiber.Ctx, inputs []walletRecipientInput, liabilityRelease float64) (walletops.SendTokenResult, string, error) {
	tokenContract, err := s.requireWalletConfig(c.Context())
	if err != nil {
		return walletops.SendTokenResult{}, "", fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if len(inputs) == 0 {
		return walletops.SendTokenResult{}, "", fiber.NewError(fiber.StatusBadRequest, "add at least one recipient")
	}

	transactionIDs := []string{}
	totalQuantity := ""
	totalOutgoing := 0.0
	for _, input := range inputs {
		address := strings.TrimSpace(input.Address)
		if !walletops.IsEVMAddress(address) {
			return walletops.SendTokenResult{}, "", fiber.NewError(fiber.StatusBadRequest, "invalid recipient address")
		}
		quantity, err := walletops.TokenQuantity(input.Amount, s.cfg.WelcomeTokenDecimals)
		if err != nil {
			return walletops.SendTokenResult{}, "", fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		if totalQuantity == "" {
			totalQuantity = quantity
		}
		amount, err := strconv.ParseFloat(strings.TrimSpace(input.Amount), 64)
		if err != nil || amount <= 0 {
			return walletops.SendTokenResult{}, "", fiber.NewError(fiber.StatusBadRequest, "invalid token amount")
		}
		totalOutgoing += amount
	}
	if err := s.ensureCollateralOutflow(c.Context(), totalOutgoing, liabilityRelease); err != nil {
		if errors.Is(err, errUnderCollateralized) {
			return walletops.SendTokenResult{}, "", fiber.NewError(fiber.StatusConflict, "transfer rejected: funds are reserved to guarantee market payouts")
		}
		return walletops.SendTokenResult{}, "", fiber.NewError(fiber.StatusBadGateway, "failed to verify payout collateral")
	}

	for _, input := range inputs {
		address := strings.TrimSpace(input.Address)
		result, err := s.gmrEngine.TransferERC20(c.Context(), gmrengine.TransferRequest{
			Amount:          input.Amount,
			ChainID:         s.cfg.WelcomeTokenChainID,
			ContractAddress: tokenContract,
			Decimals:        s.cfg.WelcomeTokenDecimals,
			Recipient:       address,
		})
		if err != nil {
			return walletops.SendTokenResult{}, "", fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		transactionIDs = append(transactionIDs, result.TransactionIDs...)
	}
	return walletops.SendTokenResult{TransactionIDs: transactionIDs}, totalQuantity, nil
}

func (s Server) sendSettlementPayoutsLocked(c *fiber.Ctx, preview store.SettlementPreview) (walletops.SendTokenResult, error) {
	if len(preview.Recipients) == 0 || preview.PayoutRequired <= 0 {
		return walletops.SendTokenResult{}, nil
	}
	inputs := make([]walletRecipientInput, 0, len(preview.Recipients))
	for _, recipient := range preview.Recipients {
		inputs = append(inputs, walletRecipientInput{
			Address: recipient.WalletAddress,
			Amount:  settlementAmountString(recipient.Amount),
		})
	}
	result, _, err := s.sendWalletTokensLocked(c, inputs, preview.PayoutRequired)
	return result, err
}

func settlementAmountString(amount float64) string {
	return strconv.FormatFloat(amount, 'f', 2, 64)
}

func networkName(chainID int) string {
	switch chainID {
	case 2651420:
		return "Horizen Testnet"
	case 26514:
		return "Horizen"
	case 421614:
		return "Arbitrum Sepolia"
	case 42161:
		return "Arbitrum One"
	case 11155111:
		return "Ethereum Sepolia"
	case 1:
		return "Ethereum"
	default:
		return "Chain " + strconv.Itoa(chainID)
	}
}

func (s Server) projectWalletTokenBalance(c *fiber.Ctx) fiber.Map {
	tokenContract := s.activeTokenContract(c.Context())
	walletAddress, err := s.projectWalletAddress(c.Context())
	if err != nil {
		return fiber.Map{
			"error": err.Error(),
		}
	}

	balance, err := s.evm.ERC20Balance(c.Context(), tokenContract, walletAddress, s.cfg.WelcomeTokenDecimals)
	if err != nil {
		return fiber.Map{
			"error": err.Error(),
		}
	}

	return fiber.Map{
		"raw":           balance.Raw,
		"formatted":     balance.Formatted,
		"decimals":      balance.Decimals,
		"walletAddress": balance.WalletAddress,
		"tokenAddress":  balance.TokenAddress,
		"fetchedAt":     balance.FetchedAt,
	}
}

func (s Server) welcomeGrantOverview(c *fiber.Ctx) (fiber.Map, error) {
	grants, err := s.store.ListTokenGrants(c.Context(), "welcome", []string{}, 250)
	if err != nil {
		return nil, err
	}
	counts := fiber.Map{
		"pending": 0,
		"failed":  0,
		"sent":    0,
		"total":   len(grants),
	}
	for _, grant := range grants {
		switch grant.Status {
		case "pending":
			counts["pending"] = counts["pending"].(int) + 1
		case "failed":
			counts["failed"] = counts["failed"].(int) + 1
		case "sent":
			counts["sent"] = counts["sent"].(int) + 1
		}
	}
	wallet := s.welcomeGrantWalletHealth(c)
	return fiber.Map{
		"counts": counts,
		"grants": grants,
		"wallet": wallet,
	}, nil
}

func (s Server) welcomeGrantWalletHealth(c *fiber.Ctx) fiber.Map {
	tokenContract := s.activeTokenContract(c.Context())
	quantity := ""
	if s.cfg.WelcomeTokenAmount != "" {
		if parsed, err := walletops.TokenQuantity(s.cfg.WelcomeTokenAmount, s.cfg.WelcomeTokenDecimals); err == nil {
			quantity = parsed
		}
	}
	health := fiber.Map{
		"address":       "",
		"ready":         false,
		"status":        "missing",
		"message":       "GMR Engine default project wallet is not configured",
		"nativeBalance": nil,
		"tokenBalance":  nil,
	}
	if s.gmrEngine == nil || !s.gmrEngine.Configured() {
		health["message"] = "GMR Engine is not configured"
		return health
	}
	projectWallet, ok, err := s.gmrEngine.DefaultAdminWallet(c.Context())
	if err != nil {
		health["message"] = err.Error()
		return health
	}
	if !ok {
		return health
	}
	health["address"] = projectWallet.Address
	health["status"] = projectWallet.Status

	nativeBalance, err := s.evm.NativeBalance(c.Context(), projectWallet.Address)
	if err != nil {
		health["message"] = err.Error()
		return health
	}
	health["nativeBalance"] = nativeBalance
	if isZeroDecimalString(nativeBalance.Raw) {
		health["message"] = "Project wallet needs " + networkName(s.cfg.WelcomeTokenChainID) + " ETH for gas"
		return health
	}
	tokenBalance, err := s.evm.ERC20Balance(c.Context(), tokenContract, projectWallet.Address, s.cfg.WelcomeTokenDecimals)
	if err != nil {
		health["message"] = err.Error()
		return health
	}
	health["tokenBalance"] = tokenBalance
	if quantity != "" && !hasAtLeastRawTokenBalance(tokenBalance.Raw, quantity) {
		health["message"] = "Project wallet needs BUDOL tokens"
		return health
	}
	health["ready"] = true
	health["message"] = "Project wallet is ready to send welcome grants"
	return health
}

func (s Server) waitForWalletOpsTransactionStatus(c *fiber.Ctx, transactionIDs []string) (string, string) {
	if len(transactionIDs) == 0 {
		return "submitted", ""
	}

	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			time.Sleep(750 * time.Millisecond)
		}
		if isBytes32Hex(transactionIDs[0]) {
			status, err := s.evm.TransactionReceiptStatus(c.Context(), transactionIDs[0])
			if err != nil {
				return "submitted", err.Error()
			}
			if status.Failed {
				return "failed", "on-chain transaction reverted"
			}
			if status.Confirmed {
				return "sent", ""
			}
			continue
		}
		status, err := s.walletops.TransactionStatus(c.Context(), transactionIDs[0])
		if err != nil {
			return "submitted", err.Error()
		}
		switch status.Status {
		case "FAILED", "CANCELLED":
			if status.ErrorMessage == "" {
				status.ErrorMessage = "walletops transaction failed"
			}
			return "failed", status.ErrorMessage
		case "CONFIRMED", "MINED":
			return "sent", ""
		case "SUBMITTED", "QUEUED", "PENDING", "WAITING":
			continue
		}
	}
	return "submitted", ""
}

func (s Server) walletopsTransactionStatus(c *fiber.Ctx, transactionID string) (string, string, string) {
	if isBytes32Hex(transactionID) {
		status, err := s.evm.TransactionReceiptStatus(c.Context(), transactionID)
		if err != nil {
			return "submitted", err.Error(), transactionID
		}
		if status.Failed {
			return "failed", "on-chain transaction reverted", transactionID
		}
		if status.Confirmed {
			return "sent", "", transactionID
		}
		return "submitted", "", transactionID
	}
	status, err := s.walletops.TransactionStatus(c.Context(), transactionID)
	if err != nil {
		return "submitted", err.Error(), ""
	}
	switch status.Status {
	case "FAILED", "CANCELLED":
		errorMessage := status.ErrorMessage
		if errorMessage == "" {
			errorMessage = "walletops transaction failed"
		}
		return "failed", errorMessage, status.TransactionHash
	case "CONFIRMED", "MINED":
		return "sent", "", status.TransactionHash
	case "SUBMITTED", "QUEUED", "PENDING", "WAITING":
		return "submitted", status.ErrorMessage, status.TransactionHash
	default:
		normalized := strings.ToLower(strings.TrimSpace(status.Status))
		if normalized == "" {
			normalized = "submitted"
		}
		return normalized, status.ErrorMessage, status.TransactionHash
	}
}

func shouldRetryTokenGrant(c *fiber.Ctx, client *walletops.Client, grant store.TokenGrant) bool {
	switch grant.Status {
	case "sent":
		return false
	case "failed":
		return true
	case "submitted", "pending":
		if len(grant.TransactionIDs) == 0 {
			updatedAt, err := time.Parse(time.RFC3339, grant.UpdatedAt)
			return err != nil || time.Since(updatedAt) >= 2*time.Minute
		}
		status, err := client.TransactionStatus(c.Context(), grant.TransactionIDs[0])
		if err != nil {
			return false
		}
		switch status.Status {
		case "FAILED", "CANCELLED":
			return true
		case "CONFIRMED", "MINED":
			return false
		default:
			return false
		}
	default:
		return false
	}
}

func walletActionResponse(action string, recipientCount int, quantity string, result walletops.SendTokenResult) fiber.Map {
	return fiber.Map{
		"action":         action,
		"ok":             true,
		"quantity":       quantity,
		"recipientCount": recipientCount,
		"transactionIds": result.TransactionIDs,
	}
}

func walletTransferLink(transactionHash string) string {
	transactionHash = strings.TrimSpace(transactionHash)
	if transactionHash == "" {
		return "/wallet"
	}
	return "https://sepolia.arbiscan.io/tx/" + transactionHash
}

func walletTransferNotificationLink(link string) string {
	link = strings.TrimSpace(link)
	if link == "" {
		return ""
	}
	return link
}

func errorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	message := "internal server error"

	if fiberError, ok := err.(*fiber.Error); ok {
		code = fiberError.Code
		message = fiberError.Message
	}

	return c.Status(code).JSON(fiber.Map{
		"error": message,
	})
}

func applyAllowedOriginHeaders(c *fiber.Ctx, allowedOrigins []string) {
	origin := strings.TrimSpace(c.Get("Origin"))
	if origin == "" {
		return
	}
	for _, allowed := range allowedOrigins {
		allowed = strings.TrimSpace(allowed)
		if allowed == "*" || allowed == origin {
			c.Set(fiber.HeaderAccessControlAllowOrigin, origin)
			c.Set(fiber.HeaderAccessControlAllowCredentials, "true")
			c.Vary(fiber.HeaderOrigin)
			return
		}
	}
}

func (s Server) requireAdminSession(c *fiber.Ctx) error {
	if _, err := s.verifyAdminCookie(c); err == nil {
		return c.Next()
	}
	if s.validAdminAPIKey(c.Get("X-Budol-Admin-Key")) {
		return c.Next()
	}
	return fiber.NewError(fiber.StatusUnauthorized, "admin login required")
}

func (s Server) validAdminCredentials(username string, password string) bool {
	usernameMatch := subtle.ConstantTimeCompare([]byte(username), []byte(s.cfg.AdminUsername)) == 1
	passwordMatch := subtle.ConstantTimeCompare([]byte(password), []byte(s.cfg.AdminPassword)) == 1
	return usernameMatch && passwordMatch
}

func (s Server) validAdminAPIKey(value string) bool {
	if value == "" || s.cfg.AdminAPIKey == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(value), []byte(s.cfg.AdminAPIKey)) == 1
}

func (s Server) verifyAdminCookie(c *fiber.Ctx) (session.Claims, error) {
	token := c.Cookies(s.cfg.AdminCookieName)
	if token == "" {
		return session.Claims{}, fiber.NewError(fiber.StatusUnauthorized, "not logged in")
	}
	claims, err := s.sessions.Verify(token)
	if err != nil {
		return session.Claims{}, err
	}
	if !strings.HasPrefix(claims.Subject, "admin:") {
		return session.Claims{}, fiber.NewError(fiber.StatusUnauthorized, "invalid admin session")
	}
	return claims, nil
}

func (s Server) setAdminCookie(c *fiber.Ctx, value string, expires time.Time) {
	c.Cookie(&fiber.Cookie{
		Name:     s.cfg.AdminCookieName,
		Value:    value,
		Expires:  expires,
		HTTPOnly: true,
		SameSite: fiber.CookieSameSiteLaxMode,
		Secure:   s.cfg.IsProduction(),
		Path:     "/",
	})
}

func (s Server) adminActor(c *fiber.Ctx) string {
	claims, err := s.verifyAdminCookie(c)
	if err != nil {
		return s.cfg.AdminUsername
	}
	return strings.TrimPrefix(claims.Subject, "admin:")
}

func (s Server) notifySettlementRecipients(c *fiber.Ctx, settlement store.SettlementPreview, payoutStatus string) {
	for _, recipient := range settlement.Recipients {
		if recipient.UserID == "" || recipient.Amount <= 0 {
			continue
		}
		title := "Payout received"
		detail := settlement.PollTitle + ": " + settlementAmountString(recipient.Amount) + " BUDOL payout status is " + payoutStatus + "."
		if settlement.Status == "cancelled" {
			title = "Market refunded"
			detail = settlement.PollTitle + ": " + settlementAmountString(recipient.Amount) + " BUDOL was returned."
		}
		_, _ = s.store.CreateNotification(c.Context(), recipient.UserID, "settlement", title, detail, "/portfolio")
	}
}

func shortAddressForAdmin(address string) string {
	address = strings.TrimSpace(address)
	if len(address) <= 12 {
		return address
	}
	return address[:6] + "..." + address[len(address)-4:]
}

func adminPollDetail(poll store.Poll, reason string) string {
	detail := poll.Title + " is " + poll.Status + " / " + poll.Visibility + "."
	if poll.TradingFrozen {
		detail += " Trading frozen."
	}
	if poll.CommentsDisabled {
		detail += " Comments disabled."
	}
	reason = strings.TrimSpace(reason)
	if reason != "" {
		detail += " Reason: " + reason
	}
	return detail
}

func slugifyForPath(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(" ", "-", "_", "-", "/", "-", "'", "", "\"", "", "?", "", "!", "", ".", "", ",", "", ":", "", ";", "", "(", "", ")", "")
	value = replacer.Replace(value)
	for strings.Contains(value, "--") {
		value = strings.ReplaceAll(value, "--", "-")
	}
	return strings.Trim(value, "-")
}

func normalizedChoiceLabels(values []string) []string {
	choices := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		choice := strings.TrimSpace(value)
		key := strings.ToLower(choice)
		if choice == "" || seen[key] {
			continue
		}
		seen[key] = true
		choices = append(choices, choice)
	}
	return choices
}

func parseOptionalFloat(value string) float64 {
	if strings.TrimSpace(value) == "" {
		return 0
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}
	return parsed
}

func matchesConfirmation(value string, expected string) bool {
	return subtle.ConstantTimeCompare([]byte(strings.TrimSpace(value)), []byte(expected)) == 1
}

func adminActivity(c *fiber.Ctx, actor string, action string, targetType string, targetID string, title string, detail string) store.AdminActivityInput {
	return store.AdminActivityInput{
		Actor:      actor,
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		Title:      title,
		Detail:     detail,
		IPAddress:  c.IP(),
		UserAgent:  c.Get("User-Agent"),
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func cacheControlHeaders(c *fiber.Ctx) error {
	if strings.HasPrefix(c.Path(), "/api/") {
		c.Set("Cache-Control", "no-store")
		c.Set("Pragma", "no-cache")
	}
	return c.Next()
}

func (s Server) requireAdminKey(c *fiber.Ctx) error {
	if !s.validAdminAPIKey(c.Get("X-Budol-Admin-Key")) {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid admin key")
	}
	return c.Next()
}
