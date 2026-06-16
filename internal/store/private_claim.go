package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

func (s *MemgraphUserStore) PrivateClaimTree(ctx context.Context, pollID string) (PrivateClaimTree, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		maxRows, err := tx.Run(ctx, `
MATCH (p:Poll {id: $pollID})
OPTIONAL MATCH (indexed:Trade)-[:ON_POLL]->(p)
WHERE coalesce(indexed.privateClaimLeaf, "") <> "" AND coalesce(indexed.privateClaimLeafIndex, -1) >= 0
RETURN max(coalesce(indexed.privateClaimLeafIndex, -1)) AS currentMax
`, map[string]any{"pollID": strings.TrimSpace(pollID)})
		if err != nil {
			return nil, err
		}
		currentMax := int64(-1)
		if maxRows.Next(ctx) {
			currentMax = int64(intValue(maxRows.Record(), "currentMax"))
		}
		if err := maxRows.Err(); err != nil {
			return nil, err
		}

		missingRows, err := tx.Run(ctx, `
MATCH (p:Poll {id: $pollID})
MATCH (missing:Trade)-[:ON_POLL]->(p)
WHERE coalesce(missing.privateClaimLeaf, "") <> "" AND coalesce(missing.privateClaimLeafIndex, -1) < 0
RETURN missing.id AS id
ORDER BY missing.createdAt ASC, missing.id ASC
`, map[string]any{"pollID": strings.TrimSpace(pollID)})
		if err != nil {
			return nil, err
		}
		nextIndex := currentMax + 1
		for missingRows.Next(ctx) {
			id := stringValue(missingRows.Record(), "id")
			if id == "" {
				continue
			}
			if _, err := tx.Run(ctx, `
MATCH (t:Trade {id: $id})
SET t.privateClaimLeafIndex = $leafIndex
`, map[string]any{"id": id, "leafIndex": nextIndex}); err != nil {
				return nil, err
			}
			nextIndex += 1
		}
		if err := missingRows.Err(); err != nil {
			return nil, err
		}

		rows, err := tx.Run(ctx, `
MATCH (p:Poll {id: $pollID})
OPTIONAL MATCH (t:Trade)-[:ON_POLL]->(p)
WHERE coalesce(t.privateClaimLeaf, "") <> ""
WITH p, t
ORDER BY coalesce(t.privateClaimLeafIndex, -1) ASC, t.createdAt ASC, t.id ASC
WITH p,
  collect(coalesce(t.privateClaimLeaf, "")) AS leaves,
  collect(coalesce(t.privateClaimLeafIndex, -1)) AS leafIndexes
RETURN
  p.id AS pollId,
  p.slug AS pollSlug,
  p.title AS pollTitle,
  leaves AS leaves,
  leafIndexes AS leafIndexes
`, map[string]any{"pollID": strings.TrimSpace(pollID)})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			record := rows.Record()
			rawLeaves := stringSliceValue(record, "leaves")
			rawIndexes := int64SliceValue(record, "leafIndexes")
			leaves := []string{}
			leafIndexes := []int64{}
			for index, leaf := range rawLeaves {
				if strings.TrimSpace(leaf) == "" {
					continue
				}
				leaves = append(leaves, leaf)
				if index < len(rawIndexes) {
					leafIndexes = append(leafIndexes, rawIndexes[index])
				}
			}
			return PrivateClaimTree{
				PollID:      stringValue(record, "pollId"),
				PollSlug:    stringValue(record, "pollSlug"),
				PollTitle:   stringValue(record, "pollTitle"),
				Leaves:      leaves,
				LeafIndexes: leafIndexes,
				LeafCount:   int64(len(leaves)),
			}, nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("poll not found")
	})
	if err != nil {
		return PrivateClaimTree{}, err
	}
	return result.(PrivateClaimTree), nil
}

