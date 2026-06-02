package store

import (
	"context"
	"errors"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

const defaultMarketLiquidity = 5000.0
const minMarketPrice = 0.01
const maxMarketPrice = 0.99

type TradeInput struct {
	PollID           string  `json:"pollId"`
	Side             string  `json:"side"`
	Amount           float64 `json:"amount"`
	EscrowTxHash     string  `json:"escrowTxHash"`
	EscrowStatus     string  `json:"escrowStatus"`
	EscrowVerifiedAt string  `json:"escrowVerifiedAt"`
	EscrowFrom       string  `json:"escrowFrom"`
	EscrowTo         string  `json:"escrowTo"`
	EscrowAmount     float64 `json:"escrowAmount"`
}

type TradeQuote struct {
	PollID            string  `json:"pollId"`
	Side              string  `json:"side"`
	OutcomeLabel      string  `json:"outcomeLabel"`
	Amount            float64 `json:"amount"`
	SpotPriceCents    int64   `json:"spotPriceCents"`
	AveragePriceCents int64   `json:"averagePriceCents"`
	Shares            float64 `json:"shares"`
	PotentialPayout   float64 `json:"potentialPayout"`
	NewYesPercent     int64   `json:"newYesPercent"`
	NewNoPercent      int64   `json:"newNoPercent"`
	PriceImpactCents  int64   `json:"priceImpactCents"`
	Liquidity         float64 `json:"liquidity"`
}

type CashoutQuote struct {
	PollID            string  `json:"pollId"`
	PollSlug          string  `json:"pollSlug"`
	PollTitle         string  `json:"pollTitle"`
	Side              string  `json:"side"`
	OutcomeLabel      string  `json:"outcomeLabel"`
	Amount            float64 `json:"amount"`
	Shares            float64 `json:"shares"`
	Proceeds          float64 `json:"proceeds"`
	CurrentPriceCents int64   `json:"currentPriceCents"`
	NewYesPercent     int64   `json:"newYesPercent"`
	NewNoPercent      int64   `json:"newNoPercent"`
	PriceImpactCents  int64   `json:"priceImpactCents"`
}

type marketMakerState struct {
	PollID            string
	PollSlug          string
	PollTitle         string
	OutcomeA          string
	OutcomeB          string
	YesPercent        int64
	YesShares         float64
	NoShares          float64
	Liquidity         float64
	MarketStateExists bool
}

type Trade struct {
	ID                   string   `json:"id"`
	UserID               string   `json:"userId"`
	UserWalletAddress    string   `json:"userWalletAddress"`
	PollID               string   `json:"pollId"`
	PollSlug             string   `json:"pollSlug"`
	PollTitle            string   `json:"pollTitle"`
	Side                 string   `json:"side"`
	OutcomeLabel         string   `json:"outcomeLabel"`
	PriceCents           int64    `json:"priceCents"`
	Amount               float64  `json:"amount"`
	Shares               float64  `json:"shares"`
	PotentialPayout      float64  `json:"potentialPayout"`
	Status               string   `json:"status"`
	EscrowTxHash         string   `json:"escrowTxHash"`
	EscrowStatus         string   `json:"escrowStatus"`
	EscrowVerifiedAt     string   `json:"escrowVerifiedAt"`
	EscrowFrom           string   `json:"escrowFrom"`
	EscrowTo             string   `json:"escrowTo"`
	EscrowAmount         float64  `json:"escrowAmount"`
	EscrowError          string   `json:"escrowError"`
	SettlementStatus     string   `json:"settlementStatus"`
	SettlementOutcome    string   `json:"settlementOutcome"`
	SettlementPayout     float64  `json:"settlementPayout"`
	PayoutStatus         string   `json:"payoutStatus"`
	PayoutError          string   `json:"payoutError"`
	PayoutTransactionIDs []string `json:"payoutTransactionIds"`
	SettledAt            string   `json:"settledAt"`
	CreatedAt            string   `json:"createdAt"`
}

type Position struct {
	PollID          string  `json:"pollId"`
	PollSlug        string  `json:"pollSlug"`
	PollTitle       string  `json:"pollTitle"`
	Category        string  `json:"category"`
	Region          string  `json:"region"`
	Side            string  `json:"side"`
	OutcomeLabel    string  `json:"outcomeLabel"`
	Amount          float64 `json:"amount"`
	Shares          float64 `json:"shares"`
	AveragePrice    float64 `json:"averagePrice"`
	CurrentPrice    int64   `json:"currentPrice"`
	CurrentValue    float64 `json:"currentValue"`
	PotentialPayout float64 `json:"potentialPayout"`
	UnrealizedPnl   float64 `json:"unrealizedPnl"`
	RealizedPnl     float64 `json:"realizedPnl"`
	TradeCount      int64   `json:"tradeCount"`
	LastTradeAt     string  `json:"lastTradeAt"`
}

type PortfolioSummary struct {
	OpenPositions   int64   `json:"openPositions"`
	TotalExposure   float64 `json:"totalExposure"`
	CurrentValue    float64 `json:"currentValue"`
	PotentialPayout float64 `json:"potentialPayout"`
	SettledPnl      float64 `json:"settledPnl"`
	UnrealizedPnl   float64 `json:"unrealizedPnl"`
	RealizedPnl     float64 `json:"realizedPnl"`
	NetPnl          float64 `json:"netPnl"`
}

type Portfolio struct {
	Summary   PortfolioSummary `json:"summary"`
	Positions []Position       `json:"positions"`
	Trades    []Trade          `json:"trades"`
}

type CashoutInput struct {
	PollID string  `json:"pollId"`
	Side   string  `json:"side"`
	Amount float64 `json:"amount"`
}

type MarketStats struct {
	PollID               string  `json:"pollId"`
	PollSlug             string  `json:"pollSlug"`
	YesShares            float64 `json:"yesShares"`
	NoShares             float64 `json:"noShares"`
	Liquidity            float64 `json:"liquidity"`
	TradeCount           int64   `json:"tradeCount"`
	HolderCount          int64   `json:"holderCount"`
	YesHolderCount       int64   `json:"yesHolderCount"`
	NoHolderCount        int64   `json:"noHolderCount"`
	OpenInterest         float64 `json:"openInterest"`
	MarketMakerCollected float64 `json:"marketMakerCollected"`
}

type MarketActivity struct {
	ID               string  `json:"id"`
	Kind             string  `json:"kind"`
	Status           string  `json:"status"`
	Actor            string  `json:"actor"`
	Side             string  `json:"side"`
	OutcomeLabel     string  `json:"outcomeLabel"`
	Amount           float64 `json:"amount"`
	Payout           float64 `json:"payout"`
	PriceCents       int64   `json:"priceCents"`
	PriceImpactCents int64   `json:"priceImpactCents"`
	CreatedAt        string  `json:"createdAt"`
}

type MarketComment struct {
	ID                 string `json:"id"`
	UserID             string `json:"userId"`
	PollID             string `json:"pollId"`
	PollSlug           string `json:"pollSlug"`
	Actor              string `json:"actor"`
	Body               string `json:"body"`
	Status             string `json:"status"`
	ReportCount        int64  `json:"reportCount"`
	LatestReportReason string `json:"latestReportReason"`
	LatestReportedAt   string `json:"latestReportedAt"`
	CreatedAt          string `json:"createdAt"`
}

type SettlementRecipient struct {
	UserID        string  `json:"userId"`
	WalletAddress string  `json:"walletAddress"`
	Email         string  `json:"email"`
	Amount        float64 `json:"amount"`
	Shares        float64 `json:"shares"`
	TradeCount    int64   `json:"tradeCount"`
}

type SettlementPreview struct {
	PollID            string                `json:"pollId"`
	PollTitle         string                `json:"pollTitle"`
	Status            string                `json:"status"`
	Outcome           string                `json:"outcome"`
	WinnerSide        string                `json:"winnerSide"`
	RecipientCount    int64                 `json:"recipientCount"`
	OpenTradeCount    int64                 `json:"openTradeCount"`
	WinningTradeCount int64                 `json:"winningTradeCount"`
	LosingTradeCount  int64                 `json:"losingTradeCount"`
	WinningShares     float64               `json:"winningShares"`
	PayoutRequired    float64               `json:"payoutRequired"`
	Recipients        []SettlementRecipient `json:"recipients"`
}

type SettlementPayoutAttempt struct {
	ID             string   `json:"id"`
	PollID         string   `json:"pollId"`
	AttemptType    string   `json:"attemptType"`
	PayoutStatus   string   `json:"payoutStatus"`
	PayoutError    string   `json:"payoutError"`
	TransactionIDs []string `json:"transactionIds"`
	PayoutRequired float64  `json:"payoutRequired"`
	RecipientCount int64    `json:"recipientCount"`
	Actor          string   `json:"actor"`
	CreatedAt      string   `json:"createdAt"`
}

func (s *MemgraphUserStore) CreateTrade(ctx context.Context, user User, input TradeInput) (Trade, error) {
	side := strings.ToLower(strings.TrimSpace(input.Side))
	if err := validateTradeInput(input.PollID, side, input.Amount); err != nil {
		return Trade{}, err
	}
	if input.Amount > 1000000 {
		return Trade{}, errors.New("amount is too large")
	}
	escrowTxHash := strings.ToLower(strings.TrimSpace(input.EscrowTxHash))
	if escrowTxHash == "" {
		return Trade{}, errors.New("escrow transaction hash is required")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	params := map[string]any{
		"id":           uuid.NewString(),
		"userID":       user.ID,
		"pollID":       input.PollID,
		"side":         side,
		"amount":       roundMoney(input.Amount),
		"escrowTxHash": escrowTxHash,
		"now":          now,
	}

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		usedRows, err := tx.Run(ctx, `
MATCH (t:Trade {escrowTxHash: $escrowTxHash})
RETURN count(t) AS used
`, params)
		if err != nil {
			return nil, err
		}
		if usedRows.Next(ctx) && intValue(usedRows.Record(), "used") > 0 {
			return nil, errors.New("escrow transaction already used")
		}
		if err := usedRows.Err(); err != nil {
			return nil, err
		}

		rows, err := tx.Run(ctx, tradeablePollStateCypher(), params)
		if err != nil {
			return nil, err
		}
		if !rows.Next(ctx) {
			if err := rows.Err(); err != nil {
				return nil, err
			}
			return nil, errors.New("poll not found or not tradeable")
		}
		state := marketMakerStateFromRecord(rows.Record())
		if err := rows.Err(); err != nil {
			return nil, err
		}

		quote, newYesShares, newNoShares, err := quoteMarketMaker(state, side, roundMoney(input.Amount))
		if err != nil {
			return nil, err
		}

		writeParams := map[string]any{
			"id":               params["id"],
			"userID":           user.ID,
			"pollID":           input.PollID,
			"side":             side,
			"amount":           roundMoney(input.Amount),
			"escrowTxHash":     escrowTxHash,
			"escrowStatus":     defaultString(strings.TrimSpace(input.EscrowStatus), "verified"),
			"escrowVerifiedAt": strings.TrimSpace(input.EscrowVerifiedAt),
			"escrowFrom":       strings.ToLower(strings.TrimSpace(input.EscrowFrom)),
			"escrowTo":         strings.ToLower(strings.TrimSpace(input.EscrowTo)),
			"escrowAmount":     roundMoney(input.EscrowAmount),
			"escrowError":      "",
			"outcomeLabel":     quote.OutcomeLabel,
			"priceCents":       quote.AveragePriceCents,
			"spotPriceCents":   quote.SpotPriceCents,
			"shares":           quote.Shares,
			"potentialPayout":  quote.PotentialPayout,
			"priceImpactCents": quote.PriceImpactCents,
			"liquidity":        quote.Liquidity,
			"newYesShares":     newYesShares,
			"newNoShares":      newNoShares,
			"newYesPercent":    quote.NewYesPercent,
			"newNoPercent":     quote.NewNoPercent,
			"now":              now,
		}
		tradeRows, err := tx.Run(ctx, `
MATCH (u:User {id: $userID})
MATCH (p:Poll {id: $pollID})-[:CLASSIFIED_AS]->(c:PollClassification)
WHERE p.status = "published" AND p.visibility = "public" AND coalesce(p.tradingFrozen, false) = false
SET
  p.yesShares = $newYesShares,
  p.noShares = $newNoShares,
  p.yesPercent = $newYesPercent,
  p.noPercent = $newNoPercent,
  p.liquidity = $liquidity,
  p.marketMakerCollected = coalesce(p.marketMakerCollected, 0.0) + $amount,
  p.updatedAt = $now
CREATE (t:Trade {
  id: $id,
  userId: $userID,
  pollId: $pollID,
  side: $side,
  outcomeLabel: $outcomeLabel,
  priceCents: $priceCents,
  spotPriceCents: $spotPriceCents,
  amount: $amount,
  escrowTxHash: $escrowTxHash,
  escrowStatus: $escrowStatus,
  escrowVerifiedAt: $escrowVerifiedAt,
  escrowFrom: $escrowFrom,
  escrowTo: $escrowTo,
  escrowAmount: $escrowAmount,
  escrowError: $escrowError,
  shares: $shares,
  potentialPayout: $potentialPayout,
  priceImpactCents: $priceImpactCents,
  marketMakerLiquidity: $liquidity,
  status: "open",
  createdAt: $now
})
MERGE (u)-[:PLACED_TRADE]->(t)
MERGE (t)-[:ON_POLL]->(p)
RETURN
  t.id AS id,
  t.userId AS userId,
  coalesce(u.walletAddress, "") AS userWalletAddress,
  p.id AS pollId,
  p.slug AS pollSlug,
  p.title AS pollTitle,
  t.side AS side,
  t.outcomeLabel AS outcomeLabel,
  t.priceCents AS priceCents,
  t.amount AS amount,
  t.shares AS shares,
  t.potentialPayout AS potentialPayout,
  t.status AS status,
  coalesce(t.escrowTxHash, "") AS escrowTxHash,
  CASE WHEN coalesce(t.escrowStatus, "") <> "" THEN t.escrowStatus WHEN coalesce(t.escrowTxHash, "") <> "" THEN "verified" ELSE "" END AS escrowStatus,
  coalesce(t.escrowVerifiedAt, "") AS escrowVerifiedAt,
  coalesce(t.escrowFrom, "") AS escrowFrom,
  coalesce(t.escrowTo, "") AS escrowTo,
  coalesce(t.escrowAmount, t.amount, 0.0) AS escrowAmount,
  coalesce(t.escrowError, "") AS escrowError,
  coalesce(t.settlementStatus, "") AS settlementStatus,
  coalesce(t.settlementOutcome, "") AS settlementOutcome,
  coalesce(t.settlementPayout, 0.0) AS settlementPayout,
  coalesce(t.payoutStatus, "") AS payoutStatus,
  coalesce(t.payoutError, "") AS payoutError,
  coalesce(t.payoutTransactionIds, []) AS payoutTransactionIds,
  coalesce(t.settledAt, "") AS settledAt,
  t.createdAt AS createdAt
`, writeParams)
		if err != nil {
			return nil, err
		}
		if tradeRows.Next(ctx) {
			return tradeFromRecord(tradeRows.Record()), nil
		}
		if err := tradeRows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("poll not found or not tradeable")
	})
	if err != nil {
		return Trade{}, err
	}
	return result.(Trade), nil
}

