# BudolPH API: Horizen Testnet Setup

This branch lets the API serve Horizen testnet trade config and verify escrow transfers against Horizen RPC.

## Required env for Horizen testnet

```env
WELCOME_TOKEN_RPC_URL=https://horizen-testnet.rpc.caldera.xyz/http
WELCOME_TOKEN_CHAIN_ID=2651420
WELCOME_TOKEN_CONTRACT=0xb06EC4ce262D8dbDc24Fac87479A49A7DC4cFb87
WELCOME_TOKEN_AMOUNT=100
WELCOME_TOKEN_DECIMALS=18
BUDOL_PROJECT_WALLET_ADDRESS=<project-wallet-on-horizen-testnet>
```

The current option-1 frontend requires external wallets for Horizen trading. Managed/social wallet escrow should remain Arbitrum-only until we build Horizen smart wallet or relayer support.

