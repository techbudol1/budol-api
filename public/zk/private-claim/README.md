# BudolPH Private Claim Proving Artifacts

These files let the browser generate Groth16 proofs for BudolPH private payout claims:

- `private_winning_claim.wasm`
- `private_winning_claim_final.zkey`
- `verification_key.json`

Current status: development setup. The `.zkey` was generated locally so the end-to-end app flow can run. It is not production-ready.

To replace these with freshly generated artifacts:

```bash
bun run zk:private-claim:artifacts /path/to/pot20_final.ptau "Contributor name"
```

Before using this for real value, replace the current `.zkey` with a final zkey from a proper audited multi-party ceremony, publish the resulting `checksums.sha256`, and update `ceremony.json` with `productionReady: true`.

Production ceremony instructions live in `docs/zk-production-ceremony.md`.