func (s *MemgraphUserStore) QuoteTrade(ctx context.Context, input TradeInput) (TradeQuote, error) {
	side := strings.ToLower(strings.TrimSpace(input.Side))
	if err := validateTradeInput(input.PollID, side, input.Amount); err != nil {
		return TradeQuote{}, err
	}

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, tradeablePollStateCypher(), map[string]any{"pollID": input.PollID})
		if err != nil {
			return nil, err
		}
		if !rows.Next(ctx) {
			if err := rows.Err(); err != nil {
				return nil, err
			}
			return nil, errors.New("poll not found or not tradeable")
		}
		state := marketMakerStateFromRecord(rows.Record())
		if err := rows.Err(); err != nil {
			return nil, err
		}
		quote, _, _, err := quoteMarketMaker(state, side, roundMoney(input.Amount))
		if err != nil {
			return nil, err
		}
		return quote, nil
	})
	if err != nil {
		return TradeQuote{}, err
	}
	return result.(TradeQuote), nil
}

func (s *MemgraphUserStore) QuoteCashout(ctx context.Context, userID string, input CashoutInput) (CashoutQuote, error) {
	side := strings.ToLower(strings.TrimSpace(input.Side))
	if side != "yes" && side != "no" {
		return CashoutQuote{}, errors.New("side must be yes or no")
	}
	if strings.TrimSpace(input.PollID) == "" {
		return CashoutQuote{}, errors.New("poll is required")
	}

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		state, shares, amount, _, err := cashoutState(ctx, tx, userID, input.PollID, side, input.Amount)
		if err != nil {
			return nil, err
		}
		quote, _, _, err := quoteCashoutMarketMaker(state, side, shares)
		if err != nil {
			return nil, err
		}
		quote.Amount = amount
		return quote, nil
	})
	if err != nil {
		return CashoutQuote{}, err
	}
	return result.(CashoutQuote), nil
}

