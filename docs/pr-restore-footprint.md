# RestoreFootprint / ExtendFootprintTtl Deepening

Closes #120

## Summary

This PR improves the explain/deepening logic for `ExtendFootprintTtl` and `RestoreFootprint` failures by using the operation body to surface protocol-aware explanations when the root cause is directly provable from the input XDR.

The deepening logic now calls out cases where the failure is caused by:

- a malformed footprint TTL operation or archival footprint payload,
- exceeding the protocol resource limit for the footprint extension/restore operation,
- an insufficient refundable fee when the operation tries to restore or extend archival storage.

This remains deterministic and offline-safe: it only rewrites the `Summary` and `Hint` when the cause is inferable from the operation itself, and it does not invent ledger state.

## Changes

- added `ExtendFootprintTtl` and `RestoreFootprint` failure deepening in `pkg/lens/explain.go`
- added regression coverage in `pkg/lens/explain_test.go`
- verified the deepened explanations preserve protocol behavior and remain offline-safe

## Validation

Ran successfully:

- `GOTOOLCHAIN=go1.25.0 go test ./... -count=1`

## Notes

GitHub CLI/auth was not available in this environment, so the branch was pushed to the remote but the actual PR could not be opened automatically from here. The branch is ready to be used to open a PR manually in the browser:

- `https://github.com/Purity-Euphemia/stellar-xdr-lens/pull/new/RestoreFootprint`
