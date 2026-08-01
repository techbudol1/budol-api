# Shielded trade coordinator

The API coordinates delayed private-order batches without publishing individual
orders. A browser deposits a fixed-denomination BUDOL note, generates a Groth16
proof locally, pays the configured privacy-access fee, and submits the proof to
the coordinator. The coordinator pins the verification key, submits the proof
to zkVerify, relays the same proof to the Horizen vault, and records the order as
pending.

The API opens a batch lazily when the first user requests a private trade for a
supported denomination. It does not spend relayer gas continuously on empty
batches. After the batch delay, the worker settles the vault's aggregate collateral and
creates the corresponding market positions while public market reads are held
behind a short publication lock. Public activity and holder statistics exclude
the individual shielded trades; the final market odds reflect the batch total.

## Configuration

```dotenv
SHIELDED_TRADING_ENABLED=true
SHIELDED_TRADE_VAULTS=10@50:0xVaultFor10_05,25@50:0xVaultFor25_125
SHIELDED_TRADE_BATCH_WINDOW_SECONDS=120
SHIELDED_TRADE_BATCH_EXPIRY_SECONDS=900
SHIELDED_TRADE_POLL_INTERVAL_SECONDS=10
SHIELDED_TRADE_ZKVERIFY_DOMAIN_ID=175
SHIELDED_TRADE_VERIFICATION_KEY_PATH=zk/shielded-trade/verification_key.json
```

The format is `tradeAmount@feeBps:vaultAddress`. A vault is available only when
its immutable fee matches the active GMR Engine fee.

## Endpoints

- `GET /api/shielded-trades/config`: active vaults and collecting batches.
- `POST /api/shielded-trades/batches`: lazily open or return the current batch
  for one supported trade amount.
- `POST /api/shielded-trades`: validate, submit to zkVerify, relay on-chain, and
  queue a hidden order.
- `GET /api/zk/shielded-trade/:file`: pinned proving artifacts.

## Security boundary

This version protects the trader's wallet, side, and amount from public-chain
observers. The coordinator receives plaintext order details and can correlate
the authenticated user with the order. Confidential operator-side matching
requires a TEE/FHE/MPC follow-up. Development proving artifacts must not be used
for real value without a new ceremony and independent audit.
