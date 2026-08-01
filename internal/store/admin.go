package store

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

type PollClassification struct {
	ID          string `json:"id"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
	CallName    string `json:"callName"`
	SortOrder   int64  `json:"sortOrder"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

type Poll struct {
	ID                             string   `json:"id"`
	Slug                           string   `json:"slug"`
	Title                          string   `json:"title"`
	PollType                       string   `json:"pollType"`
	ClassificationID               string   `json:"classificationId"`
	Category                       string   `json:"category"`
	CallName                       string   `json:"callName"`
	Region                         string   `json:"region"`
	Status                         string   `json:"status"`
	Visibility                     string   `json:"visibility"`
	OutcomeA                       string   `json:"outcomeA"`
	OutcomeB                       string   `json:"outcomeB"`
	MarketGroupID                  string   `json:"marketGroupId"`
	MarketGroupTitle               string   `json:"marketGroupTitle"`
	MarketChoiceLabel              string   `json:"marketChoiceLabel"`
	MarketChoiceIndex              int64    `json:"marketChoiceIndex"`
	YesPercent                     int64    `json:"yesPercent"`
	NoPercent                      int64    `json:"noPercent"`
	Volume                         string   `json:"volume"`
	Change                         string   `json:"change"`
	Color                          string   `json:"color"`
	Hot                            bool     `json:"hot"`
	Featured                       bool     `json:"featured"`
	SortOrder                      int64    `json:"sortOrder"`
	Liquidity                      float64  `json:"liquidity"`
	YesShares                      float64  `json:"yesShares"`
	NoShares                       float64  `json:"noShares"`
	MarketMakerCollected           float64  `json:"marketMakerCollected"`
	TradingFrozen                  bool     `json:"tradingFrozen"`
	CommentsDisabled               bool     `json:"commentsDisabled"`
	AuditReason                    string   `json:"auditReason"`
	ResolutionSource               string   `json:"resolutionSource"`
	ResolutionOutcome              string   `json:"resolutionOutcome"`
	ResolutionEvidenceURL          string   `json:"resolutionEvidenceUrl"`
	ResolutionNotes                string   `json:"resolutionNotes"`
	ResolvedBy                     string   `json:"resolvedBy"`
	SettlementStatus               string   `json:"settlementStatus"`
	SettlementPayoutStatus         string   `json:"settlementPayoutStatus"`
	SettlementPayoutError          string   `json:"settlementPayoutError"`
	SettlementPayoutTransactionIDs []string `json:"settlementPayoutTransactionIds"`
	ResolvedAt                     string   `json:"resolvedAt"`
	StartsAt                       string   `json:"startsAt"`
	EndsAt                         string   `json:"endsAt"`
	CreatedAt                      string   `json:"createdAt"`
	UpdatedAt                      string   `json:"updatedAt"`
}

type PollClassificationInput struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
	CallName    string `json:"callName"`
	SortOrder   int64  `json:"sortOrder"`
}

type PollInput struct {
	Slug                  string   `json:"slug"`
	Title                 string   `json:"title"`
	PollType              string   `json:"pollType"`
	ClassificationID      string   `json:"classificationId"`
	CallName              string   `json:"callName"`
	Region                string   `json:"region"`
	Status                string   `json:"status"`
	Visibility            string   `json:"visibility"`
	OutcomeA              string   `json:"outcomeA"`
	OutcomeB              string   `json:"outcomeB"`
	ChoiceLabels          []string `json:"choiceLabels"`
	MarketGroupID         string   `json:"marketGroupId"`
	MarketGroupTitle      string   `json:"marketGroupTitle"`
	MarketChoiceLabel     string   `json:"marketChoiceLabel"`
	MarketChoiceIndex     int64    `json:"marketChoiceIndex"`
	YesPercent            int64    `json:"yesPercent"`
	Volume                string   `json:"volume"`
	Change                string   `json:"change"`
	Color                 string   `json:"color"`
	Hot                   bool     `json:"hot"`
	Featured              bool     `json:"featured"`
	SortOrder             int64    `json:"sortOrder"`
	Liquidity             float64  `json:"liquidity"`
	TradingFrozen         bool     `json:"tradingFrozen"`
	CommentsDisabled      bool     `json:"commentsDisabled"`
	AuditReason           string   `json:"auditReason"`
	ResolutionSource      string   `json:"resolutionSource"`
	ResolutionOutcome     string   `json:"resolutionOutcome"`
	ResolutionEvidenceURL string   `json:"resolutionEvidenceUrl"`
	ResolutionNotes       string   `json:"resolutionNotes"`
	StartsAt              string   `json:"startsAt"`
	EndsAt                string   `json:"endsAt"`
}

type UserUpdateInput struct {
	Role   string `json:"role"`
	Status string `json:"status"`
	Notes  string `json:"notes"`
}

type AdminUserDetail struct {
	User        User            `json:"user"`
	Portfolio   Portfolio       `json:"portfolio"`
	TokenGrants []TokenGrant    `json:"tokenGrants"`
	Comments    []MarketComment `json:"comments"`
}

type AdminSettlementDetail struct {
	Poll           Poll                      `json:"poll"`
	Settlement     SettlementPreview         `json:"settlement"`
	Trades         []Trade                   `json:"trades"`
	PayoutAttempts []SettlementPayoutAttempt `json:"payoutAttempts"`
}

type EngineApp struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Environment      string   `json:"environment"`
	Status           string   `json:"status"`
	AllowedChains    []int64  `json:"allowedChains"`
	AllowedContracts []string `json:"allowedContracts"`
	RateLimitPerMin  int64    `json:"rateLimitPerMinute"`
	CreatedAt        string   `json:"createdAt"`
	UpdatedAt        string   `json:"updatedAt"`
}

type EngineAppInput struct {
	Name             string   `json:"name"`
	Environment      string   `json:"environment"`
	AllowedChains    []int64  `json:"allowedChains"`
	AllowedContracts []string `json:"allowedContracts"`
	RateLimitPerMin  int64    `json:"rateLimitPerMinute"`
}

type EngineAPIKey struct {
	ID             string   `json:"id"`
	AppID          string   `json:"appId"`
	AppName        string   `json:"appName,omitempty"`
	Name           string   `json:"name"`
	Prefix         string   `json:"prefix"`
	Scopes         []string `json:"scopes"`
	AllowedOrigins []string `json:"allowedOrigins"`
	AllowedIPs     []string `json:"allowedIps"`
	Status         string   `json:"status"`
	ExpiresAt      string   `json:"expiresAt,omitempty"`
	LastUsedAt     string   `json:"lastUsedAt,omitempty"`
	CreatedAt      string   `json:"createdAt"`
	RevokedAt      string   `json:"revokedAt,omitempty"`
}

type EngineAPIKeyInput struct {
	Name           string   `json:"name"`
	Scopes         []string `json:"scopes"`
	AllowedOrigins []string `json:"allowedOrigins"`
	AllowedIPs     []string `json:"allowedIps"`
	ExpiresAt      string   `json:"expiresAt"`
}

