package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

type ShieldedTradeBatchInput struct {
	BatchID         string
	VaultAddress    string
	TradeAmount     float64
	FeeBps          int64
	ExecuteAfter    time.Time
	ExpiresAt       time.Time
	TransactionID   string
	TransactionHash string
}

type ShieldedTradeBatch struct {
	ID                        string  `json:"id"`
	BatchID                   string  `json:"batchId"`
	VaultAddress              string  `json:"vaultAddress"`
	TradeAmount               float64 `json:"tradeAmount"`
	FeeBps                    int64   `json:"feeBps"`
	Status                    string  `json:"status"`
	ExecuteAfter              string  `json:"executeAfter"`
	ExpiresAt                 string  `json:"expiresAt"`
	TransactionID             string  `json:"transactionId"`
	TransactionHash           string  `json:"transactionHash"`
	SettlementTransactionID   string  `json:"settlementTransactionId"`
	SettlementTransactionHash string  `json:"settlementTransactionHash"`
	Error                     string  `json:"error"`
	CreatedAt                 string  `json:"createdAt"`
	UpdatedAt                 string  `json:"updatedAt"`
}

type ShieldedTradeOrderInput struct {
	BatchID             string
	UserID              string
	PollID              string
	Side                string
	Amount              float64
	NullifierHash       string
	OrderCommitment     string
	ZKProofSubmissionID string
	TransactionID       string
	TransactionHash     string
	PrivateClaimLeaf    string
}

type ShieldedTradeOrder struct {
	ID                  string  `json:"id"`
	BatchID             string  `json:"batchId"`
	UserID              string  `json:"userId"`
	PollID              string  `json:"pollId"`
	Side                string  `json:"side"`
	Amount              float64 `json:"amount"`
	NullifierHash       string  `json:"nullifierHash"`
	OrderCommitment     string  `json:"orderCommitment"`
	ZKProofSubmissionID string  `json:"zkProofSubmissionId"`
	TransactionID       string  `json:"transactionId"`
	TransactionHash     string  `json:"transactionHash"`
	PrivateClaimLeaf    string  `json:"privateClaimLeaf"`
	TradeID             string  `json:"tradeId"`
	Status              string  `json:"status"`
	Error               string  `json:"error"`
	CreatedAt           string  `json:"createdAt"`
	UpdatedAt           string  `json:"updatedAt"`
}

