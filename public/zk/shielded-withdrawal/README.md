# Shielded Withdrawal Artifacts

Development artifacts for the BudolPH shielded payout withdrawal circuit.

- `shielded_withdrawal.wasm`
- `shielded_withdrawal_final.zkey`
- `verification_key.json`
- `checksums.sha256`
- `ceremony.json`

These artifacts let the browser prove ownership of a credited shielded payout
note without sending the note secret or blinding to BudolPH.

Current status: development setup. The `.zkey` was generated locally so the
end-to-end app flow can run. It is not production-ready.

Before real-value usage, replace the `.zkey` with output from a verified multi-party ceremony, retain the ceremony transcript with the release, publish `checksums.sha256`, and update `ceremony.json` with `productionReady: true`. The ceremony process is maintained with GMR Engine.