type EngineAPIKeySecret struct {
	Key       EngineAPIKey `json:"key"`
	Secret    string       `json:"secret"`
	Plaintext string       `json:"plaintext"`
}

type EngineAPIKeyVerification struct {
	App EngineApp    `json:"app"`
	Key EngineAPIKey `json:"key"`
}

type EngineAPIUsageInput struct {
	AppID      string
	KeyID      string
	Method     string
	Path       string
	StatusCode int
	IPAddress  string
	UserAgent  string
}

type EngineAPIUsage struct {
	ID         string `json:"id"`
	AppID      string `json:"appId"`
	KeyID      string `json:"keyId"`
	Method     string `json:"method"`
	Path       string `json:"path"`
	StatusCode int64  `json:"statusCode"`
	IPAddress  string `json:"ipAddress"`
	UserAgent  string `json:"userAgent"`
	CreatedAt  string `json:"createdAt"`
}

type AdminStore interface {
	UserStore
	EngineStore
	ListUsers(ctx context.Context) ([]User, error)
	AdminUserDetail(ctx context.Context, id string) (AdminUserDetail, error)
	UpdateUser(ctx context.Context, id string, input UserUpdateInput) (User, error)
	ListPollClassifications(ctx context.Context) ([]PollClassification, error)
	UpsertPollClassification(ctx context.Context, input PollClassificationInput) (PollClassification, error)
	ListPolls(ctx context.Context, publicOnly bool) ([]Poll, error)
	GetPollByID(ctx context.Context, id string) (Poll, bool, error)
	GetPollBySlug(ctx context.Context, slug string, publicOnly bool) (Poll, bool, error)
	UpsertPoll(ctx context.Context, input PollInput) (Poll, error)
	UpdatePoll(ctx context.Context, id string, input PollInput) (Poll, error)
	UpdatePollResolution(ctx context.Context, id string, status string, outcome string, resolvedBy string, resolutionSource string, evidenceURL string, notes string) (Poll, error)
	CreateNewsCandidateIfMissing(ctx context.Context, input NewsCandidateInput) (NewsCandidate, bool, error)
	ListNewsCandidates(ctx context.Context, status string, limit int64) ([]NewsCandidate, error)
	GetNewsCandidate(ctx context.Context, id string) (NewsCandidate, bool, error)
	UpdatePendingNewsCandidate(ctx context.Context, id string, input NewsCandidateUpdateInput) (NewsCandidate, error)
	ReviewNewsCandidate(ctx context.Context, id string, status string, reviewedBy string, reviewNotes string, pollID string) (NewsCandidate, error)
	QuoteTrade(ctx context.Context, input TradeInput) (TradeQuote, error)
	CollateralRequirement(ctx context.Context) (CollateralRequirement, error)
	CreateTrade(ctx context.Context, user User, input TradeInput) (Trade, error)
	CreateShieldedTradeBatch(ctx context.Context, input ShieldedTradeBatchInput) (ShieldedTradeBatch, error)
	CurrentShieldedTradeBatch(ctx context.Context, vault string, now time.Time) (ShieldedTradeBatch, bool, error)
	ListDueShieldedTradeBatches(ctx context.Context, now time.Time) ([]ShieldedTradeBatch, error)
	UpdateShieldedTradeBatch(ctx context.Context, batchID string, status string, transactionID string, transactionHash string, errorMessage string) (ShieldedTradeBatch, error)
	CreateShieldedTradeOrder(ctx context.Context, input ShieldedTradeOrderInput) (ShieldedTradeOrder, error)
	ListShieldedTradeOrders(ctx context.Context, batchID string) ([]ShieldedTradeOrder, error)
	CompleteShieldedTradeOrder(ctx context.Context, id string, tradeID string, status string, errorMessage string) error
	QuoteCashout(ctx context.Context, userID string, input CashoutInput) (CashoutQuote, error)
	CashoutPosition(ctx context.Context, userID string, input CashoutInput, transactionIDs []string, payoutStatus string, payoutError string) (CashoutQuote, error)
	UserPortfolio(ctx context.Context, userID string) (Portfolio, error)
	MarketActivity(ctx context.Context, slug string) ([]MarketActivity, error)
	MarketStats(ctx context.Context, slug string) (MarketStats, error)
	ListMarketComments(ctx context.Context, slug string) ([]MarketComment, error)
	CreateMarketComment(ctx context.Context, user User, slug string, body string) (MarketComment, error)
	ReportMarketComment(ctx context.Context, user User, id string, reason string) (MarketComment, error)
	ListAllMarketComments(ctx context.Context) ([]MarketComment, error)
	UpdateMarketCommentStatus(ctx context.Context, id string, status string) (MarketComment, error)
	ListNotifications(ctx context.Context, userID string) ([]Notification, error)
	CreateNotification(ctx context.Context, userID string, kind string, title string, detail string, link string) (Notification, error)
	MarkNotificationRead(ctx context.Context, userID string, id string) (Notification, bool, error)
	MarkAllNotificationsRead(ctx context.Context, userID string) (int64, error)
	ListWatchlist(ctx context.Context, userID string) ([]WatchlistItem, error)
	AddWatchlist(ctx context.Context, userID string, slug string) ([]WatchlistItem, bool, error)
	RemoveWatchlist(ctx context.Context, userID string, slug string) ([]WatchlistItem, error)
	GetMarketAlert(ctx context.Context, userID string, slug string) (MarketAlert, bool, error)
	UpsertMarketAlert(ctx context.Context, userID string, slug string, input MarketAlertInput) (MarketAlert, error)
	DeleteMarketAlert(ctx context.Context, userID string, slug string) error
	NotifyMarketPriceAlerts(ctx context.Context, pollID string, exceptUserID string, oldYesPercent int64, newYesPercent int64) (int64, error)
	NotifyMarketResolutionAlerts(ctx context.Context, pollSlug string, status string) (int64, error)
	ProcessDueMarketClosingAlerts(ctx context.Context, now time.Time) (int64, error)
	NotifyWatchers(ctx context.Context, pollSlug string, exceptUserID string, kind string, title string, detail string, link string) (int64, error)
	ListAdminActivity(ctx context.Context) ([]AdminActivity, error)
	CreateAdminActivity(ctx context.Context, input AdminActivityInput) (AdminActivity, error)
	PreviewPollSettlement(ctx context.Context, pollID string, status string, outcome string) (SettlementPreview, error)
	SettlePollTrades(ctx context.Context, pollID string, status string, outcome string, transactionIDs []string, payoutStatus string, payoutError string) (SettlementPreview, error)
	SettlementPayoutRetryPreview(ctx context.Context, pollID string) (SettlementPreview, error)
	RecordSettlementPayoutRetry(ctx context.Context, pollID string, transactionIDs []string, payoutStatus string, payoutError string) (SettlementPreview, error)
	AdminSettlementDetail(ctx context.Context, pollID string) (AdminSettlementDetail, error)
	GetTradeByID(ctx context.Context, id string) (Trade, error)
	UpdateTradeEscrowReconciliation(ctx context.Context, id string, status string, errorMessage string, verifiedAt string, from string, to string, amount float64) (Trade, error)
	UpdateSettlementPayoutReconciliation(ctx context.Context, pollID string, payoutStatus string, payoutError string, transactionHash string) (Poll, error)
	ListTrades(ctx context.Context, userID string, pollID string, limit int64) ([]Trade, error)
	PrivateClaimTree(ctx context.Context, pollID string) (PrivateClaimTree, error)
	RecordPrivateClaimRoot(ctx context.Context, pollID string, root string, leafCount int64, reason string) (PrivateClaimRootSnapshot, error)
	PrivateClaimRootHistory(ctx context.Context, pollID string, limit int64) ([]PrivateClaimRootSnapshot, error)
	ReserveDirectTradePayout(ctx context.Context, userID string, tradeID string) (Trade, error)
	CompleteDirectTradePayout(ctx context.Context, userID string, tradeID string, transactionIDs []string, payoutStatus string, payoutError string) (Trade, error)
	ReservePrivateClaim(ctx context.Context, userID string, tradeID string, leaf string, root string, nullifierHash string, zkProofSubmissionID string) (PrivateClaim, Trade, error)
	RecordPrivateClaimRegistryTransaction(ctx context.Context, userID string, claimID string, transactionID string, registryStatus string, registryError string) (PrivateClaim, Trade, error)
	CompletePrivateClaimPayout(ctx context.Context, userID string, claimID string, transactionIDs []string, payoutStatus string, payoutError string) (PrivateClaim, Trade, error)
	CreateShieldedWithdrawal(ctx context.Context, userID string, input ShieldedWithdrawalInput) (ShieldedWithdrawal, error)
	ListUserShieldedWithdrawals(ctx context.Context, userID string, limit int64) ([]ShieldedWithdrawal, error)
	ListShieldedWithdrawals(ctx context.Context, status string, limit int64) ([]ShieldedWithdrawal, error)
	ListDueShieldedWithdrawals(ctx context.Context, limit int64, now time.Time) ([]ShieldedWithdrawal, error)
	GetShieldedWithdrawal(ctx context.Context, id string) (ShieldedWithdrawal, bool, error)
	MarkShieldedWithdrawalProcessing(ctx context.Context, id string) (ShieldedWithdrawal, bool, error)
	CompleteShieldedWithdrawal(ctx context.Context, id string, status string, transactionID string, transactionHash string, errorMessage string) (ShieldedWithdrawal, error)
	ScheduleShieldedWithdrawalRetry(ctx context.Context, id string, executeAfter time.Time, errorMessage string) (ShieldedWithdrawal, error)
	RetryAnyShieldedWithdrawal(ctx context.Context, id string, executeAfter time.Time) (ShieldedWithdrawal, error)
	RetryShieldedWithdrawal(ctx context.Context, userID string, id string, executeAfter time.Time) (ShieldedWithdrawal, error)
	MarkShieldedWithdrawalSuspicious(ctx context.Context, id string, reason string) (ShieldedWithdrawal, error)
	RecoverStaleShieldedWithdrawals(ctx context.Context, staleBefore time.Time, executeAfter time.Time, maxAttempts int64) (int64, error)
	GetSystemSetting(ctx context.Context, key string) (string, bool, error)
	SetSystemSetting(ctx context.Context, key string, value string) error
	CountSponsoredEscrows(ctx context.Context, owner string, since string) (int64, error)
	CountGlobalSponsoredEscrows(ctx context.Context, since string) (int64, error)
	CreateSponsoredEscrow(ctx context.Context, owner string, pollID string, side string, amount float64, fee float64, total float64, txHash string) error
	ListSettlementPayoutAttempts(ctx context.Context, pollID string) ([]SettlementPayoutAttempt, error)
	ApplyMarketAutomation(ctx context.Context) (int64, error)
	SeedDefaults(ctx context.Context, seedDemoPolls bool) error
}