func (s *MemgraphUserStore) RecordPrivateClaimRoot(ctx context.Context, pollID string, root string, leafCount int64, reason string) (PrivateClaimRootSnapshot, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	pollID = strings.TrimSpace(pollID)
	root = strings.TrimSpace(root)
	reason = strings.TrimSpace(reason)
	if pollID == "" {
		return PrivateClaimRootSnapshot{}, errors.New("pollID is required")
	}
	if root == "" {
		return PrivateClaimRootSnapshot{}, errors.New("root is required")
	}
	if reason == "" {
		reason = "observed"
	}

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (p:Poll {id: $pollID})
MERGE (snapshot:PrivateClaimRootSnapshot {pollId: $pollID, root: $root, leafCount: $leafCount})
ON CREATE SET
  snapshot.id = $id,
  snapshot.reason = $reason,
  snapshot.createdAt = $now
SET
  snapshot.lastSeenAt = $now,
  snapshot.reason = CASE WHEN coalesce(snapshot.reason, "") = "" OR snapshot.reason = "observed" THEN $reason ELSE snapshot.reason END
MERGE (p)-[:HAS_PRIVATE_CLAIM_ROOT]->(snapshot)
RETURN
  snapshot.id AS id,
  snapshot.pollId AS pollId,
  snapshot.root AS root,
  coalesce(snapshot.leafCount, 0) AS leafCount,
  coalesce(snapshot.reason, "") AS reason,
  coalesce(snapshot.createdAt, "") AS createdAt,
  coalesce(snapshot.lastSeenAt, "") AS lastSeenAt
`, map[string]any{
			"id":        uuid.NewString(),
			"pollID":    pollID,
			"root":      root,
			"leafCount": leafCount,
			"reason":    reason,
			"now":       now,
		})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return privateClaimRootSnapshotFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("poll not found")
	})
	if err != nil {
		return PrivateClaimRootSnapshot{}, err
	}
	return result.(PrivateClaimRootSnapshot), nil
}

func (s *MemgraphUserStore) PrivateClaimRootHistory(ctx context.Context, pollID string, limit int64) ([]PrivateClaimRootSnapshot, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:Poll {id: $pollID})-[:HAS_PRIVATE_CLAIM_ROOT]->(snapshot:PrivateClaimRootSnapshot)
RETURN
  snapshot.id AS id,
  snapshot.pollId AS pollId,
  snapshot.root AS root,
  coalesce(snapshot.leafCount, 0) AS leafCount,
  coalesce(snapshot.reason, "") AS reason,
  coalesce(snapshot.createdAt, "") AS createdAt,
  coalesce(snapshot.lastSeenAt, "") AS lastSeenAt
ORDER BY snapshot.createdAt DESC, snapshot.leafCount DESC
LIMIT $limit
`, map[string]any{"pollID": strings.TrimSpace(pollID), "limit": limit})
		if err != nil {
			return nil, err
		}
		history := []PrivateClaimRootSnapshot{}
		for rows.Next(ctx) {
			history = append(history, privateClaimRootSnapshotFromRecord(rows.Record()))
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return history, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]PrivateClaimRootSnapshot), nil
}

func privateClaimRootSnapshotFromRecord(record *neo4j.Record) PrivateClaimRootSnapshot {
	return PrivateClaimRootSnapshot{
		ID:         stringValue(record, "id"),
		PollID:     stringValue(record, "pollId"),
		Root:       stringValue(record, "root"),
		LeafCount:  intValue(record, "leafCount"),
		Reason:     stringValue(record, "reason"),
		CreatedAt:  stringValue(record, "createdAt"),
		LastSeenAt: stringValue(record, "lastSeenAt"),
	}
}

