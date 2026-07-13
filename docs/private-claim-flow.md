# BudolPH Private Claim Flow

This is the current implemented private-claim payout path.

## What Happens On Trade

Before BudolPH records a trade, the frontend creates a private claim note using the same Poseidon commitment scheme as the ZK circuit.

The trade API receives and stores only the public commitment:

- `privateClaimLeaf`

The frontend stores the full note in browser local storage:

- `marketId`
- `outcome`
- `amount`
- `userSalt`
- `secret`
- `leaf`
- `nullifierHash`

The note is required later to claim a payout.

## What Happens On Resolution

Admin resolution no longer pays all winners immediately.

For winning or cancelled trades with a payout, BudolPH marks them:

```text
payoutStatus = claimable
```

That makes the user claim the payout one trade at a time.

## What Happens On Claim

Before the payout claim, the frontend loads:

- the local private claim note
- the public market claim tree

It builds the circuit input in the browser. If proving artifacts are available under `public/zk/private-claim/`, BudolPH API serves them from `GET /api/zk/private-claim/:file` and the browser can generate the Groth16 proof automatically. Otherwise, the portfolio page shows the circuit input so the proof can be generated manually.

The checked-in artifacts are development artifacts from a local setup. Replace the `.zkey` files with audited multi-party ceremony outputs before real-value usage. The production process is documented in `docs/zk-production-ceremony.md`.

The frontend submits the proof through BudolPH:

```text
POST /api/private-claims/proof-submissions
```

If `ZEN_PRIVATE_CLAIM_FEE` is greater than zero, the request must include `privacyReceiptTxHash`. BudolPH verifies that this native tZEN transaction was sent by the logged-in user's wallet to `ZEN_PRIVACY_ACCESS_FEE_COLLECTOR_ADDRESS` for at least the configured fee before forwarding the proof to GMR Engine/ZKVerify.

BudolPH validates the proof public signals against the logged-in user's trade before forwarding the proof to GMR Engine/ZKVerify.

The user submits:

- `tradeId`
- `nullifierHash`
- `zkProofSubmissionId`
- `privacyReceiptTxHash` when shielded payout fee is enabled

BudolPH then:

1. Loads the stored trade commitment leaf.
2. Recomputes the market claim root from all trade commitments.
3. Loads the ZKVerify proof submission from GMR Engine.
4. Requires the proof status to be `submitted` or `finalized`.
5. Confirms the proof public signals match the current root, the trade outcome, and the submitted nullifier.
6. Confirms the trade belongs to the logged-in user.
7. Confirms the trade is settled as `won` or `cancelled`.
8. Confirms the trade payout is still claimable.
9. Reserves the nullifier in Memgraph.
10. If shielded payout mode is enabled, verifies `ZEN_SHIELDED_PAYOUT_FEE` payment and credits one or more fixed-denomination shielded payout notes through GMR Engine.
11. If shielded payout mode is disabled, sends the BUDOL payout directly through GMR Engine.
12. Records the payout transaction IDs on the trade and private claim.

If the same nullifier is submitted twice, the second claim is rejected.

## ZKVerify Status

Every claim must include `zkProofSubmissionId`. BudolPH checks GMR Engine:

```text
GET /v1/zkverify/proofs/:id
```

The proof must be `submitted` or `finalized`.
The proof public signals must match:

- `root`: current market claim root
- `resolvedOutcome`: the winning trade outcome
- `nullifierHash`: the submitted nullifier

Claims without a verified matching proof are rejected before the nullifier is reserved or funds are sent.

## What Is Still Public

This flow hides the private note values from public observers. If shielded payout mode is disabled, the payout transfer is still a normal ERC20 transfer, so the final recipient and payout amount remain visible on Horizen.

BudolPH now supports fixed-denomination shielded payout pools. In that mode, the claim credits notes to one or more pools and the user later withdraws with a separate proof. The withdrawal is still public, but it is detached from the original claim and uses configured denominations.

## Privacy Access Fees

The MVP privacy-fee loop uses native tZEN transfers:

- `ZEN_PRIVACY_ACCESS_FEE_COLLECTOR_ADDRESS`: collector wallet that receives privacy access fees.
- `ZEN_HIDE_POSITION_FEE`: configured but not enforced yet because BudolPH does not currently publish user portfolio/trade-history pages to other users.
- `ZEN_PRIVATE_CLAIM_FEE`: enforced before `POST /api/private-claims/proof-submissions`.
- `ZEN_SHIELDED_PAYOUT_FEE`: enforced before `POST /api/private-claims` credits a shielded payout note.

The public config endpoint is:

```text
GET /api/privacy-access/config
```

Managed Google wallets pay the same native tZEN fees through GMR Engine Vault:

```text
POST /api/privacy-access/managed-fee
```

BudolPH calls GMR Engine to broadcast a native tZEN transfer from the managed user wallet to the configured collector, then verifies the resulting transaction hash exactly like a self-custody payment.

## Current MVP Limitations

- Hide-position is a planned paid privacy tier, but there is no public profile/trade-history surface to hide from yet. Until that exists, all trade notes are private-note capable by default and hide-position fee collection is not triggered.
- Browser claim notes are still critical. If the user loses the browser note and has no encrypted backup, they cannot generate the private claim or shielded withdrawal proof.
- Self-custody users pay privacy tZEN fees from the browser. Managed Google wallets pay through GMR Engine Vault. Both paths require the paying wallet to have enough native tZEN for the fee and gas.
- Shielded payouts only work when the payout amount can be exactly split across configured pool denominations. Unsupported amounts use direct fallback only if `SHIELDED_PAYOUT_DIRECT_FALLBACK=true`.
- The checked-in ZK artifacts are development artifacts unless replaced with audited ceremony outputs and checksums before production.