type EngineStore interface {
	CreateEngineApp(ctx context.Context, input EngineAppInput) (EngineApp, error)
	ListEngineApps(ctx context.Context) ([]EngineApp, error)
	GetEngineApp(ctx context.Context, id string) (EngineApp, bool, error)
	CreateEngineAPIKey(ctx context.Context, appID string, input EngineAPIKeyInput, prefix string, secretHash string) (EngineAPIKey, error)
	ListEngineAPIKeys(ctx context.Context, appID string) ([]EngineAPIKey, error)
	VerifyEngineAPIKey(ctx context.Context, prefix string, secretHash string) (EngineAPIKeyVerification, bool, error)
	RevokeEngineAPIKey(ctx context.Context, id string) (EngineAPIKey, error)
	RecordEngineAPIUsage(ctx context.Context, input EngineAPIUsageInput) error
	ListEngineAPIUsage(ctx context.Context, appID string, limit int64) ([]EngineAPIUsage, error)
	Close(ctx context.Context) error
}

var defaultClassifications = []PollClassificationInput{
	{Slug: "elections", Name: "Elections", DisplayName: "Elections", CallName: "market", Description: "Candidate, filing, polling, and election outcome markets.", SortOrder: 10},
	{Slug: "congress", Name: "Congress", DisplayName: "Congress", CallName: "market", Description: "House and Senate legislative movement markets.", SortOrder: 20},
	{Slug: "lgu", Name: "LGU", DisplayName: "LGU", CallName: "market", Description: "Local government announcements, city hall moves, and regional policy markets.", SortOrder: 30},
	{Slug: "policy", Name: "Policy", DisplayName: "Policy", CallName: "market", Description: "Comelec, agency, court, and national policy markets.", SortOrder: 40},
}

