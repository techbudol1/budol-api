package httpapi

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	"budol/server/internal/config"
	"budol/server/internal/evm"
	"budol/server/internal/session"
	"budol/server/internal/store"
	"budol/server/internal/thirdweb"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/helmet"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

type Server struct {
	cfg      config.Config
	evm      *evm.Client
	store    store.AdminStore
	thirdweb *thirdweb.Client
	sessions session.Manager
}

type ThirdwebLoginRequest struct {
	AuthToken  string          `json:"authToken"`
	AuthResult json.RawMessage `json:"authResult"`
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

type TradeRequest struct {
	PollID       string  `json:"pollId"`
	Side         string  `json:"side"`
	Amount       float64 `json:"amount"`
	EscrowTxHash string  `json:"escrowTxHash"`
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
const transferConfirmText = "CONFIRM TRANSFER"
const airdropConfirmText = "CONFIRM AIRDROP"
const burnConfirmText = "CONFIRM BURN"

func New(cfg config.Config, userStore store.AdminStore, thirdwebClient *thirdweb.Client, sessions session.Manager) *fiber.App {
	server := Server{
		cfg:      cfg,
		evm:      evm.NewClient(cfg.ArbitrumSepoliaRPCURL),
		store:    userStore,
		thirdweb: thirdwebClient,
		sessions: sessions,
	}

	app := fiber.New(fiber.Config{
		AppName:      "Budol API",
		ErrorHandler: errorHandler,
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
	api.Get("/auth/social/:provider", server.startSocialAuth)
	api.Get("/auth/social/callback", server.socialAuthCallback)
	api.Post("/auth/thirdweb", authRateLimit, server.loginWithThirdweb)
	api.Get("/auth/me", server.me)
	api.Post("/auth/logout", server.logout)
	api.Get("/polls", server.publicPolls)
	api.Get("/polls/:slug/activity", server.publicPollActivity)
	api.Get("/polls/:slug/stats", server.publicPollStats)
	api.Get("/polls/:slug/comments", server.publicPollComments)
	api.Post("/polls/:slug/comments", server.createPollComment)
	api.Post("/comments/:id/report", server.reportPollComment)
	api.Get("/polls/:slug", server.publicPollDetail)
	api.Get("/portfolio", server.portfolio)
	api.Get("/trade-quote", server.tradeQuote)
	api.Post("/trades", tradeRateLimit, server.createTrade)
	api.Get("/cashout-quote", server.cashoutQuote)
	api.Post("/cashouts", server.cashoutPosition)
	api.Get("/notifications", server.notifications)
	api.Post("/notifications/read-all", server.markAllNotificationsRead)
	api.Post("/notifications/:id/read", server.markNotificationRead)
	api.Get("/watchlist", server.watchlist)
	api.Post("/watchlist", server.addWatchlist)
	api.Delete("/watchlist/:slug", server.removeWatchlist)
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
	admin.Get("/polls/:id/settlement-preview", server.adminSettlementPreview)
	admin.Get("/polls/:id/settlement-detail", server.adminSettlementDetail)
	admin.Post("/polls/:id/resolution", adminMutationRateLimit, server.adminResolvePoll)
	admin.Post("/polls/:id/payout-retry", adminMutationRateLimit, server.adminRetrySettlementPayout)
	admin.Post("/polls/:id/payout-reconcile", adminMutationRateLimit, server.adminReconcileSettlementPayout)
	admin.Get("/trades", server.adminTrades)
	admin.Post("/trades/:id/escrow-reconcile", adminMutationRateLimit, server.adminReconcileTradeEscrow)
	admin.Get("/activity", server.adminActivity)
	admin.Get("/comments", server.adminComments)
	admin.Patch("/comments/:id", server.adminModerateComment)
	admin.Get("/wallet", server.adminWalletConfig)
	admin.Post("/wallet/transfer", adminMutationRateLimit, server.adminWalletTransfer)
	admin.Post("/wallet/airdrop", adminMutationRateLimit, server.adminWalletAirdrop)
	admin.Post("/wallet/burn", adminMutationRateLimit, server.adminWalletBurn)

	return app
}

func (s Server) loginWithThirdweb(c *fiber.Ctx) error {
	var request ThirdwebLoginRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}

	authToken, err := authTokenFromRequest(request)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	user, err := s.verifyAndLogin(c, authToken)
	if err != nil {
		return err
	}

	return c.JSON(fiber.Map{
		"user": user,
	})
}

func (s Server) startSocialAuth(c *fiber.Ctx) error {
	provider := c.Params("provider")
	if !validSocialProvider(provider) {
		return fiber.NewError(fiber.StatusBadRequest, "unsupported social auth provider")
	}

	redirectURL := s.callbackURL()
	target := strings.TrimRight(s.cfg.ThirdwebAPIBase, "/") + "/v1/auth/social?provider=" + url.QueryEscape(provider) + "&redirectUrl=" + url.QueryEscape(redirectURL)
	return c.Redirect(target, fiber.StatusFound)
}

func (s Server) socialAuthCallback(c *fiber.Ctx) error {
	authResult := c.Query("authResult")
	if authResult == "" {
		return fiber.NewError(fiber.StatusBadRequest, "missing thirdweb authResult")
	}

	authToken, err := authTokenFromAuthResult(json.RawMessage(authResult))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	if _, err := s.verifyAndLogin(c, authToken); err != nil {
		return err
	}

	return c.Redirect(s.cfg.PublicAppURL, fiber.StatusFound)
}

func (s Server) verifyAndLogin(c *fiber.Ctx, authToken string) (store.User, error) {
	identity, err := s.thirdweb.VerifyAuthToken(c.Context(), authToken)
	if err != nil {
		return store.User{}, fiber.NewError(fiber.StatusUnauthorized, "invalid thirdweb login")
	}

	user, err := s.store.UpsertFromThirdweb(c.Context(), identity)
	if err != nil {
		return store.User{}, fiber.NewError(fiber.StatusInternalServerError, "failed to save user")
	}
	s.grantWelcomeTokens(c, user)
	_, _ = s.store.CreateNotification(c.Context(), user.ID, "login", "Login successful", "Your Budol session is active on this browser.", "/account")

	token, err := s.sessions.Issue(user)
	if err != nil {
		return store.User{}, fiber.NewError(fiber.StatusInternalServerError, "failed to issue session")
	}

	c.Cookie(&fiber.Cookie{
		Name:     s.cfg.SessionCookieName,
		Value:    token,
		Expires:  time.Now().UTC().Add(s.sessions.TTL()),
		HTTPOnly: true,
		SameSite: fiber.CookieSameSiteLaxMode,
		Secure:   s.cfg.IsProduction(),
		Path:     "/",
	})

	return user, nil
}

func (s Server) me(c *fiber.Ctx) error {
	user, err := s.authenticatedUser(c)
	if err != nil {
		return err
	}

	return c.JSON(fiber.Map{
		"user": user,
	})
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
	if s.cfg.WelcomeTokenContract == "" || s.cfg.WelcomeTokenAmount == "" {
		return
	}

	quantity, err := thirdweb.TokenQuantity(s.cfg.WelcomeTokenAmount, s.cfg.WelcomeTokenDecimals)
	if err != nil {
		return
	}

	grant, created, err := s.store.CreateTokenGrantIfMissing(c.Context(), user, store.TokenGrantInput{
		GrantType:    "welcome",
		TokenAddress: s.cfg.WelcomeTokenContract,
		ChainID:      int64(s.cfg.WelcomeTokenChainID),
		Amount:       s.cfg.WelcomeTokenAmount,
		Quantity:     quantity,
	})
	if err != nil {
		return
	}
	if !created && !shouldRetryTokenGrant(c, s.thirdweb, grant) {
		return
	}

	result, err := s.thirdweb.SendToken(c.Context(), thirdweb.SendTokenRequest{
		ChainID:      s.cfg.WelcomeTokenChainID,
		From:         s.cfg.ProjectWallet,
		Recipient:    user.WalletAddress,
		TokenAddress: s.cfg.WelcomeTokenContract,
		Quantity:     quantity,
	})
	if err != nil {
		_, _ = s.store.UpdateTokenGrantStatus(c.Context(), grant.ID, "failed", nil, err.Error())
		_, _ = s.store.CreateNotification(c.Context(), user.ID, "welcome_tokens", "Welcome token grant failed", "Budol could not send the welcome tokens yet. Try again later or contact support.", "/wallet")
		return
	}

	status, errorMessage := s.waitForThirdwebTransactionStatus(c, result.TransactionIDs)
	_, _ = s.store.UpdateTokenGrantStatus(c.Context(), grant.ID, status, result.TransactionIDs, errorMessage)
	if status == "confirmed" || status == "submitted" {
		_, _ = s.store.CreateNotification(c.Context(), user.ID, "welcome_tokens", "Welcome tokens received", s.cfg.WelcomeTokenAmount+" BUDOL welcome tokens were sent to your wallet.", "/wallet")
	}
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
	polls, err := s.store.ListPolls(c.Context(), true)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load polls")
	}
	return c.JSON(fiber.Map{"polls": polls})
}

func (s Server) publicPollDetail(c *fiber.Ctx) error {
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
	activities, err := s.store.MarketActivity(c.Context(), c.Params("slug"))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load market activity")
	}
	return c.JSON(fiber.Map{"activity": activities})
}

func (s Server) publicPollStats(c *fiber.Ctx) error {
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
	_, _ = s.store.CreateNotification(c.Context(), user.ID, "comment_report", "Comment reported", "Thanks. Budol admins will review that Marites note.", "/markets/"+comment.PollSlug)
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
	escrow, err := s.verifyTradeEscrow(c, user, request)
	if err != nil {
		return err
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
	})
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	_, _ = s.store.CreateNotification(c.Context(), user.ID, "trade", "Trade placed", trade.PollTitle+": bought "+trade.OutcomeLabel+" for "+settlementAmountString(trade.Amount)+" BUDOL.", "/markets/"+trade.PollSlug)
	watchDetail := trade.PollTitle + ": " + trade.OutcomeLabel + " traded at " + strconv.FormatInt(trade.PriceCents, 10) + "c."
	_, _ = s.store.NotifyWatchers(c.Context(), trade.PollSlug, user.ID, "watchlist_trade", "Watched market moved", watchDetail, "/markets/"+trade.PollSlug)

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

func (s Server) tradeQuote(c *fiber.Ctx) error {
	amount, err := strconv.ParseFloat(c.Query("amount"), 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid amount")
	}
	quote, err := s.store.QuoteTrade(c.Context(), store.TradeInput{
		PollID: c.Query("pollId"),
		Side:   c.Query("side"),
		Amount: amount,
	})
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	return c.JSON(fiber.Map{"quote": quote})
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

	quote, err := s.store.QuoteCashout(c.Context(), user.ID, store.CashoutInput{
		PollID: request.PollID,
		Side:   request.Side,
		Amount: request.Amount,
	})
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	result, _, err := s.sendWalletTokens(c, []walletRecipientInput{
		{Address: user.WalletAddress, Amount: settlementAmountString(quote.Proceeds)},
	})
	if err != nil {
		return err
	}
	payoutStatus, payoutError := s.waitForThirdwebTransactionStatus(c, result.TransactionIDs)

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
	if strings.TrimSpace(s.cfg.ProjectWallet) == "" {
		return tradeEscrowVerification{}, fiber.NewError(fiber.StatusBadRequest, "project vault wallet is not configured")
	}
	if !thirdweb.IsEVMAddress(s.cfg.ProjectWallet) {
		return tradeEscrowVerification{}, fiber.NewError(fiber.StatusBadRequest, "project vault wallet is invalid")
	}
	txHash := strings.TrimSpace(request.EscrowTxHash)
	if txHash == "" {
		return tradeEscrowVerification{}, fiber.NewError(fiber.StatusBadRequest, "escrow transaction hash is required")
	}
	quantity, err := thirdweb.TokenQuantity(settlementAmountString(request.Amount), s.cfg.WelcomeTokenDecimals)
	if err != nil {
		return tradeEscrowVerification{}, fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	ok, err := s.evm.WaitForERC20Transfer(c.Context(), s.cfg.WelcomeTokenContract, txHash, user.WalletAddress, s.cfg.ProjectWallet, quantity)
	if err != nil {
		return tradeEscrowVerification{}, fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	if !ok {
		return tradeEscrowVerification{}, fiber.NewError(fiber.StatusBadRequest, "escrow transfer was not found or is still pending")
	}
	return tradeEscrowVerification{
		Status:     "verified",
		From:       strings.ToLower(user.WalletAddress),
		To:         strings.ToLower(s.cfg.ProjectWallet),
		Amount:     request.Amount,
		VerifiedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func (s Server) ensureTradingBalance(c *fiber.Ctx, user store.User, amount float64) error {
	if amount <= 0 {
		return fiber.NewError(fiber.StatusBadRequest, "amount must be greater than zero")
	}
	balance, err := s.evm.ERC20Balance(c.Context(), s.cfg.WelcomeTokenContract, user.WalletAddress, s.cfg.WelcomeTokenDecimals)
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, "failed to verify token balance")
	}
	portfolio, err := s.store.UserPortfolio(c.Context(), user.ID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to verify open exposure")
	}
	tokenBalance, err := strconv.ParseFloat(balance.Formatted, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, "failed to parse token balance")
	}
	available := tokenBalance - portfolio.Summary.TotalExposure
	if amount > available {
		return fiber.NewError(fiber.StatusBadRequest, "order exceeds available BUDOL after open exposure")
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
		_, _ = s.store.CreateNotification(c.Context(), user.ID, "watchlist", "Added to watchlist", "You will get alerts when this market moves.", "/markets/"+slugifyForPath(request.Slug))
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

func (s Server) walletHistory(c *fiber.Ctx) error {
	user, err := s.authenticatedUser(c)
	if err != nil {
		return err
	}
	transfers, err := s.evm.ERC20TransferHistory(c.Context(), s.cfg.WelcomeTokenContract, user.WalletAddress, s.cfg.WelcomeTokenDecimals)
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, "failed to load wallet history")
	}
	return c.JSON(fiber.Map{
		"transfers": transfers,
	})
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
	quantity, err := thirdweb.TokenQuantity(settlementAmountString(amount), s.cfg.WelcomeTokenDecimals)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	found, pending, err := s.evm.ERC20TransferInTransaction(c.Context(), s.cfg.WelcomeTokenContract, trade.EscrowTxHash, from, to, quantity)
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

func (s Server) adminUpdatePoll(c *fiber.Ctx) error {
	var input store.PollInput
	if err := c.BodyParser(&input); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
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
			result, err := s.sendSettlementPayouts(c, preview)
			if err != nil {
				return err
			}
			transactionIDs = result.TransactionIDs
			payoutStatus, payoutError = s.waitForThirdwebTransactionStatus(c, transactionIDs)
		}

		poll, err := s.store.UpdatePollResolution(c.Context(), c.Params("id"), request.Status, request.Outcome, s.adminActor(c), request.ResolutionSource, request.ResolutionEvidenceURL, request.ResolutionNotes)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		settlement, err := s.store.SettlePollTrades(c.Context(), c.Params("id"), request.Status, request.Outcome, transactionIDs, payoutStatus, payoutError)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to record settlement")
		}
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
		_, _ = s.store.NotifyWatchers(c.Context(), poll.Slug, "", "watchlist_resolution", "Watched market "+request.Status, poll.Title+" has been "+request.Status+".", "/markets/"+poll.Slug)
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

func (s Server) adminRetrySettlementPayout(c *fiber.Ctx) error {
	var request PayoutRetryRequest
	if err := c.BodyParser(&request); err != nil && len(c.Body()) > 0 {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	if !matchesConfirmation(request.ConfirmText, retryPayoutConfirmText) {
		return fiber.NewError(fiber.StatusBadRequest, "type "+retryPayoutConfirmText+" to retry settlement payout")
	}
	preview, err := s.store.SettlementPayoutRetryPreview(c.Context(), c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if preview.PayoutRequired <= 0 || len(preview.Recipients) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "no payable settlement recipients to retry")
	}

	result, err := s.sendSettlementPayouts(c, preview)
	if err != nil {
		return err
	}
	payoutStatus, payoutError := s.waitForThirdwebTransactionStatus(c, result.TransactionIDs)
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

	payoutStatus, payoutError, transactionHash := s.thirdwebTransactionStatus(c, transactionIDs[0])
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
	return c.JSON(fiber.Map{
		"settlement": preview,
		"wallet": fiber.Map{
			"chainId":              s.cfg.WelcomeTokenChainID,
			"chainName":            "Arbitrum Sepolia",
			"tokenAddress":         s.cfg.WelcomeTokenContract,
			"projectWallet":        s.cfg.ProjectWallet,
			"projectWalletBalance": balance,
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
	if err := s.requireWalletConfig(); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	balance := s.projectWalletTokenBalance(c)
	return c.JSON(fiber.Map{
		"wallet": fiber.Map{
			"chainId":              s.cfg.WelcomeTokenChainID,
			"chainName":            "Arbitrum Sepolia",
			"tokenAddress":         s.cfg.WelcomeTokenContract,
			"tokenDecimals":        s.cfg.WelcomeTokenDecimals,
			"projectWallet":        s.cfg.ProjectWallet,
			"projectWalletBalance": balance,
			"burnAddress":          deadBurnAddress,
		},
	})
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

func (s Server) requireWalletConfig() error {
	if s.cfg.WelcomeTokenChainID <= 0 {
		return errors.New("wallet chain is not configured")
	}
	if strings.TrimSpace(s.cfg.WelcomeTokenContract) == "" {
		return errors.New("token contract is not configured")
	}
	if !thirdweb.IsEVMAddress(s.cfg.WelcomeTokenContract) {
		return errors.New("token contract is invalid")
	}
	if s.cfg.ProjectWallet != "" && !thirdweb.IsEVMAddress(s.cfg.ProjectWallet) {
		return errors.New("project wallet address is invalid")
	}
	if s.cfg.WelcomeTokenDecimals < 0 {
		return errors.New("token decimals are invalid")
	}
	if strings.TrimSpace(s.cfg.ThirdwebSendURL) == "" {
		return errors.New("thirdweb send URL is not configured")
	}
	return nil
}

func (s Server) sendWalletTokens(c *fiber.Ctx, inputs []walletRecipientInput) (thirdweb.SendTokenResult, string, error) {
	if err := s.requireWalletConfig(); err != nil {
		return thirdweb.SendTokenResult{}, "", fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if len(inputs) == 0 {
		return thirdweb.SendTokenResult{}, "", fiber.NewError(fiber.StatusBadRequest, "add at least one recipient")
	}

	recipients := make([]thirdweb.TokenRecipient, 0, len(inputs))
	totalQuantity := ""
	for _, input := range inputs {
		address := strings.TrimSpace(input.Address)
		if !thirdweb.IsEVMAddress(address) {
			return thirdweb.SendTokenResult{}, "", fiber.NewError(fiber.StatusBadRequest, "invalid recipient address")
		}
		quantity, err := thirdweb.TokenQuantity(input.Amount, s.cfg.WelcomeTokenDecimals)
		if err != nil {
			return thirdweb.SendTokenResult{}, "", fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		if totalQuantity == "" {
			totalQuantity = quantity
		}
		recipients = append(recipients, thirdweb.TokenRecipient{
			Address:  address,
			Quantity: quantity,
		})
	}

	result, err := s.thirdweb.SendToken(c.Context(), thirdweb.SendTokenRequest{
		ChainID:      s.cfg.WelcomeTokenChainID,
		From:         s.cfg.ProjectWallet,
		TokenAddress: s.cfg.WelcomeTokenContract,
		Recipients:   recipients,
	})
	if err != nil {
		return thirdweb.SendTokenResult{}, "", fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	return result, totalQuantity, nil
}

func (s Server) sendSettlementPayouts(c *fiber.Ctx, preview store.SettlementPreview) (thirdweb.SendTokenResult, error) {
	if len(preview.Recipients) == 0 || preview.PayoutRequired <= 0 {
		return thirdweb.SendTokenResult{}, nil
	}
	inputs := make([]walletRecipientInput, 0, len(preview.Recipients))
	for _, recipient := range preview.Recipients {
		inputs = append(inputs, walletRecipientInput{
			Address: recipient.WalletAddress,
			Amount:  settlementAmountString(recipient.Amount),
		})
	}
	result, _, err := s.sendWalletTokens(c, inputs)
	return result, err
}

func settlementAmountString(amount float64) string {
	return strconv.FormatFloat(amount, 'f', 2, 64)
}

func (s Server) projectWalletTokenBalance(c *fiber.Ctx) fiber.Map {
	if strings.TrimSpace(s.cfg.ProjectWallet) == "" {
		return fiber.Map{
			"error": "project wallet address is not configured",
		}
	}

	balance, err := s.evm.ERC20Balance(c.Context(), s.cfg.WelcomeTokenContract, s.cfg.ProjectWallet, s.cfg.WelcomeTokenDecimals)
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

func (s Server) waitForThirdwebTransactionStatus(c *fiber.Ctx, transactionIDs []string) (string, string) {
	if len(transactionIDs) == 0 {
		return "submitted", ""
	}

	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			time.Sleep(750 * time.Millisecond)
		}
		status, err := s.thirdweb.TransactionStatus(c.Context(), transactionIDs[0])
		if err != nil {
			return "submitted", err.Error()
		}
		switch status.Status {
		case "FAILED", "CANCELLED":
			if status.ErrorMessage == "" {
				status.ErrorMessage = "thirdweb transaction failed"
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

func (s Server) thirdwebTransactionStatus(c *fiber.Ctx, transactionID string) (string, string, string) {
	status, err := s.thirdweb.TransactionStatus(c.Context(), transactionID)
	if err != nil {
		return "submitted", err.Error(), ""
	}
	switch status.Status {
	case "FAILED", "CANCELLED":
		errorMessage := status.ErrorMessage
		if errorMessage == "" {
			errorMessage = "thirdweb transaction failed"
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

func shouldRetryTokenGrant(c *fiber.Ctx, client *thirdweb.Client, grant store.TokenGrant) bool {
	switch grant.Status {
	case "sent":
		return false
	case "failed":
		return true
	case "submitted", "pending":
		if len(grant.TransactionIDs) == 0 {
			return true
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

func walletActionResponse(action string, recipientCount int, quantity string, result thirdweb.SendTokenResult) fiber.Map {
	return fiber.Map{
		"action":         action,
		"ok":             true,
		"quantity":       quantity,
		"recipientCount": recipientCount,
		"transactionIds": result.TransactionIDs,
	}
}

func (s Server) callbackURL() string {
	base := strings.TrimRight(s.cfg.PublicAPIURL, "/")
	return base + "/api/auth/social/callback"
}

func validSocialProvider(provider string) bool {
	switch provider {
	case "google", "facebook":
		return true
	default:
		return false
	}
}

func authTokenFromRequest(request ThirdwebLoginRequest) (string, error) {
	if strings.TrimSpace(request.AuthToken) != "" {
		return request.AuthToken, nil
	}
	if len(request.AuthResult) > 0 {
		return authTokenFromAuthResult(request.AuthResult)
	}
	return "", fiber.NewError(fiber.StatusBadRequest, "missing thirdweb auth token")
}

func authTokenFromAuthResult(raw json.RawMessage) (string, error) {
	var payload struct {
		StoredToken struct {
			CookieString string `json:"cookieString"`
		} `json:"storedToken"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", fiber.NewError(fiber.StatusBadRequest, "invalid thirdweb authResult")
	}
	if strings.TrimSpace(payload.StoredToken.CookieString) == "" {
		return "", fiber.NewError(fiber.StatusBadRequest, "missing thirdweb authResult stored token")
	}
	return payload.StoredToken.CookieString, nil
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
	value = strings.ReplaceAll(value, " ", "-")
	return value
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