func (s *MemgraphUserStore) ReservePrivateClaim(ctx context.Context, userID string, tradeID string, leaf string, root string, nullifierHash string, zkProofSubmissionID string) (PrivateClaim, Trade, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, privateClaimReturnQuery(`
MATCH (u:User {id: $userID})-[:PLACED_TRADE]->(t:Trade {id: $tradeID})-[:ON_POLL]->(p:Poll)
OPTIONAL MATCH (existing:PrivateClaim {nullifierHash: $nullifierHash})
WITH u, t, p, existing
WHERE existing IS NULL
  AND coalesce(t.privateClaimLeaf, "") = $leaf
  AND t.status IN ["won", "cancelled"]
  AND coalesce(t.settlementPayout, 0.0) > 0.0
  AND coalesce(t.payoutStatus, "") IN ["", "none", "claimable", "claim_failed", "failed"]
CREATE (claim:PrivateClaim {
  id: $claimID,
  userId: $userID,
  tradeId: $tradeID,
  pollId: p.id,
  pollSlug: p.slug,
  walletAddress: coalesce(u.walletAddress, ""),
  leaf: $leaf,
  root: $root,
  nullifierHash: $nullifierHash,
  zkProofSubmissionId: $zkProofSubmissionID,
  registryTransactionId: "",
  registryStatus: "",
  registryError: "",
  amount: coalesce(t.settlementPayout, 0.0),
  status: "reserved",
  payoutStatus: "pending",
  payoutError: "",
  transactionIds: [],
  createdAt: $now,
  updatedAt: $now
})
SET t.payoutStatus = "claim_pending",
  t.privateClaimRoot = $root,
  t.privateClaimNullifierHash = $nullifierHash,
  t.privateClaimId = $claimID,
  t.updatedAt = $now
MERGE (t)-[:HAS_PRIVATE_CLAIM]->(claim)
`), map[string]any{
			"claimID":             uuid.NewString(),
			"userID":              strings.TrimSpace(userID),
			"tradeID":             strings.TrimSpace(tradeID),
			"leaf":                strings.TrimSpace(leaf),
			"root":                strings.TrimSpace(root),
			"nullifierHash":       strings.TrimSpace(nullifierHash),
			"zkProofSubmissionID": strings.TrimSpace(zkProofSubmissionID),
			"now":                 now,
		})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			record := rows.Record()
			return privateClaimPayload{Claim: privateClaimFromRecord(record), Trade: tradeFromRecord(record)}, nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("claim is not eligible or nullifier was already used")
	})
	if err != nil {
		return PrivateClaim{}, Trade{}, err
	}
	payload := result.(privateClaimPayload)
	return payload.Claim, payload.Trade, nil
}

func (s *MemgraphUserStore) RecordPrivateClaimRegistryTransaction(ctx context.Context, userID string, claimID string, transactionID string, registryStatus string, registryError string) (PrivateClaim, Trade, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	registryStatus = strings.TrimSpace(registryStatus)
	if registryStatus == "" {
		registryStatus = "submitted"
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, privateClaimReturnQuery(`
MATCH (u:User {id: $userID})-[:PLACED_TRADE]->(t:Trade)-[:HAS_PRIVATE_CLAIM]->(claim:PrivateClaim {id: $claimID})
MATCH (t)-[:ON_POLL]->(p:Poll)
SET claim.registryTransactionId = $transactionID,
  claim.registryStatus = $registryStatus,
  claim.registryError = $registryError,
  claim.updatedAt = $now,
  t.updatedAt = $now
`), map[string]any{
			"userID":         strings.TrimSpace(userID),
			"claimID":        strings.TrimSpace(claimID),
			"transactionID":  strings.TrimSpace(transactionID),
			"registryStatus": registryStatus,
			"registryError":  strings.TrimSpace(registryError),
			"now":            now,
		})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			record := rows.Record()
			return privateClaimPayload{Claim: privateClaimFromRecord(record), Trade: tradeFromRecord(record)}, nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("private claim not found")
	})
	if err != nil {
		return PrivateClaim{}, Trade{}, err
	}
	payload := result.(privateClaimPayload)
	return payload.Claim, payload.Trade, nil
}

func (s *MemgraphUserStore) CompletePrivateClaimPayout(ctx context.Context, userID string, claimID string, transactionIDs []string, payoutStatus string, payoutError string) (PrivateClaim, Trade, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	if payoutStatus == "" {
		payoutStatus = "submitted"
	}
	tradePayoutStatus := payoutStatus
	if payoutStatus == "failed" {
		tradePayoutStatus = "claim_failed"
	}

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, privateClaimReturnQuery(`
MATCH (u:User {id: $userID})-[:PLACED_TRADE]->(t:Trade)-[:HAS_PRIVATE_CLAIM]->(claim:PrivateClaim {id: $claimID})
MATCH (t)-[:ON_POLL]->(p:Poll)
SET claim.status = CASE WHEN $payoutStatus = "failed" THEN "failed" ELSE "submitted" END,
  claim.payoutStatus = $payoutStatus,
  claim.payoutError = $payoutError,
  claim.transactionIds = $transactionIDs,
  claim.updatedAt = $now,
  t.payoutStatus = $tradePayoutStatus,
  t.payoutError = $payoutError,
  t.payoutTransactionIds = $transactionIDs,
  t.updatedAt = $now
`), map[string]any{
			"userID":            strings.TrimSpace(userID),
			"claimID":           strings.TrimSpace(claimID),
			"transactionIDs":    transactionIDs,
			"payoutStatus":      strings.TrimSpace(payoutStatus),
			"tradePayoutStatus": tradePayoutStatus,
			"payoutError":       strings.TrimSpace(payoutError),
			"now":               now,
		})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			record := rows.Record()
			return privateClaimPayload{Claim: privateClaimFromRecord(record), Trade: tradeFromRecord(record)}, nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("private claim not found")
	})
	if err != nil {
		return PrivateClaim{}, Trade{}, err
	}
	payload := result.(privateClaimPayload)
	return payload.Claim, payload.Trade, nil
}