var defaultPolls = []PollInput{
	{Slug: "non-dynasty-2028-poll-lead", Title: "Will a non-dynasty candidate lead any major 2028 presidential poll before filing week?", PollType: "yes_no", ClassificationID: "elections", CallName: "market", Region: "National", Status: "published", Visibility: "public", OutcomeA: "Yes", OutcomeB: "No", YesPercent: 42, Volume: "P8.4M", Change: "+6.2%", Color: "coral", Hot: true, Featured: true, SortOrder: 10, Liquidity: defaultMarketLiquidity},
	{Slug: "dynasty-reform-third-reading-2026", Title: "Will the House pass a political dynasty reform bill on third reading in 2026?", PollType: "bill_passage", ClassificationID: "congress", CallName: "market", Region: "House", Status: "published", Visibility: "public", OutcomeA: "Passes", OutcomeB: "Fails", YesPercent: 27, Volume: "P2.1M", Change: "-1.8%", Color: "green", SortOrder: 20, Liquidity: defaultMarketLiquidity},
	{Slug: "metro-manila-four-day-workweek-pilot", Title: "Will any Metro Manila city announce a four-day workweek pilot this quarter?", PollType: "deadline", ClassificationID: "lgu", CallName: "market", Region: "Metro Manila", Status: "published", Visibility: "public", OutcomeA: "Announced", OutcomeB: "Not announced", YesPercent: 61, Volume: "P5.7M", Change: "+3.5%", Color: "blue", Hot: true, SortOrder: 30, Liquidity: defaultMarketLiquidity},
	{Slug: "online-voter-registration-year-end", Title: "Will online voter registration expansion be formally rolled out before year-end?", PollType: "deadline", ClassificationID: "policy", CallName: "market", Region: "Comelec", Status: "published", Visibility: "public", OutcomeA: "Rolled out", OutcomeB: "Delayed", YesPercent: 54, Volume: "P3.9M", Change: "+0.9%", Color: "yellow", SortOrder: 40, Liquidity: defaultMarketLiquidity},
	{Slug: "positive-net-satisfaction-next-sws", Title: "Will the President post a positive net satisfaction rating in the next SWS national survey?", PollType: "approval_rating", ClassificationID: "elections", CallName: "approval line", Region: "National", Status: "published", Visibility: "public", OutcomeA: "Positive", OutcomeB: "Not positive", YesPercent: 49, Volume: "P1.8M", Change: "+2.4%", Color: "yellow", SortOrder: 50, Liquidity: defaultMarketLiquidity},
	{Slug: "overseas-voter-turnout-over-35", Title: "Will overseas voter turnout exceed 35% in the next national election?", PollType: "turnout", ClassificationID: "elections", CallName: "turnout line", Region: "Overseas", Status: "published", Visibility: "public", OutcomeA: "Over 35%", OutcomeB: "35% or less", YesPercent: 38, Volume: "P1.2M", Change: "-0.7%", Color: "green", SortOrder: 60, Liquidity: defaultMarketLiquidity},
	{Slug: "supreme-court-election-rule-tro", Title: "Will the Supreme Court issue a TRO on a major election rule before campaign season?", PollType: "court_decision", ClassificationID: "policy", CallName: "case market", Region: "Supreme Court", Status: "published", Visibility: "public", OutcomeA: "Issued", OutcomeB: "Not issued", YesPercent: 44, Volume: "P2.6M", Change: "+1.1%", Color: "blue", SortOrder: 70, Liquidity: defaultMarketLiquidity},
	{Slug: "new-comelec-commissioner-before-filing-week", Title: "Will a new Comelec commissioner be appointed before the next filing week?", PollType: "appointment", ClassificationID: "policy", CallName: "appointment market", Region: "Comelec", Status: "published", Visibility: "public", OutcomeA: "Appointed", OutcomeB: "Not appointed", YesPercent: 57, Volume: "P3.1M", Change: "+4.0%", Color: "coral", SortOrder: 80, Liquidity: defaultMarketLiquidity},
	{Slug: "transport-subsidy-tranche-before-quarter-end", Title: "Will a named transport subsidy tranche be released before quarter-end?", PollType: "budget_release", ClassificationID: "policy", CallName: "funding market", Region: "DOTr", Status: "published", Visibility: "public", OutcomeA: "Released", OutcomeB: "Not released", YesPercent: 63, Volume: "P2.4M", Change: "+0.6%", Color: "green", SortOrder: 90, Liquidity: defaultMarketLiquidity},
}

func (s *MemgraphUserStore) SeedDefaults(ctx context.Context, seedDemoPolls bool) error {
	for _, classification := range defaultClassifications {
		if _, err := s.UpsertPollClassification(ctx, classification); err != nil {
			return err
		}
	}
	if !seedDemoPolls {
		return nil
	}
	for _, poll := range defaultPolls {
		if _, ok, err := s.GetPollBySlug(ctx, poll.Slug, false); err != nil {
			return err
		} else if ok {
			continue
		}
		if _, err := s.UpsertPoll(ctx, poll); err != nil {
			return err
		}
	}
	return nil
}

func (s *MemgraphUserStore) ListUsers(ctx context.Context) ([]User, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (u:User)
RETURN
  u.id AS id,
  u.walletAddress AS walletAddress,
  coalesce(u.publicAlias, "") AS publicAlias,
  coalesce(u.walletopsUserId, "") AS walletopsUserId,
  coalesce(u.authProvider, "") AS authProvider,
  coalesce(u.authType, "") AS authType,
  coalesce(u.email, "") AS email,
  coalesce(u.phone, "") AS phone,
  coalesce(u.role, "user") AS role,
  coalesce(u.status, "active") AS status,
  coalesce(u.walletCustody, "") AS walletCustody,
  coalesce(u.notes, "") AS notes,
  u.createdAt AS createdAt,
  u.lastLoginAt AS lastLoginAt,
  u.loginCount AS loginCount
ORDER BY u.lastLoginAt DESC
`, nil)
		if err != nil {
			return nil, err
		}
		users := []User{}
		for rows.Next(ctx) {
			users = append(users, userFromRecord(rows.Record()))
		}
		return users, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]User), nil
}

func (s *MemgraphUserStore) AdminUserDetail(ctx context.Context, id string) (AdminUserDetail, error) {
	user, ok, err := s.GetByID(ctx, id)
	if err != nil {
		return AdminUserDetail{}, err
	}
	if !ok {
		return AdminUserDetail{}, errors.New("user not found")
	}

	portfolio, err := s.UserPortfolio(ctx, id)
	if err != nil {
		return AdminUserDetail{}, err
	}
	grants, err := s.listTokenGrantsForUser(ctx, id)
	if err != nil {
		return AdminUserDetail{}, err
	}
	comments, err := s.listCommentsForUser(ctx, id)
	if err != nil {
		return AdminUserDetail{}, err
	}
	return AdminUserDetail{User: user, Portfolio: portfolio, TokenGrants: grants, Comments: comments}, nil
}

func (s *MemgraphUserStore) listTokenGrantsForUser(ctx context.Context, id string) ([]TokenGrant, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:User {id: $id})-[:RECEIVED_TOKEN_GRANT]->(g:TokenGrant)
RETURN
  g.id AS id,
  g.userId AS userId,
  g.walletAddress AS walletAddress,
  g.grantType AS grantType,
  g.status AS status,
  g.tokenAddress AS tokenAddress,
  g.chainId AS chainId,
  g.amount AS amount,
  g.quantity AS quantity,
  coalesce(g.transactionIds, []) AS transactionIds,
  coalesce(g.error, "") AS error,
  g.createdAt AS createdAt,
  g.updatedAt AS updatedAt
ORDER BY g.createdAt DESC
`, map[string]any{"id": id})
		if err != nil {
			return nil, err
		}
		grants := []TokenGrant{}
		for rows.Next(ctx) {
			grants = append(grants, tokenGrantFromRecord(rows.Record()))
		}
		return grants, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]TokenGrant), nil
}