func (s *MemgraphUserStore) CreateShieldedTradeBatch(ctx context.Context, input ShieldedTradeBatchInput) (ShieldedTradeBatch, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	params := map[string]any{
		"id": uuid.NewString(), "batchID": strings.ToLower(strings.TrimSpace(input.BatchID)),
		"vault": strings.ToLower(strings.TrimSpace(input.VaultAddress)), "amount": roundMoney(input.TradeAmount),
		"feeBps": input.FeeBps, "executeAfter": input.ExecuteAfter.UTC().Format(time.RFC3339),
		"expiresAt": input.ExpiresAt.UTC().Format(time.RFC3339), "transactionID": input.TransactionID,
		"transactionHash": strings.ToLower(strings.TrimSpace(input.TransactionHash)), "now": now,
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MERGE (b:ShieldedTradeBatch {batchId: $batchID})
ON CREATE SET b.id=$id, b.vaultAddress=$vault, b.tradeAmount=$amount, b.feeBps=$feeBps,
  b.status="collecting", b.executeAfter=$executeAfter, b.expiresAt=$expiresAt,
  b.transactionId=$transactionID, b.transactionHash=$transactionHash, b.error="", b.createdAt=$now, b.updatedAt=$now
RETURN b
`, params)
		if err != nil || !rows.Next(ctx) {
			if err == nil {
				err = errors.New("shielded trade batch was not created")
			}
			return nil, err
		}
		return shieldedTradeBatchFromNode(rows.Record()), rows.Err()
	})
	if err != nil {
		return ShieldedTradeBatch{}, err
	}
	return result.(ShieldedTradeBatch), nil
}

func (s *MemgraphUserStore) CurrentShieldedTradeBatch(ctx context.Context, vault string, now time.Time) (ShieldedTradeBatch, bool, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `MATCH (b:ShieldedTradeBatch {vaultAddress:$vault, status:"collecting"})
WHERE b.executeAfter > $now RETURN b ORDER BY b.executeAfter ASC LIMIT 1`, map[string]any{"vault": strings.ToLower(strings.TrimSpace(vault)), "now": now.UTC().Format(time.RFC3339)})
		if err != nil {
			return nil, err
		}
		if !rows.Next(ctx) {
			return ShieldedTradeBatch{}, nil
		}
		return shieldedTradeBatchFromNode(rows.Record()), rows.Err()
	})
	if err != nil {
		return ShieldedTradeBatch{}, false, err
	}
	batch := result.(ShieldedTradeBatch)
	return batch, batch.ID != "", nil
}

func (s *MemgraphUserStore) ListDueShieldedTradeBatches(ctx context.Context, now time.Time) ([]ShieldedTradeBatch, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `MATCH (b:ShieldedTradeBatch {status:"collecting"}) WHERE b.executeAfter <= $now RETURN b ORDER BY b.executeAfter ASC LIMIT 20`, map[string]any{"now": now.UTC().Format(time.RFC3339)})
		if err != nil {
			return nil, err
		}
		items := []ShieldedTradeBatch{}
		for rows.Next(ctx) {
			items = append(items, shieldedTradeBatchFromNode(rows.Record()))
		}
		return items, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]ShieldedTradeBatch), nil
}

func (s *MemgraphUserStore) UpdateShieldedTradeBatch(ctx context.Context, batchID, status, transactionID, transactionHash, errorMessage string) (ShieldedTradeBatch, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `MATCH (b:ShieldedTradeBatch {batchId:$batchID}) SET b.status=$status,
 b.settlementTransactionId=$transactionID, b.settlementTransactionHash=$transactionHash, b.error=$error, b.updatedAt=$now RETURN b`, map[string]any{
			"batchID": strings.ToLower(strings.TrimSpace(batchID)), "status": status, "transactionID": transactionID,
			"transactionHash": strings.ToLower(strings.TrimSpace(transactionHash)), "error": errorMessage, "now": time.Now().UTC().Format(time.RFC3339),
		})
		if err != nil || !rows.Next(ctx) {
			if err == nil {
				err = errors.New("shielded trade batch not found")
			}
			return nil, err
		}
		return shieldedTradeBatchFromNode(rows.Record()), rows.Err()
	})
	if err != nil {
		return ShieldedTradeBatch{}, err
	}
	return result.(ShieldedTradeBatch), nil
}

func (s *MemgraphUserStore) CreateShieldedTradeOrder(ctx context.Context, input ShieldedTradeOrderInput) (ShieldedTradeOrder, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (b:ShieldedTradeBatch {batchId:$batchID, status:"collecting"}), (u:User {id:$userID}), (p:Poll {id:$pollID})
OPTIONAL MATCH (existing:ShieldedTradeOrder {nullifierHash:$nullifier})
WITH b, u, p, existing
WHERE existing IS NULL
CREATE (o:ShieldedTradeOrder {id:$id, batchId:$batchID, userId:$userID, pollId:$pollID, side:$side,
 amount:$amount, nullifierHash:$nullifier, orderCommitment:$commitment, zkProofSubmissionId:$zkID,
 transactionId:$transactionID, transactionHash:$transactionHash, privateClaimLeaf:$privateClaimLeaf,
 tradeId:"", status:"accepted", error:"", createdAt:$now, updatedAt:$now})
MERGE (u)-[:PLACED_SHIELDED_ORDER]->(o) MERGE (o)-[:IN_BATCH]->(b) MERGE (o)-[:ON_POLL]->(p)
RETURN o`, map[string]any{
			"id": uuid.NewString(), "batchID": strings.ToLower(strings.TrimSpace(input.BatchID)), "userID": input.UserID,
			"pollID": input.PollID, "side": strings.ToLower(strings.TrimSpace(input.Side)), "amount": roundMoney(input.Amount),
			"nullifier": strings.TrimSpace(input.NullifierHash), "commitment": strings.TrimSpace(input.OrderCommitment),
			"zkID": input.ZKProofSubmissionID, "transactionID": input.TransactionID,
			"transactionHash": strings.ToLower(strings.TrimSpace(input.TransactionHash)), "privateClaimLeaf": strings.TrimSpace(input.PrivateClaimLeaf), "now": now,
		})
		if err != nil {
			return nil, err
		}
		if !rows.Next(ctx) {
			return nil, errors.New("batch unavailable or shielded nullifier already used")
		}
		return shieldedTradeOrderFromNode(rows.Record()), rows.Err()
	})
	if err != nil {
		return ShieldedTradeOrder{}, err
	}
	return result.(ShieldedTradeOrder), nil
}