type privateClaimPayload struct {
	Claim PrivateClaim
	Trade Trade
}

func privateClaimReturnQuery(prefix string) string {
	return prefix + `
RETURN
  claim.id AS claimId,
  claim.userId AS claimUserId,
  claim.tradeId AS claimTradeId,
  claim.pollId AS claimPollId,
  claim.pollSlug AS claimPollSlug,
  p.title AS claimPollTitle,
  claim.walletAddress AS claimWalletAddress,
  claim.leaf AS claimLeaf,
  claim.root AS claimRoot,
  claim.nullifierHash AS claimNullifierHash,
  coalesce(claim.zkProofSubmissionId, "") AS claimZKProofSubmissionId,
  coalesce(claim.registryTransactionId, "") AS claimRegistryTransactionId,
  coalesce(claim.registryStatus, "") AS claimRegistryStatus,
  coalesce(claim.registryError, "") AS claimRegistryError,
  coalesce(claim.amount, 0.0) AS claimAmount,
  coalesce(claim.status, "") AS claimStatus,
  coalesce(claim.payoutStatus, "") AS claimPayoutStatus,
  coalesce(claim.payoutError, "") AS claimPayoutError,
  coalesce(claim.transactionIds, []) AS claimTransactionIds,
  claim.createdAt AS claimCreatedAt,
  claim.updatedAt AS claimUpdatedAt,
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
  coalesce(t.privateClaimLeaf, "") AS privateClaimLeaf,
  coalesce(t.privateClaimLeafIndex, -1) AS privateClaimLeafIndex,
  coalesce(t.privateClaimRoot, "") AS privateClaimRoot,
  coalesce(t.privateClaimNullifierHash, "") AS privateClaimNullifierHash,
  coalesce(t.privateClaimId, "") AS privateClaimId,
  coalesce(t.settledAt, "") AS settledAt,
  t.createdAt AS createdAt
`
}

func privateClaimFromRecord(record *neo4j.Record) PrivateClaim {
	return PrivateClaim{
		ID:                    stringValue(record, "claimId"),
		UserID:                stringValue(record, "claimUserId"),
		TradeID:               stringValue(record, "claimTradeId"),
		PollID:                stringValue(record, "claimPollId"),
		PollSlug:              stringValue(record, "claimPollSlug"),
		PollTitle:             stringValue(record, "claimPollTitle"),
		WalletAddress:         stringValue(record, "claimWalletAddress"),
		Leaf:                  stringValue(record, "claimLeaf"),
		Root:                  stringValue(record, "claimRoot"),
		NullifierHash:         stringValue(record, "claimNullifierHash"),
		ZKProofSubmissionID:   stringValue(record, "claimZKProofSubmissionId"),
		RegistryTransactionID: stringValue(record, "claimRegistryTransactionId"),
		RegistryStatus:        stringValue(record, "claimRegistryStatus"),
		RegistryError:         stringValue(record, "claimRegistryError"),
		Amount:                roundMoney(floatValue(record, "claimAmount")),
		Status:                stringValue(record, "claimStatus"),
		PayoutStatus:          stringValue(record, "claimPayoutStatus"),
		PayoutError:           stringValue(record, "claimPayoutError"),
		TransactionIDs:        stringSliceValue(record, "claimTransactionIds"),
		CreatedAt:             stringValue(record, "claimCreatedAt"),
		UpdatedAt:             stringValue(record, "claimUpdatedAt"),
	}
}

