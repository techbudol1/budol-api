# BudolPH API: Horizen Testnet Setup

This branch lets the API serve Horizen testnet trade config and verify escrow transfers against Horizen RPC.

## Required env for Horizen testnet

```env
WELCOME_TOKEN_RPC_URL=https://horizen-testnet.rpc.caldera.xyz/http
WELCOME_TOKEN_CHAIN_ID=2651420
WELCOME_TOKEN_CONTRACT=0x689513fb392e460c6d9225f911fce57fe50d6db4
WELCOME_TOKEN_AMOUNT=
WELCOME_TOKEN_DECIMALS=18
WELCOME_TOKEN_SYMBOL=BUDOL
BUDOL_PROJECT_WALLET_ADDRESS=<project-wallet-on-horizen-testnet>
```

## Optional ERC-4337 smart-wallet env

Set these after deploying the Horizen AA contracts from `gmr-engine` and starting a bundler:

```env
HORIZEN_AA_ENABLED=true
HORIZEN_AA_CHAIN_ID=2651420
HORIZEN_AA_RPC_URL=https://horizen-testnet.rpc.caldera.xyz/http
HORIZEN_AA_ENTRYPOINT_ADDRESS=0xaab43855ac951ad96ba9646e7eb7d7c39378238f
HORIZEN_AA_ENTRYPOINT_VERSION=0.8
HORIZEN_AA_FACTORY_ADDRESS=0xa85ab2137e77b083a615fc861af140f370a26f3e
HORIZEN_AA_BUNDLER_URL=https://bundler.budolph.xyz
HORIZEN_AA_PAYMASTER_ADDRESS=0x2aA3A9D58F21Fb585D78B81246a265d0d013f3b9
HORIZEN_AA_PAYMASTER_URL=https://bundler.budolph.xyz
HORIZEN_AA_GAS_SPONSORED=true
```

`HORIZEN_AA_BUNDLER_URL` should point to the private BudolPH bundler from `gmr-engine`:

```bash
cd gmr-engine
HORIZEN_AA_ENTRYPOINT_ADDRESS=0xaab43855ac951ad96ba9646e7eb7d7c39378238f \
HORIZEN_BUNDLER_VAULT_WALLET_REF=<gmr-vault:v1:...> \
HORIZEN_BUNDLER_ADDRESS=<vault-wallet-address> \
HORIZEN_BUNDLER_PORT=8092 \
bun run aa:bundler:horizen
```

Expose that service through Cloudflare Tunnel, for example:

```text
bundler.budolph.xyz -> http://localhost:8092
```

The API exposes this at:

```text
GET /api/smart-wallet/config
```

When enabled, the frontend may submit escrow transfers from the user's derived SimpleAccount instead of the EOA. The API only accepts `escrowFromAddress` when it matches the SimpleAccount derived from the configured factory and the authenticated user's EOA.

When disabled, the API verifies escrow transfers from the authenticated user's EOA.

Gas sponsorship is active when `HORIZEN_AA_GAS_SPONSORED=true` and the paymaster fields are set. The current BudolPH paymaster only sponsors SimpleAccount BUDOL transfers into the configured project escrow wallet.
