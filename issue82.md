## Linked issue

Closes #82

## What changed and why

This change standardizes the public library error surface in `pkg/lens` around stable typed error constants and `errors.Is`/`errors.As`-compatible behavior.

The main goal was to make failure classification deterministic, explicit, and easy for callers to handle without losing context. Each library error now has:

- a stable `ErrorKind` value
- a matching exported sentinel such as `lens.ErrInvalidBase64`
- `errors.Is` support for exact condition matching
- `errors.As` support for inspecting the shared `lens.ErrorKind`
- contextual wrapping so outer layers can preserve the underlying classification

This keeps the library offline-first and adds no network dependency to the core execution path. It also keeps the public API stable and maintains compatibility with existing callers.

## Test evidence

```console
$ go vet ./...
0 issues.

$ golangci-lint run ./...
0 issues.

$ go test -v -race ./pkg/...
=== RUN   TestAccountExtensionNote
=== PAUSE TestAccountExtensionNote
=== RUN   TestAccountExtensionNoteReachesTheWalker
=== PAUSE TestAccountExtensionNoteReachesTheWalker
...
PASS
ok  	github.com/stellar-optics/stellar-xdr-lens/pkg/lens	<time>

$ go build ./cmd/lens
# no output, exit 0