func (s *MemgraphUserStore) CreateShieldedWithdrawal(ctx context.Context, userID string, input ShieldedWithdrawalInput) (ShieldedWithdrawal, error) {
	now := time.Now().UTC()
	executeAfter := input.ExecuteAfter.UTC()
	if executeAfter.IsZero() {
		executeAfter = now
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, shieldedWithdrawalReturnQuery(`
MATCH (u:User {id: $userID})
OPTIONAL MATCH (existing:ShieldedWithdrawal {nullifierHash: $nullifierHash})
WITH u, existing
WHERE existing IS NULL
CREATE (w:ShieldedWithdrawal {
  id: $id,
  userId: $userID,
  denomination: $denomination,
  mode: $mode,
  noteCommitment: $noteCommitment,
  nullifierHash: $nullifierHash,
  poolAddress: $poolAddress,
  publicSignals: $publicSignals,
  recipient: $recipient,
  relayer: $relayer,
  relayerFee: $relayerFee,
  solidityProof: $solidityProof,
  zkProofSubmissionId: $zkProofSubmissionID,
  zkProofSubmissionRef: $zkProofSubmissionRef,
  status: "queued",
  error: "",
  transactionId: "",
  transactionHash: "",
  executeAfter: $executeAfter,
  attempts: 0,
  createdAt: $now,
  updatedAt: $now
})
MERGE (u)-[:REQUESTED_SHIELDED_WITHDRAWAL]->(w)
`), map[string]any{
			"id":                   uuid.NewString(),
			"userID":               strings.TrimSpace(userID),
			"denomination":         strings.TrimSpace(input.Denomination),
			"mode":                 strings.TrimSpace(input.Mode),
			"noteCommitment":       strings.ToLower(strings.TrimSpace(input.NoteCommitment)),
			"nullifierHash":        strings.ToLower(strings.TrimSpace(input.NullifierHash)),
			"poolAddress":          strings.ToLower(strings.TrimSpace(input.PoolAddress)),
			"publicSignals":        strings.TrimSpace(input.PublicSignals),
			"recipient":            strings.ToLower(strings.TrimSpace(input.Recipient)),
			"relayer":              strings.ToLower(strings.TrimSpace(input.Relayer)),
			"relayerFee":           strings.TrimSpace(input.RelayerFee),
			"solidityProof":        strings.TrimSpace(input.SolidityProof),
			"zkProofSubmissionID":  strings.TrimSpace(input.ZKProofSubmissionID),
			"zkProofSubmissionRef": strings.TrimSpace(input.ZKProofSubmissionRef),
			"executeAfter":         executeAfter.Format(time.RFC3339),
			"now":                  now.Format(time.RFC3339),
		})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return shieldedWithdrawalFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("shielded withdrawal was already requested for this nullifier")
	})
	if err != nil {
		return ShieldedWithdrawal{}, err
	}
	return result.(ShieldedWithdrawal), nil
}

func (s *MemgraphUserStore) ListUserShieldedWithdrawals(ctx context.Context, userID string, limit int64) ([]ShieldedWithdrawal, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, shieldedWithdrawalReturnQuery(`
MATCH (:User {id: $userID})-[:REQUESTED_SHIELDED_WITHDRAWAL]->(w:ShieldedWithdrawal)
WITH w
ORDER BY w.createdAt DESC
LIMIT $limit
`), map[string]any{
			"userID": strings.TrimSpace(userID),
			"limit":  limit,
		})
		if err != nil {
			return nil, err
		}
		withdrawals := []ShieldedWithdrawal{}
		for rows.Next(ctx) {
			withdrawals = append(withdrawals, shieldedWithdrawalFromRecord(rows.Record()))
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return withdrawals, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]ShieldedWithdrawal), nil
}

func (s *MemgraphUserStore) ListShieldedWithdrawals(ctx context.Context, status string, limit int64) ([]ShieldedWithdrawal, error) {
	if limit <= 0 || limit > 250 {
		limit = 100
	}
	status = strings.ToLower(strings.TrimSpace(status))
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, shieldedWithdrawalReturnQuery(`
MATCH (w:ShieldedWithdrawal)
OPTIONAL MATCH (u:User {id: w.userId})
WITH w, u
WHERE $status = "" OR coalesce(w.status, "") = $status
ORDER BY w.createdAt DESC
LIMIT $limit
`), map[string]any{
			"status": status,
			"limit":  limit,
		})
		if err != nil {
			return nil, err
		}
		withdrawals := []ShieldedWithdrawal{}
		for rows.Next(ctx) {
			withdrawals = append(withdrawals, shieldedWithdrawalFromRecord(rows.Record()))
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return withdrawals, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]ShieldedWithdrawal), nil
}