func (s *MemgraphUserStore) listCommentsForUser(ctx context.Context, id string) ([]MarketComment, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (u:User {id: $id})-[:POSTED_COMMENT]->(m:MarketComment)-[:ON_POLL]->(p:Poll)
RETURN
  m.id AS id,
  u.id AS userId,
  p.id AS pollId,
  p.slug AS pollSlug,
  CASE WHEN trim(coalesce(u.publicAlias, "")) = "" THEN "Anonymous Trader" ELSE u.publicAlias END AS actor,
  coalesce(m.body, "") AS body,
  coalesce(m.status, "visible") AS status,
  coalesce(m.reportCount, 0) AS reportCount,
  coalesce(m.latestReportReason, "") AS latestReportReason,
  coalesce(m.latestReportedAt, "") AS latestReportedAt,
  m.createdAt AS createdAt
ORDER BY m.createdAt DESC
LIMIT 80
`, map[string]any{"id": id})
		if err != nil {
			return nil, err
		}
		comments := []MarketComment{}
		for rows.Next(ctx) {
			comments = append(comments, marketCommentFromRecord(rows.Record()))
		}
		return comments, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]MarketComment), nil
}

func (s *MemgraphUserStore) UpdateUser(ctx context.Context, id string, input UserUpdateInput) (User, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	params := map[string]any{
		"id":     id,
		"role":   defaultString(input.Role, "user"),
		"status": defaultString(input.Status, "active"),
		"notes":  input.Notes,
		"now":    time.Now().UTC().Format(time.RFC3339),
	}

	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (u:User {id: $id})
SET u.role = $role, u.status = $status, u.notes = $notes, u.updatedAt = $now
RETURN
  u.id AS id,
  u.walletAddress AS walletAddress,
  coalesce(u.publicAlias, "") AS publicAlias,
  coalesce(u.walletopsUserId, "") AS walletopsUserId,
  coalesce(u.authProvider, "") AS authProvider,
  coalesce(u.authType, "") AS authType,
  coalesce(u.email, "") AS email,
  coalesce(u.phone, "") AS phone,
  coalesce(u.role, "user") AS role,
  coalesce(u.status, "active") AS status,
  coalesce(u.walletCustody, "") AS walletCustody,
  coalesce(u.notes, "") AS notes,
  u.createdAt AS createdAt,
  u.lastLoginAt AS lastLoginAt,
  u.loginCount AS loginCount
`, params)
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return userFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("user not found")
	})
	if err != nil {
		return User{}, err
	}
	return result.(User), nil
}

func (s *MemgraphUserStore) AdminSettlementDetail(ctx context.Context, pollID string) (AdminSettlementDetail, error) {
	poll, err := s.pollByID(ctx, pollID)
	if err != nil {
		return AdminSettlementDetail{}, err
	}
	settlement, err := s.SettlementPayoutRetryPreview(ctx, pollID)
	if err != nil {
		return AdminSettlementDetail{}, err
	}
	trades, err := s.ListTrades(ctx, "", pollID, 300)
	if err != nil {
		return AdminSettlementDetail{}, err
	}
	attempts, err := s.ListSettlementPayoutAttempts(ctx, pollID)
	if err != nil {
		return AdminSettlementDetail{}, err
	}
	return AdminSettlementDetail{Poll: poll, Settlement: settlement, Trades: trades, PayoutAttempts: attempts}, nil
}

func (s *MemgraphUserStore) pollByID(ctx context.Context, id string) (Poll, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (p:Poll {id: $id})-[:CLASSIFIED_AS]->(c:PollClassification)
`+pollReturnCypher(), map[string]any{"id": id})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return pollFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("poll not found")
	})
	if err != nil {
		return Poll{}, err
	}
	return result.(Poll), nil
}

func (s *MemgraphUserStore) ListPollClassifications(ctx context.Context) ([]PollClassification, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (c:PollClassification)
RETURN
  c.id AS id,
  c.slug AS slug,
  c.name AS name,
  c.displayName AS displayName,
  c.description AS description,
  c.callName AS callName,
  coalesce(c.sortOrder, 0) AS sortOrder,
  c.createdAt AS createdAt,
  c.updatedAt AS updatedAt
ORDER BY c.sortOrder ASC, c.name ASC
`, nil)
		if err != nil {
			return nil, err
		}
		classifications := []PollClassification{}
		for rows.Next(ctx) {
			classifications = append(classifications, pollClassificationFromRecord(rows.Record()))
		}
		return classifications, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]PollClassification), nil
}

func (s *MemgraphUserStore) UpsertPollClassification(ctx context.Context, input PollClassificationInput) (PollClassification, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	slug := slugify(input.Slug)
	if slug == "" {
		slug = slugify(input.Name)
	}

	params := map[string]any{
		"newID":       uuid.NewString(),
		"slug":        slug,
		"name":        defaultString(input.Name, input.DisplayName),
		"displayName": defaultString(input.DisplayName, input.Name),
		"description": input.Description,
		"callName":    defaultString(input.CallName, "market"),
		"sortOrder":   input.SortOrder,
		"now":         now,
	}

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MERGE (c:PollClassification {slug: $slug})
ON CREATE SET c.id = $newID, c.createdAt = $now
SET
  c.name = $name,
  c.displayName = $displayName,
  c.description = $description,
  c.callName = $callName,
  c.sortOrder = $sortOrder,
  c.updatedAt = $now
RETURN
  c.id AS id,
  c.slug AS slug,
  c.name AS name,
  c.displayName AS displayName,
  c.description AS description,
  c.callName AS callName,
  coalesce(c.sortOrder, 0) AS sortOrder,
  c.createdAt AS createdAt,
  c.updatedAt AS updatedAt
`, params)
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return pollClassificationFromRecord(rows.Record()), nil
		}
		return nil, rows.Err()
	})
	if err != nil {
		return PollClassification{}, err
	}
	return result.(PollClassification), nil
}

func (s *MemgraphUserStore) ListPolls(ctx context.Context, publicOnly bool) ([]Poll, error) {
	if _, err := s.ApplyMarketAutomation(ctx); err != nil {
		return nil, err
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	query := `
MATCH (p:Poll)-[:CLASSIFIED_AS]->(c:PollClassification)
`
	if publicOnly {
		query += `WHERE p.status = "published"
  AND p.visibility = "public"
  AND (coalesce(p.startsAt, "") = "" OR p.startsAt <= $now)
  AND (coalesce(p.endsAt, "") = "" OR p.endsAt >= $now)
`
	}
	query += pollReturnCypher() + `
ORDER BY p.sortOrder ASC, p.updatedAt DESC
`
	params := map[string]any{
		"now": time.Now().UTC().Format(time.RFC3339),
	}

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, query, params)
		if err != nil {
			return nil, err
		}
		polls := []Poll{}
		for rows.Next(ctx) {
			polls = append(polls, pollFromRecord(rows.Record()))
		}
		return polls, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]Poll), nil
}

func (s *MemgraphUserStore) ApplyMarketAutomation(ctx context.Context) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (p:Poll)
WHERE p.status = "published"
  AND coalesce(p.endsAt, "") <> ""
  AND p.endsAt < $now
SET
  p.status = "paused",
  p.automationState = "awaiting_resolution",
  p.updatedAt = $now
RETURN count(p) AS automated
`, map[string]any{"now": now})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			automated := intValue(rows.Record(), "automated")
			liquidityRows, err := tx.Run(ctx, `
MATCH (p:Poll)
WHERE p.status = "published"
  AND coalesce(p.marketMakerCollected, 0.0) > 0.0
  AND coalesce(p.marketMakerCollected, 0.0) <= 1000.0
  AND coalesce(p.liquidity, 500.0) > 1000.0
  AND coalesce(p.yesPercent, 50) = 50
  AND coalesce(p.noPercent, 50) = 50
SET
  p.liquidity = $liquidity,
  p.updatedAt = $now
RETURN count(p) AS normalized
`, map[string]any{
				"liquidity": defaultMarketLiquidity,
				"now":       now,
			})
			if err != nil {
				return nil, err
			}
			normalized := int64(0)
			if liquidityRows.Next(ctx) {
				normalized = intValue(liquidityRows.Record(), "normalized")
			}
			if err := liquidityRows.Err(); err != nil {
				return nil, err
			}
			reconcileRows, err := tx.Run(ctx, `
MATCH (p:Poll)
WHERE p.status = "published"
OPTIONAL MATCH (t:Trade {status: "open"})-[:ON_POLL]->(p)
WITH p,
  sum(CASE WHEN t.side = "yes" THEN coalesce(t.shares, 0.0) ELSE 0.0 END) AS openYesShares,
  sum(CASE WHEN t.side = "no" THEN coalesce(t.shares, 0.0) ELSE 0.0 END) AS openNoShares
WITH p, coalesce(openYesShares, 0.0) AS openYesShares, coalesce(openNoShares, 0.0) AS openNoShares
WHERE coalesce(p.marketMakerCollected, 0.0) > 0.0
  AND (abs(coalesce(p.yesShares, 0.0) - openYesShares) > 0.01 OR abs(coalesce(p.noShares, 0.0) - openNoShares) > 0.01)
SET
  p.yesShares = openYesShares,
  p.noShares = openNoShares,
  p.yesPercent = CASE WHEN openYesShares = 0.0 AND openNoShares = 0.0 THEN 50 ELSE coalesce(p.yesPercent, 50) END,
  p.noPercent = CASE WHEN openYesShares = 0.0 AND openNoShares = 0.0 THEN 50 ELSE coalesce(p.noPercent, 50) END,
  p.updatedAt = $now
RETURN count(p) AS reconciled
`, map[string]any{"now": now})
			if err != nil {
				return nil, err
			}
			reconciled := int64(0)
			if reconcileRows.Next(ctx) {
				reconciled = intValue(reconcileRows.Record(), "reconciled")
			}
			if err := reconcileRows.Err(); err != nil {
				return nil, err
			}
			return automated + normalized + reconciled, nil
		}
		return int64(0), rows.Err()
	})
	if err != nil {
		return 0, err
	}
	return result.(int64), nil
}