func (s *MemgraphUserStore) CashoutPosition(ctx context.Context, userID string, input CashoutInput, transactionIDs []string, payoutStatus string, payoutError string) (CashoutQuote, error) {
	side := strings.ToLower(strings.TrimSpace(input.Side))
	if side != "yes" && side != "no" {
		return CashoutQuote{}, errors.New("side must be yes or no")
	}
	if strings.TrimSpace(input.PollID) == "" {
		return CashoutQuote{}, errors.New("poll is required")
	}
	if payoutStatus == "" {
		payoutStatus = "submitted"
	}

	now := time.Now().UTC().Format(time.RFC3339)
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		state, shares, amount, totalShares, err := cashoutState(ctx, tx, userID, input.PollID, side, input.Amount)
		if err != nil {
			return nil, err
		}
		quote, newYesShares, newNoShares, err := quoteCashoutMarketMaker(state, side, shares)
		if err != nil {
			return nil, err
		}
		quote.Amount = amount
		fullCashout := shares >= totalShares*0.999999
		ratio := 1.0
		if !fullCashout && totalShares > 0 {
			ratio = shares / totalShares
		}

		_, err = tx.Run(ctx, `
MATCH (u:User {id: $userID})-[:PLACED_TRADE]->(t:Trade {status: "open", side: $side})-[:ON_POLL]->(p:Poll {id: $pollID})
WITH u, p, collect(t) AS trades, sum(coalesce(t.shares, 0.0)) AS totalShares
SET
  p.yesShares = $newYesShares,
  p.noShares = $newNoShares,
  p.yesPercent = $newYesPercent,
  p.noPercent = $newNoPercent,
  p.updatedAt = $now
FOREACH (trade IN trades |
  SET trade.status = CASE WHEN $fullCashout THEN "cashed_out" ELSE "open" END,
  trade.cashoutPayout = CASE WHEN $fullCashout AND totalShares <> 0 THEN coalesce(trade.shares, 0.0) / totalShares * $proceeds ELSE coalesce(trade.cashoutPayout, 0.0) END,
  trade.settlementPayout = CASE WHEN $fullCashout AND totalShares <> 0 THEN coalesce(trade.shares, 0.0) / totalShares * $proceeds ELSE coalesce(trade.settlementPayout, 0.0) END,
  trade.payoutStatus = CASE WHEN $fullCashout THEN $payoutStatus ELSE coalesce(trade.payoutStatus, "") END,
  trade.payoutTransactionIds = CASE WHEN $fullCashout THEN $transactionIDs ELSE coalesce(trade.payoutTransactionIds, []) END,
  trade.payoutError = CASE WHEN $fullCashout THEN $payoutError ELSE coalesce(trade.payoutError, "") END,
  trade.settledAt = CASE WHEN $fullCashout THEN $now ELSE coalesce(trade.settledAt, "") END,
  trade.amount = CASE WHEN $fullCashout THEN coalesce(trade.amount, 0.0) ELSE coalesce(trade.amount, 0.0) * (1.0 - $cashoutRatio) END,
  trade.shares = CASE WHEN $fullCashout THEN coalesce(trade.shares, 0.0) ELSE coalesce(trade.shares, 0.0) * (1.0 - $cashoutRatio) END,
  trade.potentialPayout = CASE WHEN $fullCashout THEN coalesce(trade.potentialPayout, 0.0) ELSE coalesce(trade.shares, 0.0) * (1.0 - $cashoutRatio) END,
  trade.updatedAt = $now
)
FOREACH (_ IN CASE WHEN $fullCashout THEN [] ELSE [1] END |
  CREATE (cashout:Trade {
    id: $cashoutID,
    userId: $userID,
    pollId: $pollID,
    side: $side,
    outcomeLabel: $outcomeLabel,
    priceCents: $currentPriceCents,
    amount: $amount,
    shares: $shares,
    potentialPayout: $shares,
    status: "cashed_out",
    settlementPayout: $proceeds,
    cashoutPayout: $proceeds,
    payoutStatus: $payoutStatus,
    payoutTransactionIds: $transactionIDs,
    payoutError: $payoutError,
    settledAt: $now,
    createdAt: $now,
    updatedAt: $now
  })
  MERGE (u)-[:PLACED_TRADE]->(cashout)
  MERGE (cashout)-[:ON_POLL]->(p)
)
`, map[string]any{
			"userID":            userID,
			"pollID":            input.PollID,
			"side":              side,
			"cashoutID":         uuid.NewString(),
			"amount":            amount,
			"shares":            shares,
			"cashoutRatio":      ratio,
			"fullCashout":       fullCashout,
			"outcomeLabel":      quote.OutcomeLabel,
			"currentPriceCents": quote.CurrentPriceCents,
			"newYesShares":      newYesShares,
			"newNoShares":       newNoShares,
			"newYesPercent":     quote.NewYesPercent,
			"newNoPercent":      quote.NewNoPercent,
			"proceeds":          quote.Proceeds,
			"payoutStatus":      payoutStatus,
			"transactionIDs":    transactionIDs,
			"payoutError":       payoutError,
			"now":               now,
		})
		if err != nil {
			return nil, err
		}
		return quote, nil
	})
	if err != nil {
		return CashoutQuote{}, err
	}
	return result.(CashoutQuote), nil
}

func validateTradeInput(pollID string, side string, amount float64) error {
	if side != "yes" && side != "no" {
		return errors.New("side must be yes or no")
	}
	if strings.TrimSpace(pollID) == "" {
		return errors.New("poll is required")
	}
	if amount <= 0 || math.IsNaN(amount) || math.IsInf(amount, 0) {
		return errors.New("amount must be greater than zero")
	}
	return nil
}

func tradeablePollStateCypher() string {
	return `
MATCH (p:Poll {id: $pollID})-[:CLASSIFIED_AS]->(c:PollClassification)
WHERE p.status = "published" AND p.visibility = "public"
RETURN
  p.id AS pollId,
  p.slug AS pollSlug,
  p.title AS pollTitle,
  coalesce(p.outcomeA, "Yes") AS outcomeA,
  coalesce(p.outcomeB, "No") AS outcomeB,
  coalesce(p.yesPercent, 50) AS yesPercent,
  coalesce(p.yesShares, 0.0) AS yesShares,
  coalesce(p.noShares, 0.0) AS noShares,
  coalesce(p.liquidity, 5000.0) AS liquidity,
  p.yesShares IS NOT NULL AS marketStateExists
`
}

func marketMakerStateFromRecord(record *neo4j.Record) marketMakerState {
	state := marketMakerState{
		PollID:            stringValue(record, "pollId"),
		PollSlug:          stringValue(record, "pollSlug"),
		PollTitle:         stringValue(record, "pollTitle"),
		OutcomeA:          stringValue(record, "outcomeA"),
		OutcomeB:          stringValue(record, "outcomeB"),
		YesPercent:        clamp(intValue(record, "yesPercent"), 1, 99),
		YesShares:         floatValue(record, "yesShares"),
		NoShares:          floatValue(record, "noShares"),
		Liquidity:         defaultFloat(floatValue(record, "liquidity"), defaultMarketLiquidity),
		MarketStateExists: boolValue(record, "marketStateExists"),
	}
	if !state.MarketStateExists {
		state.YesShares, state.NoShares = initialMarketShares(state.YesPercent, state.Liquidity)
	}
	return state
}

func quoteMarketMaker(state marketMakerState, side string, amount float64) (TradeQuote, float64, float64, error) {
	spotPrice := lmsrPrice(state.YesShares, state.NoShares, state.Liquidity, side)
	shares := sharesForBudget(state.YesShares, state.NoShares, state.Liquidity, side, amount)
	if shares <= 0 || math.IsNaN(shares) || math.IsInf(shares, 0) {
		return TradeQuote{}, 0, 0, errors.New("unable to quote this order")
	}

	newYesShares := state.YesShares
	newNoShares := state.NoShares
	if side == "yes" {
		newYesShares += shares
	} else {
		newNoShares += shares
	}

	newYesPrice := lmsrPrice(newYesShares, newNoShares, state.Liquidity, "yes")
	if newYesPrice < minMarketPrice || newYesPrice > maxMarketPrice {
		return TradeQuote{}, 0, 0, errors.New("order would push the market outside 1c-99c; use a smaller amount")
	}

	newSidePrice := lmsrPrice(newYesShares, newNoShares, state.Liquidity, side)
	averagePrice := amount / shares
	if averagePrice <= 0 || math.IsNaN(averagePrice) || math.IsInf(averagePrice, 0) {
		return TradeQuote{}, 0, 0, errors.New("unable to quote this order")
	}

	outcomeLabel := state.OutcomeA
	if side == "no" {
		outcomeLabel = state.OutcomeB
	}

	return TradeQuote{
		PollID:            state.PollID,
		Side:              side,
		OutcomeLabel:      outcomeLabel,
		Amount:            roundMoney(amount),
		SpotPriceCents:    priceToCents(spotPrice),
		AveragePriceCents: priceToCents(averagePrice),
		Shares:            roundMoney(shares),
		PotentialPayout:   roundMoney(shares),
		NewYesPercent:     priceToCents(newYesPrice),
		NewNoPercent:      100 - priceToCents(newYesPrice),
		PriceImpactCents:  priceToCents(newSidePrice) - priceToCents(spotPrice),
		Liquidity:         roundMoney(state.Liquidity),
	}, roundMoney(newYesShares), roundMoney(newNoShares), nil
}

func cashoutState(ctx context.Context, tx neo4j.ManagedTransaction, userID string, pollID string, side string, requestedAmount float64) (marketMakerState, float64, float64, float64, error) {
	rows, err := tx.Run(ctx, `
MATCH (u:User {id: $userID})
MATCH (p:Poll {id: $pollID})-[:CLASSIFIED_AS]->(:PollClassification)
WHERE p.status = "published" AND p.visibility = "public" AND coalesce(p.tradingFrozen, false) = false
OPTIONAL MATCH (u)-[:PLACED_TRADE]->(t:Trade {status: "open", side: $side})-[:ON_POLL]->(p)
RETURN
  p.id AS pollId,
  p.slug AS pollSlug,
  p.title AS pollTitle,
  coalesce(p.outcomeA, "Yes") AS outcomeA,
  coalesce(p.outcomeB, "No") AS outcomeB,
  coalesce(p.yesPercent, 50) AS yesPercent,
  coalesce(p.yesShares, 0.0) AS yesShares,
  coalesce(p.noShares, 0.0) AS noShares,
  coalesce(p.liquidity, 5000.0) AS liquidity,
  p.yesShares IS NOT NULL AS marketStateExists,
  sum(coalesce(t.shares, 0.0)) AS positionShares,
  sum(coalesce(t.amount, 0.0)) AS positionAmount
`, map[string]any{"userID": userID, "pollID": pollID, "side": side})
	if err != nil {
		return marketMakerState{}, 0, 0, 0, err
	}
	if !rows.Next(ctx) {
		if err := rows.Err(); err != nil {
			return marketMakerState{}, 0, 0, 0, err
		}
		return marketMakerState{}, 0, 0, 0, errors.New("poll not found or not cashout eligible")
	}
	record := rows.Record()
	state := marketMakerStateFromRecord(record)
	totalShares := roundMoney(floatValue(record, "positionShares"))
	totalAmount := roundMoney(floatValue(record, "positionAmount"))
	if err := rows.Err(); err != nil {
		return marketMakerState{}, 0, 0, 0, err
	}
	if totalShares <= 0 || totalAmount <= 0 {
		return marketMakerState{}, 0, 0, 0, errors.New("no open position to cash out")
	}
	amount := totalAmount
	shares := totalShares
	if requestedAmount > 0 && requestedAmount < totalAmount {
		amount = roundMoney(requestedAmount)
		shares = roundMoney(totalShares * (amount / totalAmount))
	}
	return state, shares, amount, totalShares, nil
}