func (s *MemgraphUserStore) ListDueShieldedWithdrawals(ctx context.Context, limit int64, now time.Time) ([]ShieldedWithdrawal, error) {
	if limit <= 0 || limit > 50 {
		limit = 5
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, shieldedWithdrawalReturnQuery(`
MATCH (w:ShieldedWithdrawal)
WHERE coalesce(w.status, "") IN ["queued", "retry"]
  AND coalesce(w.executeAfter, "") <= $now
WITH w
ORDER BY w.executeAfter ASC, w.createdAt ASC
LIMIT $limit
`), map[string]any{
			"now":   now.UTC().Format(time.RFC3339),
			"limit": limit,
		})
		if err != nil {
			return nil, err
		}
		withdrawals := []ShieldedWithdrawal{}
		for rows.Next(ctx) {
			withdrawals = append(withdrawals, shieldedWithdrawalFromRecord(rows.Record()))
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return withdrawals, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]ShieldedWithdrawal), nil
}

func (s *MemgraphUserStore) GetShieldedWithdrawal(ctx context.Context, id string) (ShieldedWithdrawal, bool, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, shieldedWithdrawalReturnQuery(`
MATCH (w:ShieldedWithdrawal {id: $id})
OPTIONAL MATCH (u:User {id: w.userId})
WITH w, u
`), map[string]any{"id": strings.TrimSpace(id)})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return shieldedWithdrawalFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, nil
	})
	if err != nil {
		return ShieldedWithdrawal{}, false, err
	}
	if result == nil {
		return ShieldedWithdrawal{}, false, nil
	}
	return result.(ShieldedWithdrawal), true, nil
}

func (s *MemgraphUserStore) MarkShieldedWithdrawalProcessing(ctx context.Context, id string) (ShieldedWithdrawal, bool, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, shieldedWithdrawalReturnQuery(`
MATCH (w:ShieldedWithdrawal {id: $id})
WHERE coalesce(w.status, "") IN ["queued", "retry"]
SET w.status = "processing",
  w.attempts = coalesce(w.attempts, 0) + 1,
  w.updatedAt = $now
`), map[string]any{"id": strings.TrimSpace(id), "now": now})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return shieldedWithdrawalFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, nil
	})
	if err != nil {
		return ShieldedWithdrawal{}, false, err
	}
	if result == nil {
		return ShieldedWithdrawal{}, false, nil
	}
	return result.(ShieldedWithdrawal), true, nil
}

func (s *MemgraphUserStore) CompleteShieldedWithdrawal(ctx context.Context, id string, status string, transactionID string, transactionHash string, errorMessage string) (ShieldedWithdrawal, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	status = strings.TrimSpace(status)
	if status == "" {
		status = "submitted"
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, shieldedWithdrawalReturnQuery(`
MATCH (w:ShieldedWithdrawal {id: $id})
SET w.status = $status,
  w.transactionId = $transactionID,
  w.transactionHash = $transactionHash,
  w.error = $error,
  w.updatedAt = $now
`), map[string]any{
			"id":              strings.TrimSpace(id),
			"status":          status,
			"transactionID":   strings.TrimSpace(transactionID),
			"transactionHash": strings.TrimSpace(transactionHash),
			"error":           strings.TrimSpace(errorMessage),
			"now":             now,
		})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return shieldedWithdrawalFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("shielded withdrawal not found")
	})
	if err != nil {
		return ShieldedWithdrawal{}, err
	}
	return result.(ShieldedWithdrawal), nil
}

func (s *MemgraphUserStore) ScheduleShieldedWithdrawalRetry(ctx context.Context, id string, executeAfter time.Time, errorMessage string) (ShieldedWithdrawal, error) {
	now := time.Now().UTC()
	if executeAfter.IsZero() {
		executeAfter = now
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, shieldedWithdrawalReturnQuery(`
MATCH (w:ShieldedWithdrawal {id: $id})
SET w.status = "retry",
  w.error = $error,
  w.transactionId = "",
  w.transactionHash = "",
  w.executeAfter = $executeAfter,
  w.updatedAt = $now
`), map[string]any{
			"id":           strings.TrimSpace(id),
			"error":        strings.TrimSpace(errorMessage),
			"executeAfter": executeAfter.UTC().Format(time.RFC3339),
			"now":          now.Format(time.RFC3339),
		})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return shieldedWithdrawalFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("shielded withdrawal not found")
	})
	if err != nil {
		return ShieldedWithdrawal{}, err
	}
	return result.(ShieldedWithdrawal), nil
}