func (s *MemgraphUserStore) GetPollBySlug(ctx context.Context, slug string, publicOnly bool) (Poll, bool, error) {
	if _, err := s.ApplyMarketAutomation(ctx); err != nil {
		return Poll{}, false, err
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	query := `
MATCH (p:Poll {slug: $slug})-[:CLASSIFIED_AS]->(c:PollClassification)
`
	if publicOnly {
		query += `WHERE p.status IN ["published", "paused", "resolved", "cancelled"]
  AND p.visibility = "public"
`
	}
	query += pollReturnCypher()

	params := map[string]any{
		"slug": slugify(slug),
		"now":  time.Now().UTC().Format(time.RFC3339),
	}

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, query, params)
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return pollFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, nil
	})
	if err != nil {
		return Poll{}, false, err
	}
	if result == nil {
		return Poll{}, false, nil
	}
	return result.(Poll), true, nil
}

func (s *MemgraphUserStore) GetPollByID(ctx context.Context, id string) (Poll, bool, error) {
	if _, err := s.ApplyMarketAutomation(ctx); err != nil {
		return Poll{}, false, err
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (p:Poll {id: $id})-[:CLASSIFIED_AS]->(c:PollClassification)
`+pollReturnCypher(), map[string]any{"id": strings.TrimSpace(id)})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return pollFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, nil
	})
	if err != nil {
		return Poll{}, false, err
	}
	if result == nil {
		return Poll{}, false, nil
	}
	return result.(Poll), true, nil
}

func (s *MemgraphUserStore) UpsertPoll(ctx context.Context, input PollInput) (Poll, error) {
	return s.savePoll(ctx, "", input)
}

func (s *MemgraphUserStore) UpdatePoll(ctx context.Context, id string, input PollInput) (Poll, error) {
	return s.savePoll(ctx, id, input)
}

func (s *MemgraphUserStore) savePoll(ctx context.Context, id string, input PollInput) (Poll, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	startsAt, err := normalizePollTimestamp(input.StartsAt)
	if err != nil {
		return Poll{}, fmt.Errorf("invalid start date: %w", err)
	}
	endsAt, err := normalizePollTimestamp(input.EndsAt)
	if err != nil {
		return Poll{}, fmt.Errorf("invalid end date: %w", err)
	}
	if startsAt != "" && endsAt != "" && endsAt <= startsAt {
		return Poll{}, errors.New("end date must be after start date")
	}
	slug := slugify(input.Slug)
	if slug == "" {
		slug = slugify(input.Title)
	}
	classificationSlug := slugify(input.ClassificationID)
	if classificationSlug == "" {
		classificationSlug = "elections"
	}
	yesPercent := clamp(input.YesPercent, 1, 99)
	liquidity := clampFloat(defaultFloat(input.Liquidity, defaultMarketLiquidity), 100, 1000000)
	initialYesShares, initialNoShares := initialMarketShares(yesPercent, liquidity)

	params := map[string]any{
		"id":                    id,
		"newID":                 uuid.NewString(),
		"slug":                  slug,
		"title":                 input.Title,
		"pollType":              defaultString(input.PollType, "yes_no"),
		"classificationID":      classificationSlug,
		"callName":              defaultString(input.CallName, "market"),
		"region":                input.Region,
		"status":                defaultString(input.Status, "draft"),
		"visibility":            defaultString(input.Visibility, "public"),
		"outcomeA":              defaultString(input.OutcomeA, "Yes"),
		"outcomeB":              defaultString(input.OutcomeB, "No"),
		"marketGroupId":         strings.TrimSpace(input.MarketGroupID),
		"marketGroupTitle":      strings.TrimSpace(input.MarketGroupTitle),
		"marketChoiceLabel":     strings.TrimSpace(input.MarketChoiceLabel),
		"marketChoiceIndex":     input.MarketChoiceIndex,
		"yesPercent":            yesPercent,
		"noPercent":             100 - yesPercent,
		"volume":                defaultString(input.Volume, "P0"),
		"change":                defaultString(input.Change, "+0.0%"),
		"color":                 defaultString(input.Color, "blue"),
		"hot":                   input.Hot,
		"featured":              input.Featured,
		"sortOrder":             input.SortOrder,
		"liquidity":             liquidity,
		"tradingFrozen":         input.TradingFrozen,
		"commentsDisabled":      input.CommentsDisabled,
		"auditReason":           input.AuditReason,
		"initialYesShares":      initialYesShares,
		"initialNoShares":       initialNoShares,
		"resolutionSource":      input.ResolutionSource,
		"resolutionOutcome":     input.ResolutionOutcome,
		"resolutionEvidenceUrl": input.ResolutionEvidenceURL,
		"resolutionNotes":       input.ResolutionNotes,
		"startsAt":              startsAt,
		"endsAt":                endsAt,
		"now":                   now,
	}

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	matchPoll := `MERGE (p:Poll {slug: $slug})
ON CREATE SET p.id = $newID, p.createdAt = $now, p.yesShares = $initialYesShares, p.noShares = $initialNoShares, p.marketMakerCollected = 0.0`
	if id != "" {
		matchPoll = `MATCH (p:Poll {id: $id})`
	}

	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (c:PollClassification {slug: $classificationID})
`+matchPoll+`
SET
  p.slug = $slug,
  p.title = $title,
  p.pollType = $pollType,
  p.callName = $callName,
  p.region = $region,
  p.status = $status,
  p.visibility = $visibility,
  p.outcomeA = $outcomeA,
  p.outcomeB = $outcomeB,
  p.marketGroupId = $marketGroupId,
  p.marketGroupTitle = $marketGroupTitle,
  p.marketChoiceLabel = $marketChoiceLabel,
  p.marketChoiceIndex = $marketChoiceIndex,
  p.yesPercent = $yesPercent,
  p.noPercent = $noPercent,
  p.volume = $volume,
  p.change = $change,
  p.color = $color,
  p.hot = $hot,
  p.featured = $featured,
  p.sortOrder = $sortOrder,
  p.liquidity = $liquidity,
  p.tradingFrozen = $tradingFrozen,
  p.commentsDisabled = $commentsDisabled,
  p.auditReason = $auditReason,
  p.yesShares = coalesce(p.yesShares, $initialYesShares),
  p.noShares = coalesce(p.noShares, $initialNoShares),
  p.marketMakerCollected = coalesce(p.marketMakerCollected, 0.0),
  p.resolutionSource = $resolutionSource,
  p.resolutionOutcome = $resolutionOutcome,
  p.resolutionEvidenceUrl = $resolutionEvidenceUrl,
  p.resolutionNotes = $resolutionNotes,
  p.startsAt = $startsAt,
  p.endsAt = $endsAt,
  p.updatedAt = $now
WITH p, c
OPTIONAL MATCH (p)-[oldClassification:CLASSIFIED_AS]->(:PollClassification)
DELETE oldClassification
WITH p, c
MERGE (p)-[:CLASSIFIED_AS]->(c)
`+pollReturnCypher(), params)
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return pollFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("classification or poll not found")
	})
	if err != nil {
		return Poll{}, err
	}
	return result.(Poll), nil
}