func (s *MemgraphUserStore) ListShieldedTradeOrders(ctx context.Context, batchID string) ([]ShieldedTradeOrder, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `MATCH (o:ShieldedTradeOrder {batchId:$batchID}) RETURN o ORDER BY o.createdAt ASC`, map[string]any{"batchID": strings.ToLower(strings.TrimSpace(batchID))})
		if err != nil {
			return nil, err
		}
		items := []ShieldedTradeOrder{}
		for rows.Next(ctx) {
			items = append(items, shieldedTradeOrderFromNode(rows.Record()))
		}
		return items, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]ShieldedTradeOrder), nil
}

func (s *MemgraphUserStore) CompleteShieldedTradeOrder(ctx context.Context, id, tradeID, status, errorMessage string) error {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		_, err := tx.Run(ctx, `MATCH (o:ShieldedTradeOrder {id:$id}) SET o.tradeId=$tradeID, o.status=$status, o.error=$error, o.updatedAt=$now`, map[string]any{"id": id, "tradeID": tradeID, "status": status, "error": errorMessage, "now": time.Now().UTC().Format(time.RFC3339)})
		return nil, err
	})
	return err
}

func shieldedTradeBatchFromNode(record *neo4j.Record) ShieldedTradeBatch {
	node, _ := record.Get("b")
	props := node.(neo4j.Node).Props
	return ShieldedTradeBatch{ID: stringAny(props["id"]), BatchID: stringAny(props["batchId"]), VaultAddress: stringAny(props["vaultAddress"]), TradeAmount: floatAny(props["tradeAmount"]), FeeBps: intAny(props["feeBps"]), Status: stringAny(props["status"]), ExecuteAfter: stringAny(props["executeAfter"]), ExpiresAt: stringAny(props["expiresAt"]), TransactionID: stringAny(props["transactionId"]), TransactionHash: stringAny(props["transactionHash"]), SettlementTransactionID: stringAny(props["settlementTransactionId"]), SettlementTransactionHash: stringAny(props["settlementTransactionHash"]), Error: stringAny(props["error"]), CreatedAt: stringAny(props["createdAt"]), UpdatedAt: stringAny(props["updatedAt"])}
}

func shieldedTradeOrderFromNode(record *neo4j.Record) ShieldedTradeOrder {
	node, _ := record.Get("o")
	props := node.(neo4j.Node).Props
	return ShieldedTradeOrder{ID: stringAny(props["id"]), BatchID: stringAny(props["batchId"]), UserID: stringAny(props["userId"]), PollID: stringAny(props["pollId"]), Side: stringAny(props["side"]), Amount: floatAny(props["amount"]), NullifierHash: stringAny(props["nullifierHash"]), OrderCommitment: stringAny(props["orderCommitment"]), ZKProofSubmissionID: stringAny(props["zkProofSubmissionId"]), TransactionID: stringAny(props["transactionId"]), TransactionHash: stringAny(props["transactionHash"]), PrivateClaimLeaf: stringAny(props["privateClaimLeaf"]), TradeID: stringAny(props["tradeId"]), Status: stringAny(props["status"]), Error: stringAny(props["error"]), CreatedAt: stringAny(props["createdAt"]), UpdatedAt: stringAny(props["updatedAt"])}
}

func stringAny(value any) string {
	if value == nil {
		return ""
	}
	if result, ok := value.(string); ok {
		return result
	}
	return ""
}
func floatAny(value any) float64 {
	switch item := value.(type) {
	case float64:
		return item
	case int64:
		return float64(item)
	case int:
		return float64(item)
	}
	return 0
}
func intAny(value any) int64 {
	switch item := value.(type) {
	case int64:
		return item
	case int:
		return int64(item)
	case float64:
		return int64(item)
	}
	return 0
}