func quoteCashoutMarketMaker(state marketMakerState, side string, shares float64) (CashoutQuote, float64, float64, error) {
	if shares <= 0 || math.IsNaN(shares) || math.IsInf(shares, 0) {
		return CashoutQuote{}, 0, 0, errors.New("no open shares to cash out")
	}

	spotPrice := lmsrPrice(state.YesShares, state.NoShares, state.Liquidity, side)
	currentCost := lmsrCost(state.YesShares, state.NoShares, state.Liquidity)
	newYesShares := state.YesShares
	newNoShares := state.NoShares
	if side == "yes" {
		newYesShares = math.Max(0, state.YesShares-shares)
	} else {
		newNoShares = math.Max(0, state.NoShares-shares)
	}
	newCost := lmsrCost(newYesShares, newNoShares, state.Liquidity)
	proceeds := roundMoney(currentCost - newCost)
	if proceeds <= 0 || math.IsNaN(proceeds) || math.IsInf(proceeds, 0) {
		return CashoutQuote{}, 0, 0, errors.New("cashout has no available payout")
	}

	newYesPrice := lmsrPrice(newYesShares, newNoShares, state.Liquidity, "yes")
	newSidePrice := lmsrPrice(newYesShares, newNoShares, state.Liquidity, side)
	outcomeLabel := state.OutcomeA
	if side == "no" {
		outcomeLabel = state.OutcomeB
	}

	return CashoutQuote{
		PollID:            state.PollID,
		PollSlug:          state.PollSlug,
		PollTitle:         state.PollTitle,
		Side:              side,
		OutcomeLabel:      outcomeLabel,
		Shares:            roundMoney(shares),
		Proceeds:          proceeds,
		CurrentPriceCents: priceToCents(spotPrice),
		NewYesPercent:     priceToCents(newYesPrice),
		NewNoPercent:      100 - priceToCents(newYesPrice),
		PriceImpactCents:  priceToCents(newSidePrice) - priceToCents(spotPrice),
	}, roundMoney(newYesShares), roundMoney(newNoShares), nil
}

func lmsrPrice(yesShares float64, noShares float64, liquidity float64, side string) float64 {
	liquidity = defaultFloat(liquidity, defaultMarketLiquidity)
	yesExp := math.Exp((yesShares - math.Max(yesShares, noShares)) / liquidity)
	noExp := math.Exp((noShares - math.Max(yesShares, noShares)) / liquidity)
	yesPrice := yesExp / (yesExp + noExp)
	if side == "no" {
		return 1 - yesPrice
	}
	return yesPrice
}

func lmsrCost(yesShares float64, noShares float64, liquidity float64) float64 {
	liquidity = defaultFloat(liquidity, defaultMarketLiquidity)
	yes := yesShares / liquidity
	no := noShares / liquidity
	maxValue := math.Max(yes, no)
	return liquidity * (maxValue + math.Log(math.Exp(yes-maxValue)+math.Exp(no-maxValue)))
}

func sharesForBudget(yesShares float64, noShares float64, liquidity float64, side string, amount float64) float64 {
	currentCost := lmsrCost(yesShares, noShares, liquidity)
	low := 0.0
	high := math.Max(amount/minMarketPrice, amount+liquidity)
	for lmsrOrderCost(yesShares, noShares, liquidity, side, high, currentCost) < amount {
		high *= 2
		if high > 1_000_000_000 {
			break
		}
	}
	for i := 0; i < 80; i++ {
		mid := (low + high) / 2
		if lmsrOrderCost(yesShares, noShares, liquidity, side, mid, currentCost) < amount {
			low = mid
		} else {
			high = mid
		}
	}
	return high
}

func lmsrOrderCost(yesShares float64, noShares float64, liquidity float64, side string, shares float64, currentCost float64) float64 {
	if side == "yes" {
		return lmsrCost(yesShares+shares, noShares, liquidity) - currentCost
	}
	return lmsrCost(yesShares, noShares+shares, liquidity) - currentCost
}

func priceToCents(price float64) int64 {
	price = clampFloat(price, minMarketPrice, maxMarketPrice)
	return clamp(int64(math.Round(price*100)), 1, 99)
}

func (s *MemgraphUserStore) UserPortfolio(ctx context.Context, userID string) (Portfolio, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		positionRows, err := tx.Run(ctx, `
MATCH (:User {id: $userID})-[:PLACED_TRADE]->(t:Trade {status: "open"})-[:ON_POLL]->(p:Poll)-[:CLASSIFIED_AS]->(c:PollClassification)
WITH p, c, t.side AS side, t.outcomeLabel AS outcomeLabel,
  sum(coalesce(t.amount, 0.0)) AS amount,
  sum(coalesce(t.shares, 0.0)) AS shares,
  count(t) AS tradeCount,
  max(t.createdAt) AS lastTradeAt
OPTIONAL MATCH (:User {id: $userID})-[:PLACED_TRADE]->(closed:Trade)-[:ON_POLL]->(p)
WHERE closed.side = side AND closed.status IN ["won", "lost", "cancelled", "cashed_out"]
WITH p, c, side, outcomeLabel, amount, shares, tradeCount, lastTradeAt,
  CASE WHEN side = "yes" THEN coalesce(p.yesPercent, 50) ELSE coalesce(p.noPercent, 50) END AS currentPrice,
  sum(coalesce(closed.settlementPayout, 0.0) - coalesce(closed.amount, 0.0)) AS realizedPnl
WITH p, c, side, outcomeLabel, amount, shares, tradeCount, lastTradeAt, currentPrice, coalesce(realizedPnl, 0.0) AS realizedPnl,
  shares * (toFloat(currentPrice) / 100.0) AS currentValue
RETURN
  p.id AS pollId,
  p.slug AS pollSlug,
  p.title AS pollTitle,
  c.displayName AS category,
  coalesce(p.region, "") AS region,
  side AS side,
  outcomeLabel AS outcomeLabel,
  amount AS amount,
  shares AS shares,
  CASE WHEN shares = 0 THEN 0.0 ELSE amount / shares * 100.0 END AS averagePrice,
  currentPrice AS currentPrice,
  currentValue AS currentValue,
  shares AS potentialPayout,
  currentValue - amount AS unrealizedPnl,
  realizedPnl AS realizedPnl,
  tradeCount AS tradeCount,
  lastTradeAt AS lastTradeAt
ORDER BY lastTradeAt DESC
`, map[string]any{"userID": userID})
		if err != nil {
			return nil, err
		}
		positions := []Position{}
		for positionRows.Next(ctx) {
			positions = append(positions, positionFromRecord(positionRows.Record()))
		}
		if err := positionRows.Err(); err != nil {
			return nil, err
		}

		tradeRows, err := tx.Run(ctx, `
MATCH (u:User {id: $userID})-[:PLACED_TRADE]->(t:Trade)-[:ON_POLL]->(p:Poll)
RETURN
  t.id AS id,
  t.userId AS userId,
  coalesce(u.walletAddress, "") AS userWalletAddress,
  p.id AS pollId,
  p.slug AS pollSlug,
  p.title AS pollTitle,
  t.side AS side,
  t.outcomeLabel AS outcomeLabel,
  t.priceCents AS priceCents,
  t.amount AS amount,
  t.shares AS shares,
  t.potentialPayout AS potentialPayout,
  t.status AS status,
  coalesce(t.escrowTxHash, "") AS escrowTxHash,
  CASE WHEN coalesce(t.escrowStatus, "") <> "" THEN t.escrowStatus WHEN coalesce(t.escrowTxHash, "") <> "" THEN "verified" ELSE "" END AS escrowStatus,
  coalesce(t.escrowVerifiedAt, "") AS escrowVerifiedAt,
  coalesce(t.escrowFrom, "") AS escrowFrom,
  coalesce(t.escrowTo, "") AS escrowTo,
  coalesce(t.escrowAmount, t.amount, 0.0) AS escrowAmount,
  coalesce(t.escrowError, "") AS escrowError,
  coalesce(t.settlementStatus, "") AS settlementStatus,
  coalesce(t.settlementOutcome, "") AS settlementOutcome,
  coalesce(t.settlementPayout, 0.0) AS settlementPayout,
  coalesce(t.payoutStatus, "") AS payoutStatus,
  coalesce(t.payoutError, "") AS payoutError,
  coalesce(t.payoutTransactionIds, []) AS payoutTransactionIds,
  coalesce(t.settledAt, "") AS settledAt,
  t.createdAt AS createdAt
ORDER BY t.createdAt DESC
LIMIT 40
`, map[string]any{"userID": userID})
		if err != nil {
			return nil, err
		}
		trades := []Trade{}
		for tradeRows.Next(ctx) {
			trades = append(trades, tradeFromRecord(tradeRows.Record()))
		}
		if err := tradeRows.Err(); err != nil {
			return nil, err
		}

		pnlRows, err := tx.Run(ctx, `
MATCH (:User {id: $userID})-[:PLACED_TRADE]->(t:Trade)
WHERE t.status IN ["won", "lost", "cancelled", "cashed_out"]
RETURN sum(coalesce(t.settlementPayout, 0.0) - coalesce(t.amount, 0.0)) AS settledPnl
`, map[string]any{"userID": userID})
		if err != nil {
			return nil, err
		}
		settledPnl := 0.0
		if pnlRows.Next(ctx) {
			settledPnl = floatValue(pnlRows.Record(), "settledPnl")
		}
		if err := pnlRows.Err(); err != nil {
			return nil, err
		}

		summary := PortfolioSummary{OpenPositions: int64(len(positions))}
		for _, position := range positions {
			summary.TotalExposure += position.Amount
			summary.CurrentValue += position.CurrentValue
			summary.PotentialPayout += position.PotentialPayout
			summary.UnrealizedPnl += position.UnrealizedPnl
		}
		summary.TotalExposure = roundMoney(summary.TotalExposure)
		summary.CurrentValue = roundMoney(summary.CurrentValue)
		summary.PotentialPayout = roundMoney(summary.PotentialPayout)
		summary.SettledPnl = roundMoney(settledPnl)
		summary.RealizedPnl = summary.SettledPnl
		summary.UnrealizedPnl = roundMoney(summary.UnrealizedPnl)
		summary.NetPnl = roundMoney(summary.RealizedPnl + summary.UnrealizedPnl)
		return Portfolio{Summary: summary, Positions: positions, Trades: trades}, nil
	})
	if err != nil {
		return Portfolio{}, err
	}
	return result.(Portfolio), nil
}