func normalizePollTimestamp(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed.UTC().Format(time.RFC3339), nil
	}

	manila := time.FixedZone("Asia/Manila", 8*60*60)
	for _, layout := range []string{"2006-01-02T15:04", "2006-01-02T15:04:05"} {
		if parsed, err := time.ParseInLocation(layout, value, manila); err == nil {
			return parsed.UTC().Format(time.RFC3339), nil
		}
	}
	return "", errors.New("use an ISO 8601 date and time")
}

func (s *MemgraphUserStore) UpdatePollResolution(ctx context.Context, id string, status string, outcome string, resolvedBy string, resolutionSource string, evidenceURL string, notes string) (Poll, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	status = defaultString(status, "paused")
	if status != "published" && status != "paused" && status != "resolved" && status != "cancelled" && status != "draft" {
		return Poll{}, errors.New("unsupported poll status")
	}
	outcome = strings.ToLower(strings.TrimSpace(outcome))
	if status == "resolved" && outcome != "a" && outcome != "b" {
		return Poll{}, errors.New("resolved polls require outcome a or b")
	}
	if status != "resolved" {
		outcome = ""
	}

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (p:Poll {id: $id})-[:CLASSIFIED_AS]->(c:PollClassification)
SET
  p.status = $status,
  p.resolutionOutcome = $outcome,
  p.resolutionSource = CASE WHEN $resolutionSource <> "" THEN $resolutionSource ELSE coalesce(p.resolutionSource, "") END,
  p.resolutionEvidenceUrl = CASE WHEN $evidenceURL <> "" THEN $evidenceURL ELSE coalesce(p.resolutionEvidenceUrl, "") END,
  p.resolutionNotes = CASE WHEN $notes <> "" THEN $notes ELSE coalesce(p.resolutionNotes, "") END,
  p.resolvedBy = CASE WHEN $status IN ["resolved", "cancelled"] THEN $resolvedBy ELSE coalesce(p.resolvedBy, "") END,
  p.resolvedAt = CASE WHEN $status IN ["resolved", "cancelled"] THEN $now ELSE "" END,
  p.automationState = CASE WHEN $status = "paused" THEN coalesce(p.automationState, "") ELSE "" END,
  p.updatedAt = $now
`+pollReturnCypher(), map[string]any{
			"id":               id,
			"status":           status,
			"outcome":          outcome,
			"resolvedBy":       resolvedBy,
			"resolutionSource": resolutionSource,
			"evidenceURL":      evidenceURL,
			"notes":            notes,
			"now":              now,
		})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return pollFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("poll not found")
	})
	if err != nil {
		return Poll{}, err
	}
	return result.(Poll), nil
}

func pollReturnCypher() string {
	return `
RETURN
  p.id AS id,
  p.slug AS slug,
  p.title AS title,
  coalesce(p.pollType, "yes_no") AS pollType,
  c.slug AS classificationId,
  c.displayName AS category,
  coalesce(p.callName, c.callName, "market") AS callName,
  coalesce(p.region, "") AS region,
  coalesce(p.status, "draft") AS status,
  coalesce(p.visibility, "public") AS visibility,
  coalesce(p.outcomeA, "Yes") AS outcomeA,
  coalesce(p.outcomeB, "No") AS outcomeB,
  coalesce(p.marketGroupId, "") AS marketGroupId,
  coalesce(p.marketGroupTitle, "") AS marketGroupTitle,
  coalesce(p.marketChoiceLabel, "") AS marketChoiceLabel,
  coalesce(p.marketChoiceIndex, 0) AS marketChoiceIndex,
  coalesce(p.yesPercent, 50) AS yesPercent,
  coalesce(p.noPercent, 50) AS noPercent,
  coalesce(p.volume, "P0") AS volume,
  coalesce(p.change, "+0.0%") AS change,
  coalesce(p.color, "blue") AS color,
  coalesce(p.hot, false) AS hot,
  coalesce(p.featured, false) AS featured,
  coalesce(p.sortOrder, 0) AS sortOrder,
  coalesce(p.liquidity, 500.0) AS liquidity,
  coalesce(p.yesShares, 0.0) AS yesShares,
  coalesce(p.noShares, 0.0) AS noShares,
  coalesce(p.marketMakerCollected, 0.0) AS marketMakerCollected,
  coalesce(p.tradingFrozen, false) AS tradingFrozen,
  coalesce(p.commentsDisabled, false) AS commentsDisabled,
  coalesce(p.auditReason, "") AS auditReason,
  coalesce(p.resolutionSource, "") AS resolutionSource,
  coalesce(p.resolutionOutcome, "") AS resolutionOutcome,
  coalesce(p.resolutionEvidenceUrl, "") AS resolutionEvidenceUrl,
  coalesce(p.resolutionNotes, "") AS resolutionNotes,
  coalesce(p.resolvedBy, "") AS resolvedBy,
  coalesce(p.settlementStatus, "") AS settlementStatus,
  coalesce(p.settlementPayoutStatus, "") AS settlementPayoutStatus,
  coalesce(p.settlementPayoutError, "") AS settlementPayoutError,
  coalesce(p.settlementPayoutTransactionIds, []) AS settlementPayoutTransactionIds,
  coalesce(p.resolvedAt, "") AS resolvedAt,
  coalesce(p.startsAt, "") AS startsAt,
  coalesce(p.endsAt, "") AS endsAt,
  p.createdAt AS createdAt,
  p.updatedAt AS updatedAt
`
}

func pollClassificationFromRecord(record *neo4j.Record) PollClassification {
	return PollClassification{
		ID:          stringValue(record, "id"),
		Slug:        stringValue(record, "slug"),
		Name:        stringValue(record, "name"),
		DisplayName: stringValue(record, "displayName"),
		Description: stringValue(record, "description"),
		CallName:    stringValue(record, "callName"),
		SortOrder:   intValue(record, "sortOrder"),
		CreatedAt:   stringValue(record, "createdAt"),
		UpdatedAt:   stringValue(record, "updatedAt"),
	}
}

func pollFromRecord(record *neo4j.Record) Poll {
	marketMakerCollected := roundMoney(floatValue(record, "marketMakerCollected"))
	volume := stringValue(record, "volume")
	if marketMakerCollected > 0 {
		volume = formatPollVolume(marketMakerCollected)
	}
	yesPercent := intValue(record, "yesPercent")
	noPercent := intValue(record, "noPercent")
	yesShares := roundMoney(floatValue(record, "yesShares"))
	noShares := roundMoney(floatValue(record, "noShares"))
	liquidity := roundMoney(floatValue(record, "liquidity"))
	if marketMakerCollected > 0 && yesShares == 0 && noShares == 0 {
		yesPercent = 50
		noPercent = 50
	} else if marketMakerCollected > 0 && (yesShares != 0 || noShares != 0) {
		yesPercent = priceToCents(lmsrPrice(yesShares, noShares, liquidity, "yes"))
		noPercent = 100 - yesPercent
	}
	return Poll{
		ID:                             stringValue(record, "id"),
		Slug:                           stringValue(record, "slug"),
		Title:                          stringValue(record, "title"),
		PollType:                       stringValue(record, "pollType"),
		ClassificationID:               stringValue(record, "classificationId"),
		Category:                       stringValue(record, "category"),
		CallName:                       stringValue(record, "callName"),
		Region:                         stringValue(record, "region"),
		Status:                         stringValue(record, "status"),
		Visibility:                     stringValue(record, "visibility"),
		OutcomeA:                       stringValue(record, "outcomeA"),
		OutcomeB:                       stringValue(record, "outcomeB"),
		MarketGroupID:                  stringValue(record, "marketGroupId"),
		MarketGroupTitle:               stringValue(record, "marketGroupTitle"),
		MarketChoiceLabel:              stringValue(record, "marketChoiceLabel"),
		MarketChoiceIndex:              intValue(record, "marketChoiceIndex"),
		YesPercent:                     yesPercent,
		NoPercent:                      noPercent,
		Volume:                         volume,
		Change:                         stringValue(record, "change"),
		Color:                          stringValue(record, "color"),
		Hot:                            boolValue(record, "hot"),
		Featured:                       boolValue(record, "featured"),
		SortOrder:                      intValue(record, "sortOrder"),
		Liquidity:                      liquidity,
		YesShares:                      yesShares,
		NoShares:                       noShares,
		MarketMakerCollected:           marketMakerCollected,
		TradingFrozen:                  boolValue(record, "tradingFrozen"),
		CommentsDisabled:               boolValue(record, "commentsDisabled"),
		AuditReason:                    stringValue(record, "auditReason"),
		ResolutionSource:               stringValue(record, "resolutionSource"),
		ResolutionOutcome:              stringValue(record, "resolutionOutcome"),
		ResolutionEvidenceURL:          stringValue(record, "resolutionEvidenceUrl"),
		ResolutionNotes:                stringValue(record, "resolutionNotes"),
		ResolvedBy:                     stringValue(record, "resolvedBy"),
		SettlementStatus:               stringValue(record, "settlementStatus"),
		SettlementPayoutStatus:         stringValue(record, "settlementPayoutStatus"),
		SettlementPayoutError:          stringValue(record, "settlementPayoutError"),
		SettlementPayoutTransactionIDs: stringSliceValue(record, "settlementPayoutTransactionIds"),
		ResolvedAt:                     stringValue(record, "resolvedAt"),
		StartsAt:                       stringValue(record, "startsAt"),
		EndsAt:                         stringValue(record, "endsAt"),
		CreatedAt:                      stringValue(record, "createdAt"),
		UpdatedAt:                      stringValue(record, "updatedAt"),
	}
}

func boolValue(record *neo4j.Record, key string) bool {
	value, ok := record.Get(key)
	if !ok || value == nil {
		return false
	}
	typed, ok := value.(bool)
	return ok && typed
}

func formatPollVolume(amount float64) string {
	amount = roundMoney(amount)
	if amount <= 0 {
		return "P0"
	}
	if amount >= 1000000 {
		return fmt.Sprintf("P%.1fM", amount/1000000)
	}
	if amount >= 1000 {
		return fmt.Sprintf("P%.1fK", amount/1000)
	}
	if math.Mod(amount, 1) == 0 {
		return fmt.Sprintf("P%.0f", amount)
	}
	return fmt.Sprintf("P%.2f", amount)
}

func defaultString(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	return strings.TrimSpace(fallback)
}

func slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(" ", "-", "_", "-", "/", "-", "'", "", "\"", "", "?", "", "!", "", ".", "", ",", "", ":", "", ";", "")
	value = replacer.Replace(value)
	for strings.Contains(value, "--") {
		value = strings.ReplaceAll(value, "--", "-")
	}
	return strings.Trim(value, "-")
}

func clamp(value int64, min int64, max int64) int64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func clampFloat(value float64, min float64, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func defaultFloat(value float64, fallback float64) float64 {
	if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return fallback
	}
	return value
}

func initialMarketShares(yesPercent int64, liquidity float64) (float64, float64) {
	probability := clampFloat(float64(yesPercent)/100.0, minMarketPrice, maxMarketPrice)
	diff := liquidity * math.Log(probability/(1-probability))
	if diff >= 0 {
		return roundMoney(diff), 0
	}
	return 0, roundMoney(-diff)
}