func (s *MemgraphUserStore) RetryAnyShieldedWithdrawal(ctx context.Context, id string, executeAfter time.Time) (ShieldedWithdrawal, error) {
	now := time.Now().UTC()
	if executeAfter.IsZero() {
		executeAfter = now
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, shieldedWithdrawalReturnQuery(`
MATCH (w:ShieldedWithdrawal {id: $id})
WHERE coalesce(w.status, "") IN ["failed", "suspicious", "retry", "queued"]
SET w.status = "retry",
  w.error = "",
  w.transactionId = "",
  w.transactionHash = "",
  w.executeAfter = $executeAfter,
  w.updatedAt = $now
`), map[string]any{
			"id":           strings.TrimSpace(id),
			"executeAfter": executeAfter.UTC().Format(time.RFC3339),
			"now":          now.Format(time.RFC3339),
		})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return shieldedWithdrawalFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("retryable shielded withdrawal not found")
	})
	if err != nil {
		return ShieldedWithdrawal{}, err
	}
	return result.(ShieldedWithdrawal), nil
}

func (s *MemgraphUserStore) RetryShieldedWithdrawal(ctx context.Context, userID string, id string, executeAfter time.Time) (ShieldedWithdrawal, error) {
	now := time.Now().UTC()
	if executeAfter.IsZero() {
		executeAfter = now
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, shieldedWithdrawalReturnQuery(`
MATCH (:User {id: $userID})-[:REQUESTED_SHIELDED_WITHDRAWAL]->(w:ShieldedWithdrawal {id: $id})
WHERE coalesce(w.status, "") = "failed"
SET w.status = "retry",
  w.error = "",
  w.transactionId = "",
  w.transactionHash = "",
  w.executeAfter = $executeAfter,
  w.updatedAt = $now
`), map[string]any{
			"userID":       strings.TrimSpace(userID),
			"id":           strings.TrimSpace(id),
			"executeAfter": executeAfter.UTC().Format(time.RFC3339),
			"now":          now.Format(time.RFC3339),
		})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return shieldedWithdrawalFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("failed shielded withdrawal not found")
	})
	if err != nil {
		return ShieldedWithdrawal{}, err
	}
	return result.(ShieldedWithdrawal), nil
}

func (s *MemgraphUserStore) MarkShieldedWithdrawalSuspicious(ctx context.Context, id string, reason string) (ShieldedWithdrawal, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "Marked suspicious by admin"
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, shieldedWithdrawalReturnQuery(`
MATCH (w:ShieldedWithdrawal {id: $id})
WHERE NOT coalesce(w.status, "") IN ["confirmed", "sent", "submitted"]
SET w.status = "suspicious",
  w.error = $reason,
  w.updatedAt = $now
`), map[string]any{
			"id":     strings.TrimSpace(id),
			"reason": reason,
			"now":    now,
		})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return shieldedWithdrawalFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("markable shielded withdrawal not found")
	})
	if err != nil {
		return ShieldedWithdrawal{}, err
	}
	return result.(ShieldedWithdrawal), nil
}

func (s *MemgraphUserStore) RecoverStaleShieldedWithdrawals(ctx context.Context, staleBefore time.Time, executeAfter time.Time, maxAttempts int64) (int64, error) {
	now := time.Now().UTC()
	if executeAfter.IsZero() {
		executeAfter = now
	}
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (w:ShieldedWithdrawal)
WHERE coalesce(w.status, "") = "processing"
  AND coalesce(w.updatedAt, "") <> ""
  AND coalesce(w.updatedAt, "") <= $staleBefore
SET w.status = CASE WHEN coalesce(w.attempts, 0) >= $maxAttempts THEN "failed" ELSE "retry" END,
  w.error = CASE WHEN coalesce(w.attempts, 0) >= $maxAttempts THEN "processing timed out and max attempts were reached" ELSE "processing timed out; retry scheduled" END,
  w.executeAfter = CASE WHEN coalesce(w.attempts, 0) >= $maxAttempts THEN coalesce(w.executeAfter, $executeAfter) ELSE $executeAfter END,
  w.updatedAt = $now
RETURN count(w) AS recovered
`, map[string]any{
			"staleBefore":  staleBefore.UTC().Format(time.RFC3339),
			"executeAfter": executeAfter.UTC().Format(time.RFC3339),
			"maxAttempts":  maxAttempts,
			"now":          now.Format(time.RFC3339),
		})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return intValue(rows.Record(), "recovered"), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return int64(0), nil
	})
	if err != nil {
		return 0, err
	}
	return result.(int64), nil
}

func (s *MemgraphUserStore) GetSystemSetting(ctx context.Context, key string) (string, bool, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (setting:SystemSetting {key: $key})
RETURN coalesce(setting.value, "") AS value
`, map[string]any{"key": strings.TrimSpace(key)})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return stringValue(rows.Record(), "value"), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, nil
	})
	if err != nil {
		return "", false, err
	}
	if result == nil {
		return "", false, nil
	}
	return result.(string), true, nil
}