func (s *MemgraphUserStore) ListTrades(ctx context.Context, userID string, pollID string, limit int64) ([]Trade, error) {
	if limit <= 0 || limit > 300 {
		limit = 100
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (u:User)-[:PLACED_TRADE]->(t:Trade)-[:ON_POLL]->(p:Poll)
WHERE ($userID = "" OR u.id = $userID)
  AND ($pollID = "" OR p.id = $pollID)
RETURN
  t.id AS id,
  t.userId AS userId,
  coalesce(u.walletAddress, "") AS userWalletAddress,
  p.id AS pollId,
  p.slug AS pollSlug,
  p.title AS pollTitle,
  t.side AS side,
  t.outcomeLabel AS outcomeLabel,
  t.priceCents AS priceCents,
  t.amount AS amount,
  t.shares AS shares,
  t.potentialPayout AS potentialPayout,
  t.status AS status,
  coalesce(t.escrowTxHash, "") AS escrowTxHash,
  CASE WHEN coalesce(t.escrowStatus, "") <> "" THEN t.escrowStatus WHEN coalesce(t.escrowTxHash, "") <> "" THEN "verified" ELSE "" END AS escrowStatus,
  coalesce(t.escrowVerifiedAt, "") AS escrowVerifiedAt,
  coalesce(t.escrowFrom, "") AS escrowFrom,
  coalesce(t.escrowTo, "") AS escrowTo,
  coalesce(t.escrowAmount, t.amount, 0.0) AS escrowAmount,
  coalesce(t.escrowError, "") AS escrowError,
  coalesce(t.settlementStatus, "") AS settlementStatus,
  coalesce(t.settlementOutcome, "") AS settlementOutcome,
  coalesce(t.settlementPayout, 0.0) AS settlementPayout,
  coalesce(t.payoutStatus, "") AS payoutStatus,
  coalesce(t.payoutError, "") AS payoutError,
  coalesce(t.payoutTransactionIds, []) AS payoutTransactionIds,
  coalesce(t.settledAt, "") AS settledAt,
  t.createdAt AS createdAt
ORDER BY t.createdAt DESC
LIMIT $limit
`, map[string]any{"userID": userID, "pollID": pollID, "limit": limit})
		if err != nil {
			return nil, err
		}
		trades := []Trade{}
		for rows.Next(ctx) {
			trades = append(trades, tradeFromRecord(rows.Record()))
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return trades, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]Trade), nil
}

func (s *MemgraphUserStore) GetTradeByID(ctx context.Context, id string) (Trade, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (u:User)-[:PLACED_TRADE]->(t:Trade {id: $id})-[:ON_POLL]->(p:Poll)
RETURN
  t.id AS id,
  t.userId AS userId,
  coalesce(u.walletAddress, "") AS userWalletAddress,
  p.id AS pollId,
  p.slug AS pollSlug,
  p.title AS pollTitle,
  t.side AS side,
  t.outcomeLabel AS outcomeLabel,
  t.priceCents AS priceCents,
  t.amount AS amount,
  t.shares AS shares,
  t.potentialPayout AS potentialPayout,
  t.status AS status,
  coalesce(t.escrowTxHash, "") AS escrowTxHash,
  CASE WHEN coalesce(t.escrowStatus, "") <> "" THEN t.escrowStatus WHEN coalesce(t.escrowTxHash, "") <> "" THEN "verified" ELSE "" END AS escrowStatus,
  coalesce(t.escrowVerifiedAt, "") AS escrowVerifiedAt,
  coalesce(t.escrowFrom, "") AS escrowFrom,
  coalesce(t.escrowTo, "") AS escrowTo,
  coalesce(t.escrowAmount, t.amount, 0.0) AS escrowAmount,
  coalesce(t.escrowError, "") AS escrowError,
  coalesce(t.settlementStatus, "") AS settlementStatus,
  coalesce(t.settlementOutcome, "") AS settlementOutcome,
  coalesce(t.settlementPayout, 0.0) AS settlementPayout,
  coalesce(t.payoutStatus, "") AS payoutStatus,
  coalesce(t.payoutError, "") AS payoutError,
  coalesce(t.payoutTransactionIds, []) AS payoutTransactionIds,
  coalesce(t.settledAt, "") AS settledAt,
  t.createdAt AS createdAt
`, map[string]any{"id": id})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return tradeFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("trade not found")
	})
	if err != nil {
		return Trade{}, err
	}
	return result.(Trade), nil
}

func (s *MemgraphUserStore) UpdateTradeEscrowReconciliation(ctx context.Context, id string, status string, errorMessage string, verifiedAt string, from string, to string, amount float64) (Trade, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	if verifiedAt == "" && status == "verified" {
		verifiedAt = now
	}

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		_, err := tx.Run(ctx, `
MATCH (t:Trade {id: $id})
SET
  t.escrowStatus = $status,
  t.escrowError = $errorMessage,
  t.escrowVerifiedAt = CASE WHEN $verifiedAt <> "" THEN $verifiedAt ELSE coalesce(t.escrowVerifiedAt, "") END,
  t.escrowFrom = CASE WHEN $from <> "" THEN $from ELSE coalesce(t.escrowFrom, "") END,
  t.escrowTo = CASE WHEN $to <> "" THEN $to ELSE coalesce(t.escrowTo, "") END,
  t.escrowAmount = CASE WHEN $amount > 0 THEN $amount ELSE coalesce(t.escrowAmount, t.amount, 0.0) END,
  t.reconciledAt = $now,
  t.updatedAt = $now
`, map[string]any{
			"id":           id,
			"status":       defaultString(status, "unknown"),
			"errorMessage": strings.TrimSpace(errorMessage),
			"verifiedAt":   verifiedAt,
			"from":         strings.ToLower(strings.TrimSpace(from)),
			"to":           strings.ToLower(strings.TrimSpace(to)),
			"amount":       roundMoney(amount),
			"now":          now,
		})
		return nil, err
	})
	if err != nil {
		return Trade{}, err
	}
	return s.GetTradeByID(ctx, id)
}

func (s *MemgraphUserStore) UpdateSettlementPayoutReconciliation(ctx context.Context, pollID string, payoutStatus string, payoutError string, transactionHash string) (Poll, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	payoutStatus = defaultString(payoutStatus, "submitted")
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		_, err := tx.Run(ctx, `
MATCH (p:Poll {id: $pollID})
OPTIONAL MATCH (t:Trade)-[:ON_POLL]->(p)
WITH p, [trade IN collect(t) WHERE trade IS NOT NULL AND trade.status IN ["won", "cancelled", "cashed_out"] AND coalesce(trade.settlementPayout, 0.0) > 0] AS trades
SET
  p.settlementPayoutStatus = $payoutStatus,
  p.settlementPayoutError = $payoutError,
  p.settlementPayoutTransactionHash = $transactionHash,
  p.payoutReconciledAt = $now,
  p.updatedAt = $now
FOREACH (trade IN trades |
  SET trade.payoutStatus = $payoutStatus,
  trade.payoutError = $payoutError,
  trade.payoutTransactionHash = $transactionHash,
  trade.payoutReconciledAt = $now,
  trade.updatedAt = $now
)
`, map[string]any{
			"pollID":          pollID,
			"payoutStatus":    payoutStatus,
			"payoutError":     strings.TrimSpace(payoutError),
			"transactionHash": strings.TrimSpace(transactionHash),
			"now":             now,
		})
		return nil, err
	})
	if err != nil {
		return Poll{}, err
	}
	return s.pollByID(ctx, pollID)
}

func (s *MemgraphUserStore) MarketActivity(ctx context.Context, slug string) ([]MarketActivity, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (u:User)-[:PLACED_TRADE]->(t:Trade)-[:ON_POLL]->(p:Poll {slug: $slug})
RETURN
  t.id AS id,
  CASE
    WHEN t.status = "cashed_out" THEN "cashout"
    WHEN t.status IN ["won", "lost", "cancelled"] THEN "settlement"
    ELSE "buy"
  END AS kind,
  coalesce(t.status, "open") AS status,
  coalesce(u.walletAddress, "") AS actor,
  coalesce(t.side, "") AS side,
  coalesce(t.outcomeLabel, "") AS outcomeLabel,
  coalesce(t.amount, 0.0) AS amount,
  coalesce(t.settlementPayout, 0.0) AS payout,
  coalesce(t.priceCents, 0) AS priceCents,
  coalesce(t.priceImpactCents, 0) AS priceImpactCents,
  CASE WHEN coalesce(t.settledAt, "") <> "" THEN t.settledAt ELSE t.createdAt END AS createdAt
ORDER BY createdAt DESC
LIMIT 40
`, map[string]any{"slug": slugify(slug)})
		if err != nil {
			return nil, err
		}
		activities := []MarketActivity{}
		for rows.Next(ctx) {
			activities = append(activities, marketActivityFromRecord(rows.Record()))
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return activities, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]MarketActivity), nil
}

func (s *MemgraphUserStore) MarketStats(ctx context.Context, slug string) (MarketStats, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (p:Poll {slug: $slug})
OPTIONAL MATCH (u:User)-[:PLACED_TRADE]->(t:Trade {status: "open"})-[:ON_POLL]->(p)
WITH p,
  count(t) AS tradeCount,
  count(DISTINCT u) AS holderCount,
  count(DISTINCT CASE WHEN t.side = "yes" THEN u ELSE null END) AS yesHolderCount,
  count(DISTINCT CASE WHEN t.side = "no" THEN u ELSE null END) AS noHolderCount,
  sum(coalesce(t.amount, 0.0)) AS openInterest
RETURN
  p.id AS pollId,
  p.slug AS pollSlug,
  coalesce(p.yesShares, 0.0) AS yesShares,
  coalesce(p.noShares, 0.0) AS noShares,
  coalesce(p.liquidity, 5000.0) AS liquidity,
  tradeCount AS tradeCount,
  holderCount AS holderCount,
  yesHolderCount AS yesHolderCount,
  noHolderCount AS noHolderCount,
  coalesce(openInterest, 0.0) AS openInterest,
  coalesce(p.marketMakerCollected, 0.0) AS marketMakerCollected
`, map[string]any{"slug": slugify(slug)})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return marketStatsFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("market not found")
	})
	if err != nil {
		return MarketStats{}, err
	}
	return result.(MarketStats), nil
}

