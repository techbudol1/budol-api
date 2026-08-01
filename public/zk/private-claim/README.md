# BudolPH Private Claim Proving Artifacts

These files let the browser generate Groth16 proofs for BudolPH private payout claims:

- `private_winning_claim.wasm`
- `private_winning_claim_final.zkey`
- `verification_key.json`

Current status: development setup. The `.zkey` was generated locally so the end-to-end app flow can run. It is not production-ready.

Before using this for real value, replace the current `.zkey` with a final key from a verified multi-party ceremony, publish `checksums.sha256`, and update `ceremony.json` with `productionReady: true`. The ceremony process is maintained with GMR Engine.