func (s *MemgraphUserStore) SetSystemSetting(ctx context.Context, key string, value string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		_, err := tx.Run(ctx, `
MERGE (setting:SystemSetting {key: $key})
ON CREATE SET setting.createdAt = $now
SET setting.value = $value,
  setting.updatedAt = $now
`, map[string]any{
			"key":   strings.TrimSpace(key),
			"value": strings.TrimSpace(value),
			"now":   now,
		})
		return nil, err
	})
	return err
}

func shieldedWithdrawalReturnQuery(prefix string) string {
	return prefix + `
OPTIONAL MATCH (u:User {id: w.userId})
RETURN
  w.id AS id,
  w.userId AS userId,
  coalesce(u.email, "") AS userEmail,
  coalesce(u.walletAddress, "") AS userWalletAddress,
  coalesce(w.denomination, "") AS denomination,
  coalesce(w.mode, "") AS mode,
  coalesce(w.noteCommitment, "") AS noteCommitment,
  coalesce(w.nullifierHash, "") AS nullifierHash,
  coalesce(w.poolAddress, "") AS poolAddress,
  coalesce(w.publicSignals, "") AS publicSignals,
  coalesce(w.recipient, "") AS recipient,
  coalesce(w.relayer, "") AS relayer,
  coalesce(w.relayerFee, "0") AS relayerFee,
  coalesce(w.solidityProof, "") AS solidityProof,
  coalesce(w.zkProofSubmissionId, "") AS zkProofSubmissionId,
  coalesce(w.zkProofSubmissionRef, "") AS zkProofSubmissionRef,
  coalesce(w.status, "") AS status,
  coalesce(w.error, "") AS error,
  coalesce(w.transactionId, "") AS transactionId,
  coalesce(w.transactionHash, "") AS transactionHash,
  coalesce(w.executeAfter, "") AS executeAfter,
  coalesce(w.attempts, 0) AS attempts,
  coalesce(w.createdAt, "") AS createdAt,
  coalesce(w.updatedAt, "") AS updatedAt
`
}

func shieldedWithdrawalFromRecord(record *neo4j.Record) ShieldedWithdrawal {
	return ShieldedWithdrawal{
		ID:                   stringValue(record, "id"),
		UserID:               stringValue(record, "userId"),
		UserEmail:            stringValue(record, "userEmail"),
		UserWalletAddress:    stringValue(record, "userWalletAddress"),
		Denomination:         stringValue(record, "denomination"),
		Mode:                 stringValue(record, "mode"),
		NoteCommitment:       stringValue(record, "noteCommitment"),
		NullifierHash:        stringValue(record, "nullifierHash"),
		PoolAddress:          stringValue(record, "poolAddress"),
		PublicSignals:        stringValue(record, "publicSignals"),
		Recipient:            stringValue(record, "recipient"),
		Relayer:              stringValue(record, "relayer"),
		RelayerFee:           stringValue(record, "relayerFee"),
		SolidityProof:        stringValue(record, "solidityProof"),
		ZKProofSubmissionID:  stringValue(record, "zkProofSubmissionId"),
		ZKProofSubmissionRef: stringValue(record, "zkProofSubmissionRef"),
		Status:               stringValue(record, "status"),
		Error:                stringValue(record, "error"),
		TransactionID:        stringValue(record, "transactionId"),
		TransactionHash:      stringValue(record, "transactionHash"),
		ExecuteAfter:         stringValue(record, "executeAfter"),
		Attempts:             intValue(record, "attempts"),
		CreatedAt:            stringValue(record, "createdAt"),
		UpdatedAt:            stringValue(record, "updatedAt"),
	}
}