func (s *MemgraphUserStore) ListMarketComments(ctx context.Context, slug string) ([]MarketComment, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (u:User)-[:POSTED_COMMENT]->(m:MarketComment)-[:ON_POLL]->(p:Poll {slug: $slug})
WHERE p.status = "published" AND p.visibility = "public"
  AND coalesce(m.status, "visible") = "visible"
RETURN
  m.id AS id,
  u.id AS userId,
  p.id AS pollId,
  p.slug AS pollSlug,
  coalesce(u.walletAddress, "") AS actor,
  coalesce(m.body, "") AS body,
  coalesce(m.status, "visible") AS status,
  coalesce(m.reportCount, 0) AS reportCount,
  coalesce(m.latestReportReason, "") AS latestReportReason,
  coalesce(m.latestReportedAt, "") AS latestReportedAt,
  m.createdAt AS createdAt
ORDER BY m.createdAt DESC
LIMIT 80
`, map[string]any{"slug": slugify(slug)})
		if err != nil {
			return nil, err
		}
		comments := []MarketComment{}
		for rows.Next(ctx) {
			comments = append(comments, marketCommentFromRecord(rows.Record()))
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return comments, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]MarketComment), nil
}

func (s *MemgraphUserStore) CreateMarketComment(ctx context.Context, user User, slug string, body string) (MarketComment, error) {
	body = strings.Join(strings.Fields(strings.TrimSpace(body)), " ")
	if body == "" {
		return MarketComment{}, errors.New("comment cannot be empty")
	}
	if len(body) > 240 {
		return MarketComment{}, errors.New("comment must be 240 characters or fewer")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	params := map[string]any{
		"id":     uuid.NewString(),
		"userID": user.ID,
		"slug":   slugify(slug),
		"body":   body,
		"now":    now,
	}

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (u:User {id: $userID})
MATCH (p:Poll {slug: $slug})
WHERE p.status = "published" AND p.visibility = "public" AND coalesce(p.commentsDisabled, false) = false
CREATE (m:MarketComment {
  id: $id,
  userId: $userID,
  pollId: p.id,
  pollSlug: p.slug,
  body: $body,
  status: "visible",
  createdAt: $now,
  updatedAt: $now
})
MERGE (u)-[:POSTED_COMMENT]->(m)
MERGE (m)-[:ON_POLL]->(p)
RETURN
  m.id AS id,
  u.id AS userId,
  p.id AS pollId,
  p.slug AS pollSlug,
  coalesce(u.walletAddress, "") AS actor,
  m.body AS body,
  coalesce(m.status, "visible") AS status,
  coalesce(m.reportCount, 0) AS reportCount,
  coalesce(m.latestReportReason, "") AS latestReportReason,
  coalesce(m.latestReportedAt, "") AS latestReportedAt,
  m.createdAt AS createdAt
`, params)
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return marketCommentFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("market not found")
	})
	if err != nil {
		return MarketComment{}, err
	}
	return result.(MarketComment), nil
}

func (s *MemgraphUserStore) ListAllMarketComments(ctx context.Context) ([]MarketComment, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (u:User)-[:POSTED_COMMENT]->(m:MarketComment)-[:ON_POLL]->(p:Poll)
RETURN
  m.id AS id,
  u.id AS userId,
  p.id AS pollId,
  p.slug AS pollSlug,
  coalesce(u.walletAddress, "") AS actor,
  coalesce(m.body, "") AS body,
  coalesce(m.status, "visible") AS status,
  coalesce(m.reportCount, 0) AS reportCount,
  coalesce(m.latestReportReason, "") AS latestReportReason,
  coalesce(m.latestReportedAt, "") AS latestReportedAt,
  m.createdAt AS createdAt
ORDER BY m.createdAt DESC
LIMIT 160
`, nil)
		if err != nil {
			return nil, err
		}
		comments := []MarketComment{}
		for rows.Next(ctx) {
			comments = append(comments, marketCommentFromRecord(rows.Record()))
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return comments, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]MarketComment), nil
}

func (s *MemgraphUserStore) UpdateMarketCommentStatus(ctx context.Context, id string, status string) (MarketComment, error) {
	status = strings.ToLower(strings.TrimSpace(status))
	if status != "visible" && status != "hidden" && status != "deleted" {
		return MarketComment{}, errors.New("comment status must be visible, hidden, or deleted")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (u:User)-[:POSTED_COMMENT]->(m:MarketComment {id: $id})-[:ON_POLL]->(p:Poll)
SET m.status = $status,
  m.updatedAt = $now
RETURN
  m.id AS id,
  u.id AS userId,
  p.id AS pollId,
  p.slug AS pollSlug,
  coalesce(u.walletAddress, "") AS actor,
  coalesce(m.body, "") AS body,
  coalesce(m.status, "visible") AS status,
  coalesce(m.reportCount, 0) AS reportCount,
  coalesce(m.latestReportReason, "") AS latestReportReason,
  coalesce(m.latestReportedAt, "") AS latestReportedAt,
  m.createdAt AS createdAt
`, map[string]any{"id": id, "status": status, "now": now})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return marketCommentFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("comment not found")
	})
	if err != nil {
		return MarketComment{}, err
	}
	return result.(MarketComment), nil
}

