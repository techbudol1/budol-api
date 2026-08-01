# BudolPH API

BudolPH API is the application backend for a privacy-aware prediction-market experience on Horizen Testnet. It provides authentication, market state, trading and settlement validation, wallet activity, privacy-access receipt validation, and the operational API consumed by BudolPH Admin.

## What it does

- Serves public markets, quotes, portfolio data, notifications, and wallet activity.
- Verifies BUDOL escrow transfers and coordinates settlement and payout requests with GMR Engine.
- Supports direct self-custody wallets, sponsored permit flows, and configured ERC-4337 smart accounts.
- Validates tZEN privacy-access receipts for opt-in hidden positions, private claims, and shielded payouts.
- Persists application state in Memgraph.

The API does not keep user private keys. Signing and project-wallet transaction execution are delegated to GMR Vault through GMR Engine.

## Architecture

```text
BudolPH User App / BudolPH Admin
                │
                ▼
           BudolPH API
          ├── Memgraph
          └── GMR Engine ── GMR Vault ── Horizen Testnet
```

## Local development

Requirements: Go, Bun, and a reachable Memgraph instance.

```bash
cp .env.server.example .env.server
bun install
go test ./...
go run ./cmd/api
```

The default local address is `http://localhost:8082`.

See [HORIZEN_TESTNET_API.md](./HORIZEN_TESTNET_API.md) for chain and smart-account configuration. Supporting documents cover the human-reviewed News Scout workflow and private-claim lifecycle.

## Security boundaries

- Use `APP_ENV=production`, strong unique credentials, HTTPS, and a restricted `ALLOWED_ORIGINS` list outside local development.
- Keep GMR Engine and Vault credentials server-side.
- Never commit `.env.server`, wallet keys, OAuth secrets, or service credentials.
- Development ZK artifacts are testnet-only and must not be used for real-value deployments.
