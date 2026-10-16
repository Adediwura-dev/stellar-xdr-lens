# Architecture

This document explains how `stellar-xdr-lens` fits together and, more usefully,
where to make the three changes contributors most often want to make:

- [Adding a new XDR type](#adding-a-new-xdr-type)
- [Improving how a type is rendered](#adding-an-enricher)
- [Adding a new output format](#adding-a-formatter)

## The pipeline

Everything flows one way:

```
base64 ──► detect ──► xdr decode ──► Node tree ──► enrich ──► Formatter ──► output
              │            │             │                        │
        registry      stellar SDK    neutral IR            tree │ json │ text │ diff │ summary │ txrep │ csv
```

The important property is that **only one stage knows about XDR**. Once a value
becomes a `Node` tree, nothing downstream imports `xdr` or uses reflection.
That is what lets a new output format work for all 58 registered types at once,
and lets the differ work without knowing what it is comparing.

## Package layout

```
cmd/lens/              main; wires os.Args to internal/cli and maps errors to exit codes
internal/cli/          cobra commands — deliberately thin
  root.go              command tree, global flags, explore (TUI) command
  decode.go            decode + types commands, formatting and spec decoding flags
  explain.go           explain command, exit-code plumbing
  diff.go              diff command
  asset.go             asset command; network resolution for contract ids
  input.go             argument / --file / stdin resolution
  tui_tree.go          interactive terminal tree explorer (TUI)
pkg/lens/              the library; this is the reusable surface
  decode.go            type registry, Detect, Decode, DecodeAs
  registry_index.go    introspection: RegistryIndex, categories, min sizes
  node.go              the neutral Node tree — the only file using reflection
  enrich.go            per-type human rendering (strkey, amounts, timestamps)
  ledger_header.go     ledger-header field notes and protocol parameters
  bucket_entry.go      bucket list, tombstone and hot archive terminology
  scp.go               consensus node identity and pledge names
  state_archival.go    TTL expiry and eviction rendering
  account_extensions.go liabilities, sponsorship counters and sequence-ledger notes
  data_entry.go        account data values: printable text vs hex
  survey.go            time-sliced survey command names and encrypted bodies
  contract_code.go     WASM section parsing: contract spec, env meta and meta
  overlay.go           peer addresses, certificate keys, overlay message names
  tx_history.go        ledger-sequence field notes for history sets and batches
  asset_types.go       asset details and Stellar Asset Contract id derivation
  explain.go           Summary, Outcome, per-operation descriptions
  reason.go            all 208 result codes → plain English
  reason_lookup.go     O(1) short code lookup table (ReasonByCode)
  protocol21_reasons.go Protocol 21 & 22 error additions and Soroban host error decoder
  op_isolator.go       Multi-operation failure isolator (pre-op vs execution pinpointing)
  trap_reasons.go      Soroban trapped host function diagnostic reason decoder
  diff.go              structural path diff
  spec_decode.go       Contract spec aware ScVal decoding (ScSpecEntry / WASM custom section)
  format/              output formats
    format.go          Formatter interface, colour palette, TTY detection
    tree.go            colourised tree
    json.go            stable JSON
    text.go            explain summaries
    diff.go            diff rendering
    summary.go         compact single-line summary (grep/pipeline friendly)
    txrep.go           SEP-0011 human-readable TxRep format
    csv.go             flat CSV export for batch ledger envelopes
testdata/              real captured mainnet XDR + manifest.json recording provenance
```

`pkg/lens` is the public API and is what external projects import. `internal/cli`
cannot be imported by anyone else, which is deliberate: the CLI is an
implementation detail, and keeping it internal means its shape is free to change
without breaking anyone.

## The Node tree

`Node` is a small, format-independent tree:

```go
type Node struct {
	Name     string   // field name in the parent
	Kind     Kind     // KindScalar | KindStruct | KindList
	TypeName string   // originating XDR type, best-effort
	Value    any      // scalar payload: string, bool, int64, uint64, float64
	Note     string   // human rendering from an enricher
	Children []*Node
}
```

`Value` is restricted to those five types so that **formatters never need
reflection**. `Note` carries the enriched rendering — a strkey address, a
formatted amount — alongside the raw value, so machine-readable output can keep
the raw form while human output shows the friendly one.

### Why reflection, and why only here

`buildNode` in [`node.go`](../pkg/lens/node.go) is the only reflective code in
the project. The alternative — hand-writing a renderer for each of the ~400
generated XDR types — is not maintainable, and the other obvious option,
`encoding/json`, loses information we need: marshalling a result code yields
`"Code": -13` rather than `TransactionResultCodeTxFeeBumpInnerFailed`, because
Go marshals the underlying integer and not the `String()` method.

So the walker does three things generically:

1. **Prunes nil pointers.** XDR unions carry one populated arm and up to 27 nil
   ones. Emitting them as `null` is what makes raw XDR JSON unreadable.
2. **Resolves enum names.** Generated enums carry a `ValidEnum` method that
   plain numeric aliases like `Int64` and `Uint32` do not, so that method is
   used to tell an enum from an ordinary number before calling `String()`.
3. **Renders byte sequences as hex.** A 32-byte hash is a hex string, not a
   list of 32 integers.

### Robustness

Generated XDR accessors dereference union arms without checking the
discriminant, so they panic on a zero or malformed value — `AccountId.Address()`
on a zero value is a nil dereference. Since this tool's entire job is parsing
untrusted, possibly truncated blobs, both the decoder and every enricher run
behind a `recover`. A value we cannot prettify degrades to its generic
rendering; it never takes the process down.

## Adding a new XDR type

Add one line to `registry` in [`decode.go`](../pkg/lens/decode.go):

```go
var registry = []xdrType{
	// ...
	{"ClaimableBalanceEntry", func() any { return new(xdr.ClaimableBalanceEntry) }, 30},
}
```

The three fields are the display name, an allocator, and a detection priority.
Then add the matching entry to `typeDocs` in
[`registry_index.go`](../pkg/lens/registry_index.go) so `lens types --json`
can describe it:

```go
"ClaimableBalanceEntry": {CategoryLedgerEntry,
	"A balance claimable by named claimants, with its predicate and asset."},
```

That is the whole change. Detection, `--type`, `lens types` (including
`--json`), the tree, the JSON output and the differ all pick it up
automatically. `TestTypeDocsCoversRegistry` fails if the two maps drift apart,
and `TestRegistryIndexMinWireSizeIsLowerBound` checks the published minimum
size against the actual fixtures.

### Choosing a priority

Priority breaks ties when several types decode the same bytes. Higher wins.
Roughly:

| Range | Use for |
|---|---|
| 90–100 | What developers paste most: envelopes, results, meta |
| 60–89 | Ledger entries, keys, inner transaction types |
| 40–59 | Ledger-level types, Soroban, history-archive sets and batches |
| 30–39 | Overlay messages and events, which are large but rarely pasted |
| 1–29 | Small building blocks that decode from very few bytes |

Small types deserve low priority because short payloads are ambiguous — a
4-byte value may decode as half a dozen types, and the ranking decides which
one `lens decode` shows by default. The overlay `Auth` message is the extreme
case: four bytes that also parse as a void `ScVal`, so it ranks at 2 and is
decoded with `--type` in practice.

## Adding an enricher

Enrichers turn an XDR value into something a human recognises. `enricherFor` in
[`enrich.go`](../pkg/lens/enrich.go) maps a concrete XDR type to its enricher.
The core entries are a declaration-time literal; a type added by a feature file
registers itself without touching `enrich.go` at all:

```go
func init() {
	enricherFor[reflect.TypeOf(xdr.MyType{})] = enrichMyType
}

func enrichMyType(rv reflect.Value) (display string, raw any, ok bool) {
	v, valid := rv.Interface().(xdr.MyType)
	if !valid {
		return "", nil, false
	}
	return "human form", v.RawForm(), true
}
```

Return `ok=false` to fall back to the generic rendering — do this for union arms
you cannot sensibly render, such as liquidity-pool-share assets, rather than
returning a misleading string.

The registry is populated once before any decoding happens and is read-only
afterwards, so it needs no locking.

### Field-name enrichment

Some renderings depend on the field name rather than the type. XDR models
amounts, sequence numbers and offer ids all as `Int64`, so the only signal that
a value is an amount is the field it sits in. `amountFields` in `enrich.go`
lists the known amount fields; anything not listed stays a plain integer,
because rendering a sequence number as `12345.0000000` would be worse than
showing it raw.

`fieldNotes` is the same idea for anything that is not an amount — a hint
attached to a field rendered alongside it. `LedgerVersion: 22` becomes
`protocol 22`, and `LiveUntilLedgerSeq` becomes `live through ledger N`.
Feature files register notes from `init`:

```go
func init() {
	fieldNotes["SlotIndex"] = slotIndexNote
}
```

A note is only rendered on a scalar leaf. The tree and JSON formatters ignore
`Node.Note` on struct and list nodes, so a note that should be visible has to
sit on a child that renders as a scalar — `account_extensions.go` attaches the
extension capability note to the union's discriminant child for exactly this
reason.

### Node hooks

Some useful content is not an XDR field at all: a Soroban contract's interface
lives in a WASM custom section inside the opaque `Code` byte array of a
`ContractCodeEntry`. `nodeHooks` in [`node.go`](../pkg/lens/node.go) maps a
concrete XDR type to a function that runs after the declared fields have been
walked and may append derived children or annotate existing ones:

```go
func init() {
	nodeHooks[reflect.TypeOf(xdr.ContractCodeEntry{})] = decorateContractCode
}

func decorateContractCode(n *Node, v any) {
	entry := v.(xdr.ContractCodeEntry)
	parsed, err := ParseContractCode(entry.Code)
	// annotate the Code leaf and append Spec, EnvMeta and Meta children
}
```

Hooks receive the concrete decoded value rather than a `reflect.Value`, so
feature files stay free of reflection, and one that panics is treated as absent
rather than being allowed to take the process down. Use a hook when the
rendering depends on content buried inside a field; use the enricher registries
when the type or field name is enough.

## Adding a formatter

Implement `Formatter` in [`format/format.go`](../pkg/lens/format/format.go):

```go
type Formatter interface {
	Format(w io.Writer, v *lens.Value) error
}
```

Because you receive a `*lens.Value` and walk its `Node` tree, a new format works
for every registered type immediately and needs no XDR knowledge. Follow the
existing files' functional-options pattern (`WithPalette`, `WithIndent`) for
configuration.

Colour is handled centrally: call `format.PaletteFor(w, mode)`, which honours
`--color`, the [`NO_COLOR`](https://no-color.org) convention, and whether the
writer is a terminal. Never write escape sequences directly.

## Adding a result code explanation

[`reason.go`](../pkg/lens/reason.go) maps every protocol result code to plain
English. Keys are the exact strings returned by the constants' `String()`
method:

```go
"PaymentResultCodePaymentUnderfunded": bad(
	"The source account does not hold enough of the asset to send this amount.",
	"Remember that XLM balances must stay above the minimum reserve, so the spendable balance is lower than the total."),
```

Use `ok(...)` for success codes and `bad(summary, hint)` otherwise; the hint may
be empty when no general advice applies.

Two rules, both enforced by tests:

- **Do not restate the constant.** "tx_bad_seq" is not an explanation; "the
  sequence number did not match the source account's next expected sequence
  number" is.
- **Cover everything.** `TestEveryResultCodeIsExplained` enumerates all 208
  codes across the 28 protocol enums by reflection and fails if any lacks an
  entry. When a new protocol version adds a code, that test tells you its name.

## Introspection and contract ids

Two small APIs round out the library for tooling use:

- [`RegistryIndex()`](../pkg/lens/registry_index.go) returns a `TypeInfo` for
  every registered type — name, category, description, priority and minimum
  encoded size — which is what `lens types --json` emits. The index is derived
  from the live registry, so it can never list a type `DecodeAs` rejects. Sizes
  are computed structurally from the Go types by reflection, with a depth cap
  for self-referential unions and a conservative fallback, so the number is a
  lower bound that holds for every valid value; no fixture or network access is
  involved.
- [`DescribeAsset`](../pkg/lens/asset_types.go) decodes a classic asset and
  derives its Stellar Asset Contract id. The id is a hash of the network id and
  the asset (`Asset.ContractID`), rendered as a strkey `C…` address, so it is
  deterministic, offline, and different on every network. `DescribeAssetValue`
  does the same for an already-decoded `Value` and reports `ErrNotAsset` when
  the value is not an asset.

`ContractId` and `ScAddress` values anywhere in a decoded tree render as strkey
addresses too, via enrichers registered in `asset_types.go` — a contract id
buried in a Soroban footprint is as readable as one at the top level.

## Explain, and why it reads the Node tree

`explainOperationResult` needs the type-specific result code buried in an
`OperationResult`'s `Tr` union. Doing that with the SDK directly means a switch
over all 27 arms.

Instead it reuses the `Node` tree, which has already selected the populated arm
and rendered its code as a constant name. Finding the one child that is not the
discriminant and reading its `Code` field costs a few lines and, more
importantly, keeps working when the protocol adds a 28th operation type.

The rest of `explain.go` uses the concrete SDK types, because
`TransactionEnvelope` exposes helpers (`SourceAccount`, `Operations`,
`Preconditions`, `IsFeeBump`) that already normalise across v0, v1 and fee-bump
envelopes. Using them is both clearer and more correct than re-deriving that
logic from the tree.

## Reason lookup table (ReasonByCode)

Downstream consumers (such as `focus` and automated diagnostic linters) frequently
require O(1) random-access lookups for short canonical result codes without
performing linear scans or importing raw XDR definitions.

[`reason_lookup.go`](../pkg/lens/reason_lookup.go) provides:

- `lens.ReasonByCode(short string) (Reason, bool)`: thread-safe, case-insensitive
  O(1) resolution of canonical short codes (e.g. `payment_underfunded`, `tx_bad_seq`).
- `lens.AllReasons() []Reason`: sorted slice of all registered reasons for enumeration.
- `lens.ReasonLookupTable() map[string]Reason`: snapshot copy of the full lookup map.

The lookup table is precomputed at initialization and automatically updated
whenever protocol extension packages register additional result codes.

## Protocol 21+ Error Codes and Explanations

Stellar Protocol 21 and 22 introduce state archival, TTL management, key freezing,
and expanded Soroban host error semantics. [`protocol21_reasons.go`](../pkg/lens/protocol21_reasons.go)
provides plain-English diagnostic explanations and actionable remediation hints for:

- **State Archival & Key Freezing**: `tx_frozen_key_accessed` (CAP-0056) when an
  operation attempts to access an expired, frozen ledger entry.
- **Soroban State and Fee Codes**: `invoke_host_function_entry_archived`,
  `invoke_host_function_insufficient_refundable_fee`,
  `extend_footprint_ttl_insufficient_refundable_fee`, and
  `restore_footprint_insufficient_refundable_fee`.
- **Soroban Host Errors**: Full decoders for `xdr.ScError`, mapping error types
  (`ScErrorTypeSceStorage`, `ScErrorTypeSceBudget`, `ScErrorTypeSceWasmVm`, etc.)
  and error codes (`ScErrorCodeScecIndexBounds`, `ScErrorCodeScecExceededLimit`, etc.)
  into descriptive developer-facing errors.

## Multi-Operation Transaction Failure Isolator

Stellar transactions execute atomically. When a transaction contains multiple
operations, developers need to isolate:

1. **Pre-operation Check Failures**: Transaction-level failures (e.g. `tx_bad_seq`,
   `tx_bad_auth`, `tx_too_early`, `tx_insufficient_balance`) reject the transaction
   during header and precondition validation before any operation runs.
2. **Operation Execution Failures**: The transaction passed pre-operation checks and
   was applied up to the failing operation index.

[`op_isolator.go`](../pkg/lens/op_isolator.go) inspects envelopes and results to pinpoint:

- The exact zero-based operation index where execution halted (`FailingOpIndex`).
- The operation type name (`FailingOpType`, e.g. `payment`, `manage_buy_offer`).
- Whether the failure was a `pre_operation` check failure or an `operation` execution failure.
- A comprehensive multi-line `DiagnosticReport()` for debugging.

The CLI exposes this via `lens explain --isolate`.

## Soroban Trapped Host Function Diagnostic Decoder

When a Soroban contract call traps, the transaction result code is simply
`invoke_host_function_trapped` without internal error details. The actionable panic
string or host abort reason is stored in `DiagnosticEvent` payloads within transaction
metadata (`TransactionMeta.V3.SorobanMeta.DiagnosticEvents`).

[`trap_reasons.go`](../pkg/lens/trap_reasons.go) correlates trapped invocations with
diagnostic events to extract:

- **Panic Messages**: Human-readable strings from `log` and `panic` topics/data.
- **Host & Contract Errors**: Decoded `ScError` values including custom contract error codes.
- **Contract & Function Identity**: Identifies the contract ID (rendered as strkey `C...`)
  and function name in the failing execution frame.
- **Diagnostic Trace**: Chronological trace of diagnostic events leading to the abort.

The CLI exposes this via `lens explain --meta <path-or-base64>`.

## Testing approach

Fixtures in `testdata/` are **real captured mainnet transactions**, with
`manifest.json` recording each transaction hash so a fixture can be traced back
to the ledger. Tests never touch the network.

For types that never appear as standalone Horizon payloads — SCP envelopes,
bucket entries, ledger headers, TTL entries, overlay messages, history-archive
sets, classic assets, account extensions, data entries, contract code entries
and survey messages — `manifest.json` also lists synthetic fixtures:
deterministic payloads built with `xdr.MarshalBase64` from fixed hashes and
sequence numbers. Each one is declared as such in the manifest, so provenance
is never in doubt. Asset fixtures double as known-answer tests: the native and
USDC assets carry well-known mainnet SAC contract ids that the tests assert
exactly.

Where a case cannot be captured — every classic operation type, memo variant and
precondition shape — tests construct XDR with the SDK encoder and round-trip it
through the real decode path, rather than reaching into internals.

The suite deliberately targets properties that would break the tool if violated:

- detection stays unambiguous on real payloads (`TestDetectIsUnambiguousForRealPayloads`);
- ambiguity is reported, not hidden (`TestDetectReportsGenuineAmbiguity`);
- the introspection index tracks the registry exactly
  (`TestTypeDocsCoversRegistry`) and its size estimates hold as lower bounds
  (`TestRegistryIndexMinWireSizeIsLowerBound`);
- enums render as names, not integers (`TestEnumsRenderAsConstantNames`);
- a failure is attributed to the operation that caused it
  (`TestExplainPairAttributesFailureToOperation`);
- JSON output is deterministic and omits nulls;
- every result code has a real explanation.

## Compact single-line summary (`pkg/lens/format/summary.go`)

`CompactFormatter` serialises any decoded XDR `*lens.Value` into a terse, single-line format ideal for shell pipelines, `grep`, and quick log examination.
- Envelopes and Transactions output source accounts, sequence numbers, operation counts, and fees.
- Operation results and transaction results highlight execution codes and failure/success status.
- Nested structures are flattened into concise key-value tokens (`type=TransactionEnvelope v1 source=G... ops=1 fee=100 stroops`).
- Like all formatters, it is strictly offline and adheres to the `Formatter` interface without calling network APIs.

## SEP-0011 TxRep formatter (`pkg/lens/format/txrep.go`)

`TxRepFormatter` implements the human-readable [SEP-0011 TxRep](https://github.com/stellar/stellar-protocol/blob/master/ecosystem/sep-0011.md) standard format for Stellar transaction envelopes.
- Emits canonical TxRep colon-delimited lines (e.g. `tx.sourceAccount: G...`, `tx.fee: 100`, `tx.operations[0].body.paymentOp...`).
- Handles `TransactionEnvelope` variants: V0, V1, and FeeBump envelopes.
- Formats signatures with key hints and base64 payloads (`tx.signatures[0].signature: ...`).
- Generates valid TxRep output for debugging and multisig coordination offline.

## Flat CSV export (`pkg/lens/format/csv.go`)

`CSVFormatter` extracts the high-level fields of many transactions into one flat,
machine-readable table, which is the shape a batch export — "who sent it, what it
cost, what it did, and whether it succeeded" — needs.

- The schema is fixed and exported: [`CSVColumns`](../pkg/lens/format/csv.go)
  lists the sixteen columns in emit order, and the column constants and the
  header row are pinned against it by `TestCSVColumnIndexesMatchSchema` and
  `TestCSVColumnsAreStable`. Renaming or reordering a column is a breaking
  change for scripts that address them positionally.
- `FormatAll` and `FormatSummaries` are the batch entry points: one header,
  then one row per value, so thousands of transactions concatenate under a
  single header. `WithCSVHeader(false)` drops the header for appends.
- Rows are fixed-width. A field that does not apply to a row — the fee bid of a
  bare `TransactionResult`, the variant of a bucket entry — is emitted as an
  empty field, never omitted, so every row parses with the same reader.
- Transaction values go through `lens.Explain`, which is what turns the raw
  tree into short result codes, short operation names and the outcome. The
  paired form, `FormatSummary` / `FormatSummaries` on a `lens.Summary` from
  `ExplainPair`, is the one that fills `success`, `result_code`,
  `inner_result_code` and `failed_operations` from a single transaction.
- Any other registered type still exports: `type` carries the XDR type name and
  `detail` carries the tree flattened to `key=value` pairs. A transaction that
  cannot be explained is an error instead, because a half-filled transaction row
  would be a lie.
- Output is written through `encoding/csv`, so a memo containing the delimiter,
  a quote or a newline is quoted per RFC 4180 and round-trips through any
  standard reader. Nothing but the value and the writer influences the bytes:
  no network, no clock, no environment, which is what makes the export
  deterministic in CI.
- Rows are built before the writer is touched, so a failed batch leaves the
  writer untouched rather than truncated mid-export.

Reachable from the CLI as `lens decode --csv` (or `--format csv`); see the
[README](../README.md#csv-export) for the column list.

## Interactive Terminal Tree Explorer (`internal/cli/tui_tree.go`)

`TUITree` provides a curses-like interactive navigation experience within standard ANSI terminals for exploring deeply nested XDR structures.
- Launchable with `lens decode -i`, `lens decode --interactive`, or `lens explore` / `lens tui`.
- Keybindings:
  - `j` / `Down`: Move selection down
  - `k` / `Up`: Move selection up
  - `Enter` / `Space` / `Right`: Expand or toggle node
  - `Left` / `h`: Collapse node or jump to parent
  - `e`: Expand all nodes
  - `c`: Collapse all nodes
  - `/`: Search / filter node paths
  - `q` / `Ctrl+C`: Quit
- Pure offline terminal handling using raw mode and ANSI escape codes with fallback to static rendering in non-TTY environments.

## Contract spec-aware ScVal decoding (`pkg/lens/spec_decode.go`)

Soroban smart contracts encode arguments, return types, storage entries, and events as opaque `xdr.ScVal` structures. `pkg/lens/spec_decode.go` resolves these generic values against contract specifications:
- Accepts contract specs as raw XDR byte slices/streams or extracted from WebAssembly (`.wasm`) binaries via the `contractspecv0` custom section.
- Resolves:
  - UDT Structs (`ScSpecUdtStructV0`): Maps tuple/map fields to human-readable struct member names.
  - UDT Enums (`ScSpecUdtEnumV0`): Maps numeric discriminant values to enum variant identifiers.
  - UDT Unions (`ScSpecUdtUnionV0`): Labels case names and associated values.
  - Function Invocations (`ScSpecFunctionV0`): Resolves function names and parameter lists in `InvokeHostFunctionOp`.
- Usable via `--spec <spec.xdr|contract.wasm>` in `lens decode`.