func (s *MemgraphUserStore) ReportMarketComment(ctx context.Context, user User, id string, reason string) (MarketComment, error) {
	reason = strings.Join(strings.Fields(strings.TrimSpace(reason)), " ")
	if reason == "" {
		reason = "Reported by user"
	}
	if len(reason) > 180 {
		return MarketComment{}, errors.New("report reason must be 180 characters or fewer")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	params := map[string]any{
		"id":       id,
		"reportID": uuid.NewString(),
		"userID":   user.ID,
		"reason":   reason,
		"now":      now,
	}

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (reporter:User {id: $userID})
MATCH (u:User)-[:POSTED_COMMENT]->(m:MarketComment {id: $id})-[:ON_POLL]->(p:Poll)
CREATE (r:CommentReport {
  id: $reportID,
  userId: $userID,
  commentId: $id,
  reason: $reason,
  createdAt: $now
})
MERGE (reporter)-[:REPORTED_COMMENT]->(r)
MERGE (r)-[:REPORTS_COMMENT]->(m)
SET m.reportCount = coalesce(m.reportCount, 0) + 1,
  m.latestReportReason = $reason,
  m.latestReportedAt = $now,
  m.updatedAt = $now
RETURN
  m.id AS id,
  u.id AS userId,
  p.id AS pollId,
  p.slug AS pollSlug,
  coalesce(u.walletAddress, "") AS actor,
  coalesce(m.body, "") AS body,
  coalesce(m.status, "visible") AS status,
  coalesce(m.reportCount, 0) AS reportCount,
  coalesce(m.latestReportReason, "") AS latestReportReason,
  coalesce(m.latestReportedAt, "") AS latestReportedAt,
  m.createdAt AS createdAt
`, params)
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return marketCommentFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("comment not found")
	})
	if err != nil {
		return MarketComment{}, err
	}
	return result.(MarketComment), nil
}

func (s *MemgraphUserStore) PreviewPollSettlement(ctx context.Context, pollID string, status string, outcome string) (SettlementPreview, error) {
	status, outcome, winnerSide, err := normalizeSettlementInput(status, outcome)
	if err != nil {
		return SettlementPreview{}, err
	}

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, settlementTradeRowsCypher(), map[string]any{"pollID": pollID})
		if err != nil {
			return nil, err
		}
		preview := SettlementPreview{Status: status, Outcome: outcome, WinnerSide: winnerSide}
		recipients := map[string]*SettlementRecipient{}
		found := false
		for rows.Next(ctx) {
			found = true
			addSettlementTradeRow(&preview, recipients, rows.Record(), status, winnerSide)
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		if !found {
			return nil, errors.New("poll not found")
		}
		preview.Recipients = settlementRecipientList(recipients)
		preview.RecipientCount = int64(len(preview.Recipients))
		preview.WinningShares = roundMoney(preview.WinningShares)
		preview.PayoutRequired = roundMoney(preview.PayoutRequired)
		return preview, nil
	})
	if err != nil {
		return SettlementPreview{}, err
	}
	return result.(SettlementPreview), nil
}

func (s *MemgraphUserStore) SettlePollTrades(ctx context.Context, pollID string, status string, outcome string, transactionIDs []string, payoutStatus string, payoutError string) (SettlementPreview, error) {
	status, outcome, winnerSide, err := normalizeSettlementInput(status, outcome)
	if err != nil {
		return SettlementPreview{}, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if payoutStatus == "" {
		payoutStatus = "none"
	}

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		previewRows, err := tx.Run(ctx, settlementTradeRowsCypher(), map[string]any{"pollID": pollID})
		if err != nil {
			return nil, err
		}
		preview := SettlementPreview{Status: status, Outcome: outcome, WinnerSide: winnerSide}
		recipients := map[string]*SettlementRecipient{}
		found := false
		for previewRows.Next(ctx) {
			found = true
			addSettlementTradeRow(&preview, recipients, previewRows.Record(), status, winnerSide)
		}
		if err := previewRows.Err(); err != nil {
			return nil, err
		}
		if !found {
			return nil, errors.New("poll not found")
		}
		preview.Recipients = settlementRecipientList(recipients)
		preview.RecipientCount = int64(len(preview.Recipients))
		preview.WinningShares = roundMoney(preview.WinningShares)
		preview.PayoutRequired = roundMoney(preview.PayoutRequired)

		_, err = tx.Run(ctx, `
MATCH (p:Poll {id: $pollID})
OPTIONAL MATCH (u:User)-[:PLACED_TRADE]->(t:Trade {status: "open"})-[:ON_POLL]->(p)
WITH p, u, t,
  CASE
    WHEN t IS NULL THEN false
    WHEN $status = "cancelled" THEN true
    WHEN $status = "resolved" AND t.side = $winnerSide THEN true
    ELSE false
  END AS shouldPay,
  CASE
    WHEN t IS NULL THEN 0.0
    WHEN $status = "cancelled" THEN coalesce(t.amount, 0.0)
    WHEN $status = "resolved" AND t.side = $winnerSide THEN coalesce(t.potentialPayout, 0.0)
    ELSE 0.0
  END AS payoutAmount
FOREACH (_ IN CASE WHEN t IS NULL THEN [] ELSE [1] END |
  SET t.status = CASE
    WHEN $status = "cancelled" THEN "cancelled"
    WHEN shouldPay THEN "won"
    ELSE "lost"
  END,
  t.settlementStatus = $status,
  t.settlementOutcome = $outcome,
  t.settlementPayout = payoutAmount,
  t.payoutStatus = CASE
    WHEN payoutAmount > 0 THEN $payoutStatus
    ELSE "none"
  END,
  t.payoutTransactionIds = CASE
    WHEN payoutAmount > 0 THEN $transactionIDs
    ELSE []
  END,
  t.payoutError = CASE
    WHEN payoutAmount > 0 THEN $payoutError
    ELSE ""
  END,
  t.settledAt = $now,
  t.updatedAt = $now
)
SET p.settlementStatus = $status,
  p.settlementPayoutStatus = $payoutStatus,
  p.settlementPayoutTransactionIds = $transactionIDs,
  p.settlementPayoutError = $payoutError,
  p.settledAt = $now,
  p.updatedAt = $now
`, map[string]any{
			"pollID":         pollID,
			"status":         status,
			"outcome":        outcome,
			"winnerSide":     winnerSide,
			"transactionIDs": transactionIDs,
			"payoutStatus":   payoutStatus,
			"payoutError":    payoutError,
			"now":            now,
		})
		if err != nil {
			return nil, err
		}
		if preview.PayoutRequired > 0 {
			_, err = tx.Run(ctx, createPayoutAttemptCypher(), map[string]any{
				"id":             uuid.NewString(),
				"pollID":         pollID,
				"attemptType":    "initial",
				"transactionIDs": transactionIDs,
				"payoutStatus":   payoutStatus,
				"payoutError":    payoutError,
				"payoutRequired": preview.PayoutRequired,
				"recipientCount": preview.RecipientCount,
				"actor":          "settlement",
				"now":            now,
			})
			if err != nil {
				return nil, err
			}
		}
		return preview, nil
	})
	if err != nil {
		return SettlementPreview{}, err
	}
	return result.(SettlementPreview), nil
}

func (s *MemgraphUserStore) SettlementPayoutRetryPreview(ctx context.Context, pollID string) (SettlementPreview, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, settlementRetryTradeRowsCypher(), map[string]any{"pollID": pollID})
		if err != nil {
			return nil, err
		}
		preview := SettlementPreview{}
		recipients := map[string]*SettlementRecipient{}
		found := false
		for rows.Next(ctx) {
			found = true
			addSettlementRetryTradeRow(&preview, recipients, rows.Record())
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		if !found {
			return nil, errors.New("poll not found")
		}
		preview.Recipients = settlementRecipientList(recipients)
		preview.RecipientCount = int64(len(preview.Recipients))
		preview.WinningShares = roundMoney(preview.WinningShares)
		preview.PayoutRequired = roundMoney(preview.PayoutRequired)
		return preview, nil
	})
	if err != nil {
		return SettlementPreview{}, err
	}
	return result.(SettlementPreview), nil
}

func (s *MemgraphUserStore) RecordSettlementPayoutRetry(ctx context.Context, pollID string, transactionIDs []string, payoutStatus string, payoutError string) (SettlementPreview, error) {
	if payoutStatus == "" {
		payoutStatus = "submitted"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, settlementRetryTradeRowsCypher(), map[string]any{"pollID": pollID})
		if err != nil {
			return nil, err
		}
		preview := SettlementPreview{}
		recipients := map[string]*SettlementRecipient{}
		found := false
		for rows.Next(ctx) {
			found = true
			addSettlementRetryTradeRow(&preview, recipients, rows.Record())
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		if !found {
			return nil, errors.New("poll not found")
		}
		preview.Recipients = settlementRecipientList(recipients)
		preview.RecipientCount = int64(len(preview.Recipients))
		preview.WinningShares = roundMoney(preview.WinningShares)
		preview.PayoutRequired = roundMoney(preview.PayoutRequired)

		_, err = tx.Run(ctx, `
MATCH (p:Poll {id: $pollID})
OPTIONAL MATCH (t:Trade)-[:ON_POLL]->(p)
WHERE t.status IN ["won", "cancelled", "cashed_out"]
  AND coalesce(t.settlementPayout, 0.0) > 0
SET t.payoutStatus = $payoutStatus,
  t.payoutTransactionIds = $transactionIDs,
  t.payoutError = $payoutError,
  t.updatedAt = $now
SET p.settlementPayoutStatus = $payoutStatus,
  p.settlementPayoutTransactionIds = $transactionIDs,
  p.settlementPayoutError = $payoutError,
  p.updatedAt = $now
`, map[string]any{
			"pollID":         pollID,
			"transactionIDs": transactionIDs,
			"payoutStatus":   payoutStatus,
			"payoutError":    payoutError,
			"now":            now,
		})
		if err != nil {
			return nil, err
		}
		if preview.PayoutRequired > 0 {
			_, err = tx.Run(ctx, createPayoutAttemptCypher(), map[string]any{
				"id":             uuid.NewString(),
				"pollID":         pollID,
				"attemptType":    "retry",
				"transactionIDs": transactionIDs,
				"payoutStatus":   payoutStatus,
				"payoutError":    payoutError,
				"payoutRequired": preview.PayoutRequired,
				"recipientCount": preview.RecipientCount,
				"actor":          "admin",
				"now":            now,
			})
			if err != nil {
				return nil, err
			}
		}
		return preview, nil
	})
	if err != nil {
		return SettlementPreview{}, err
	}
	return result.(SettlementPreview), nil
}

func (s *MemgraphUserStore) ListSettlementPayoutAttempts(ctx context.Context, pollID string) ([]SettlementPayoutAttempt, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (a:SettlementPayoutAttempt)-[:PAYOUT_FOR]->(:Poll {id: $pollID})
RETURN
  a.id AS id,
  a.pollId AS pollId,
  coalesce(a.attemptType, "retry") AS attemptType,
  coalesce(a.payoutStatus, "") AS payoutStatus,
  coalesce(a.payoutError, "") AS payoutError,
  coalesce(a.transactionIds, []) AS transactionIds,
  coalesce(a.payoutRequired, 0.0) AS payoutRequired,
  coalesce(a.recipientCount, 0) AS recipientCount,
  coalesce(a.actor, "") AS actor,
  a.createdAt AS createdAt
ORDER BY a.createdAt DESC
LIMIT 80
`, map[string]any{"pollID": pollID})
		if err != nil {
			return nil, err
		}
		attempts := []SettlementPayoutAttempt{}
		for rows.Next(ctx) {
			attempts = append(attempts, settlementPayoutAttemptFromRecord(rows.Record()))
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return attempts, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]SettlementPayoutAttempt), nil
}

func tradeFromRecord(record *neo4j.Record) Trade {
	return Trade{
		ID:                   stringValue(record, "id"),
		UserID:               stringValue(record, "userId"),
		UserWalletAddress:    stringValue(record, "userWalletAddress"),
		PollID:               stringValue(record, "pollId"),
		PollSlug:             stringValue(record, "pollSlug"),
		PollTitle:            stringValue(record, "pollTitle"),
		Side:                 stringValue(record, "side"),
		OutcomeLabel:         stringValue(record, "outcomeLabel"),
		PriceCents:           intValue(record, "priceCents"),
		Amount:               roundMoney(floatValue(record, "amount")),
		Shares:               roundMoney(floatValue(record, "shares")),
		PotentialPayout:      roundMoney(floatValue(record, "potentialPayout")),
		Status:               stringValue(record, "status"),
		EscrowTxHash:         stringValue(record, "escrowTxHash"),
		EscrowStatus:         stringValue(record, "escrowStatus"),
		EscrowVerifiedAt:     stringValue(record, "escrowVerifiedAt"),
		EscrowFrom:           stringValue(record, "escrowFrom"),
		EscrowTo:             stringValue(record, "escrowTo"),
		EscrowAmount:         roundMoney(floatValue(record, "escrowAmount")),
		EscrowError:          stringValue(record, "escrowError"),
		SettlementStatus:     stringValue(record, "settlementStatus"),
		SettlementOutcome:    stringValue(record, "settlementOutcome"),
		SettlementPayout:     roundMoney(floatValue(record, "settlementPayout")),
		PayoutStatus:         stringValue(record, "payoutStatus"),
		PayoutError:          stringValue(record, "payoutError"),
		PayoutTransactionIDs: stringSliceValue(record, "payoutTransactionIds"),
		SettledAt:            stringValue(record, "settledAt"),
		CreatedAt:            stringValue(record, "createdAt"),
	}
}

func positionFromRecord(record *neo4j.Record) Position {
	return Position{
		PollID:          stringValue(record, "pollId"),
		PollSlug:        stringValue(record, "pollSlug"),
		PollTitle:       stringValue(record, "pollTitle"),
		Category:        stringValue(record, "category"),
		Region:          stringValue(record, "region"),
		Side:            stringValue(record, "side"),
		OutcomeLabel:    stringValue(record, "outcomeLabel"),
		Amount:          roundMoney(floatValue(record, "amount")),
		Shares:          roundMoney(floatValue(record, "shares")),
		AveragePrice:    roundMoney(floatValue(record, "averagePrice")),
		CurrentPrice:    intValue(record, "currentPrice"),
		CurrentValue:    roundMoney(floatValue(record, "currentValue")),
		PotentialPayout: roundMoney(floatValue(record, "potentialPayout")),
		UnrealizedPnl:   roundMoney(floatValue(record, "unrealizedPnl")),
		RealizedPnl:     roundMoney(floatValue(record, "realizedPnl")),
		TradeCount:      intValue(record, "tradeCount"),
		LastTradeAt:     stringValue(record, "lastTradeAt"),
	}
}

