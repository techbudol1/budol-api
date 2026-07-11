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
HORIZEN_AA_ENTRYPOINT_ADDRESS=<deployed-entrypoint>
HORIZEN_AA_ENTRYPOINT_VERSION=0.8
HORIZEN_AA_FACTORY_ADDRESS=<deployed-simple-account-factory>
HORIZEN_AA_BUNDLER_URL=<erc-4337-bundler-url>
```

The API exposes this at:

```text
GET /api/smart-wallet/config
```

When enabled, the frontend may submit escrow transfers from the user's derived SimpleAccount instead of the EOA. The API only accepts `escrowFromAddress` when it matches the SimpleAccount derived from the configured factory and the authenticated user's EOA.

When disabled, the API verifies escrow transfers from the authenticated user's EOA.