func marketActivityFromRecord(record *neo4j.Record) MarketActivity {
	return MarketActivity{
		ID:               stringValue(record, "id"),
		Kind:             stringValue(record, "kind"),
		Status:           stringValue(record, "status"),
		Actor:            shortWalletLabel(stringValue(record, "actor")),
		Side:             stringValue(record, "side"),
		OutcomeLabel:     stringValue(record, "outcomeLabel"),
		Amount:           roundMoney(floatValue(record, "amount")),
		Payout:           roundMoney(floatValue(record, "payout")),
		PriceCents:       intValue(record, "priceCents"),
		PriceImpactCents: intValue(record, "priceImpactCents"),
		CreatedAt:        stringValue(record, "createdAt"),
	}
}

func marketStatsFromRecord(record *neo4j.Record) MarketStats {
	return MarketStats{
		PollID:               stringValue(record, "pollId"),
		PollSlug:             stringValue(record, "pollSlug"),
		YesShares:            roundMoney(floatValue(record, "yesShares")),
		NoShares:             roundMoney(floatValue(record, "noShares")),
		Liquidity:            roundMoney(floatValue(record, "liquidity")),
		TradeCount:           intValue(record, "tradeCount"),
		HolderCount:          intValue(record, "holderCount"),
		YesHolderCount:       intValue(record, "yesHolderCount"),
		NoHolderCount:        intValue(record, "noHolderCount"),
		OpenInterest:         roundMoney(floatValue(record, "openInterest")),
		MarketMakerCollected: roundMoney(floatValue(record, "marketMakerCollected")),
	}
}

func marketCommentFromRecord(record *neo4j.Record) MarketComment {
	return MarketComment{
		ID:                 stringValue(record, "id"),
		UserID:             stringValue(record, "userId"),
		PollID:             stringValue(record, "pollId"),
		PollSlug:           stringValue(record, "pollSlug"),
		Actor:              shortWalletLabel(stringValue(record, "actor")),
		Body:               stringValue(record, "body"),
		Status:             stringValue(record, "status"),
		ReportCount:        intValue(record, "reportCount"),
		LatestReportReason: stringValue(record, "latestReportReason"),
		LatestReportedAt:   stringValue(record, "latestReportedAt"),
		CreatedAt:          stringValue(record, "createdAt"),
	}
}

func settlementPayoutAttemptFromRecord(record *neo4j.Record) SettlementPayoutAttempt {
	return SettlementPayoutAttempt{
		ID:             stringValue(record, "id"),
		PollID:         stringValue(record, "pollId"),
		AttemptType:    stringValue(record, "attemptType"),
		PayoutStatus:   stringValue(record, "payoutStatus"),
		PayoutError:    stringValue(record, "payoutError"),
		TransactionIDs: stringSliceValue(record, "transactionIds"),
		PayoutRequired: roundMoney(floatValue(record, "payoutRequired")),
		RecipientCount: intValue(record, "recipientCount"),
		Actor:          stringValue(record, "actor"),
		CreatedAt:      stringValue(record, "createdAt"),
	}
}

func shortWalletLabel(address string) string {
	address = strings.TrimSpace(address)
	if len(address) <= 12 {
		return "Trader"
	}
	return address[:6] + "..." + address[len(address)-4:]
}

func normalizeSettlementInput(status string, outcome string) (string, string, string, error) {
	status = strings.ToLower(strings.TrimSpace(status))
	outcome = strings.ToLower(strings.TrimSpace(outcome))
	switch status {
	case "resolved":
		if outcome != "a" && outcome != "b" {
			return "", "", "", errors.New("resolved markets require outcome a or b")
		}
		if outcome == "a" {
			return status, outcome, "yes", nil
		}
		return status, outcome, "no", nil
	case "cancelled":
		return status, "", "", nil
	default:
		return "", "", "", errors.New("settlement status must be resolved or cancelled")
	}
}

func settlementTradeRowsCypher() string {
	return `
MATCH (p:Poll {id: $pollID})
OPTIONAL MATCH (u:User)-[:PLACED_TRADE]->(t:Trade {status: "open"})-[:ON_POLL]->(p)
RETURN
  p.id AS pollId,
  p.title AS pollTitle,
  coalesce(u.id, "") AS userId,
  coalesce(u.walletAddress, "") AS walletAddress,
  coalesce(u.email, "") AS email,
  coalesce(t.id, "") AS tradeId,
  coalesce(t.side, "") AS side,
  coalesce(t.amount, 0.0) AS amount,
  coalesce(t.shares, 0.0) AS shares,
  coalesce(t.potentialPayout, 0.0) AS potentialPayout
`
}

func settlementRetryTradeRowsCypher() string {
	return `
MATCH (p:Poll {id: $pollID})
OPTIONAL MATCH (u:User)-[:PLACED_TRADE]->(t:Trade)-[:ON_POLL]->(p)
WITH p, u, t
WHERE t IS NULL OR (
  t.status IN ["won", "cancelled", "cashed_out"]
  AND coalesce(t.settlementPayout, 0.0) > 0
)
RETURN
  p.id AS pollId,
  p.title AS pollTitle,
  coalesce(p.settlementStatus, p.status, "") AS settlementStatus,
  coalesce(p.resolutionOutcome, p.settlementOutcome, "") AS settlementOutcome,
  coalesce(u.id, "") AS userId,
  coalesce(u.walletAddress, "") AS walletAddress,
  coalesce(u.email, "") AS email,
  coalesce(t.id, "") AS tradeId,
  coalesce(t.side, "") AS side,
  coalesce(t.shares, 0.0) AS shares,
  coalesce(t.settlementPayout, 0.0) AS settlementPayout
`
}

func createPayoutAttemptCypher() string {
	return `
MATCH (p:Poll {id: $pollID})
CREATE (a:SettlementPayoutAttempt {
  id: $id,
  pollId: $pollID,
  attemptType: $attemptType,
  transactionIds: $transactionIDs,
  payoutStatus: $payoutStatus,
  payoutError: $payoutError,
  payoutRequired: $payoutRequired,
  recipientCount: $recipientCount,
  actor: $actor,
  createdAt: $now
})
MERGE (a)-[:PAYOUT_FOR]->(p)
RETURN a.id AS id
`
}

func addSettlementTradeRow(preview *SettlementPreview, recipients map[string]*SettlementRecipient, record *neo4j.Record, status string, winnerSide string) {
	preview.PollID = stringValue(record, "pollId")
	preview.PollTitle = stringValue(record, "pollTitle")
	tradeID := stringValue(record, "tradeId")
	if tradeID == "" {
		return
	}

	preview.OpenTradeCount++
	side := stringValue(record, "side")
	shouldPay := status == "cancelled" || (status == "resolved" && side == winnerSide)
	if !shouldPay {
		preview.LosingTradeCount++
		return
	}

	amount := floatValue(record, "potentialPayout")
	if status == "cancelled" {
		amount = floatValue(record, "amount")
	}
	shares := floatValue(record, "shares")
	preview.WinningTradeCount++
	preview.WinningShares += shares
	preview.PayoutRequired += amount

	walletAddress := stringValue(record, "walletAddress")
	if walletAddress == "" {
		return
	}
	recipient := recipients[walletAddress]
	if recipient == nil {
		recipient = &SettlementRecipient{
			UserID:        stringValue(record, "userId"),
			WalletAddress: walletAddress,
			Email:         stringValue(record, "email"),
		}
		recipients[walletAddress] = recipient
	}
	recipient.Amount = roundMoney(recipient.Amount + amount)
	recipient.Shares = roundMoney(recipient.Shares + shares)
	recipient.TradeCount++
}

func addSettlementRetryTradeRow(preview *SettlementPreview, recipients map[string]*SettlementRecipient, record *neo4j.Record) {
	preview.PollID = stringValue(record, "pollId")
	preview.PollTitle = stringValue(record, "pollTitle")
	preview.Status = stringValue(record, "settlementStatus")
	preview.Outcome = stringValue(record, "settlementOutcome")
	tradeID := stringValue(record, "tradeId")
	if tradeID == "" {
		return
	}

	amount := floatValue(record, "settlementPayout")
	if amount <= 0 {
		return
	}
	shares := floatValue(record, "shares")
	preview.OpenTradeCount++
	preview.WinningTradeCount++
	preview.WinningShares += shares
	preview.PayoutRequired += amount

	walletAddress := stringValue(record, "walletAddress")
	if walletAddress == "" {
		return
	}
	recipient := recipients[walletAddress]
	if recipient == nil {
		recipient = &SettlementRecipient{
			UserID:        stringValue(record, "userId"),
			WalletAddress: walletAddress,
			Email:         stringValue(record, "email"),
		}
		recipients[walletAddress] = recipient
	}
	recipient.Amount = roundMoney(recipient.Amount + amount)
	recipient.Shares = roundMoney(recipient.Shares + shares)
	recipient.TradeCount++
}

func settlementRecipientList(recipients map[string]*SettlementRecipient) []SettlementRecipient {
	result := make([]SettlementRecipient, 0, len(recipients))
	for _, recipient := range recipients {
		result = append(result, *recipient)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Amount > result[j].Amount
	})
	return result
}

func floatValue(record *neo4j.Record, key string) float64 {
	value, ok := record.Get(key)
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int64:
		return float64(typed)
	case int:
		return float64(typed)
	default:
		return 0
	}
}

func roundMoney(value float64) float64 {
	return math.Round(value*100) / 100
}
