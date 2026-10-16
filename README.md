# stellar-xdr-lens

A Go CLI and library that explains Stellar and Soroban XDR in plain English, and diffs it structurally.

[![CI](https://github.com/stellar-optics/stellar-xdr-lens/actions/workflows/ci.yml/badge.svg)](https://github.com/stellar-optics/stellar-xdr-lens/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/stellar-optics/stellar-xdr-lens.svg)](https://pkg.go.dev/github.com/stellar-optics/stellar-xdr-lens)
[![Go Report Card](https://goreportcard.com/badge/github.com/stellar-optics/stellar-xdr-lens)](https://goreportcard.com/report/github.com/stellar-optics/stellar-xdr-lens)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

## Why this exists

When a Stellar transaction fails, you get a result code like `tx_failed` wrapped
around `opINNER` wrapped around `InvokeHostFunctionResultCodeInvokeHostFunctionTrapped`.
Working out *which operation* failed, *what it was trying to do*, and *what to
do about it* means decoding two separate blobs and correlating them by hand.

`lens` does that correlation for you:

```console
$ lens explain "$ENVELOPE" --result "$RESULT"
Transaction failed at operation 0, invoke_host_function (call harvest on contract CBGSB…KKY3 with 2 argument(s)): The contract trapped: it panicked, or explicitly returned an error.
```

### Honest scope: what already exists

The official [`stellar` CLI](https://developers.stellar.org/docs/tools/cli/stellar-cli)
already ships `stellar xdr decode`, `guess`, `compare` and `encode`. **If you
just need to turn a blob into JSON, use that** — it is maintained by the SDF,
covers every XDR type in the protocol, and is probably already installed.

`lens` exists for the three things that tool does not do:

| | `stellar xdr` | `lens` |
|---|---|---|
| Decode to JSON | ✅ | ✅ |
| Guess the type | ✅ | ✅ |
| **Explain a failure in plain English** | ❌ | ✅ |
| **Pair an envelope with its result** | ❌ | ✅ |
| **Structural, path-level diff** | ordering only (`-1`/`0`/`1`) | ✅ full path diff |
| **Reusable Go library** | ❌ (Rust) | ✅ `pkg/lens` |

That last row matters: Horizon, ingestion pipelines and most Stellar backend
infrastructure are written in Go, and there was no Go library for any of this.

## Install

```sh
go install github.com/stellar-optics/stellar-xdr-lens/cmd/lens@latest
```

Requires Go 1.25 or later. Or build from source:

```sh
git clone https://github.com/stellar-optics/stellar-xdr-lens
cd stellar-xdr-lens
go build ./cmd/lens
```

Everything runs offline. `lens` makes no network calls, which makes it safe on
unsigned transactions and deterministic in CI.

#### Terminal colors

Human-readable terminal output uses the dark color theme by default. Set
`LENS_THEME=light` for a palette suited to light terminal backgrounds; set it
to `dark` to select the default palette explicitly. Customize individual ANSI
styles with `LENS_THEME_DARK_KEY`, `LENS_THEME_LIGHT_ADD`, and the equivalent
`KEY`, `TYPE`, `STR`, `NUM`, `NOTE`, `DIM`, `ADD`, `REMOVE`, and `CHANGE`
variables. Values are SGR parameters without escape characters, for example:

```sh
LENS_THEME=dark LENS_THEME_DARK_KEY='38;5;39' lens decode "$XDR"
```

Set a style variable to an empty string to disable that style. `--color auto`
continues to color only terminal output, while `--color always` and
`--color never` explicitly enable or disable colors. Invalid theme values or
style parameters produce an error. `NO_COLOR` remains honored in automatic
mode.

## Usage

### `lens explain` — why did this fail?

Pair an envelope with its result and `lens` names the failing operation, what it
was attempting, and what to do next:

```console
$ lens explain --file envelope.xdr --result-file result.xdr
Transaction failed at operation 0, invoke_host_function (call harvest on contract CBGSB…KKY3 with 2 argument(s)): The contract trapped: it panicked, or explicitly returned an error.

  Source           GAPB7YMUQ6MPQ23PTFDNBZP7HGGTZPI2DT5JJURUOIXNPLGHW2O3WYXZ
  Fee-bump source  GA2JRQOF6EA3HQWDCEDBPPMLYPJCFLDDGYZLEQGMS5SOBQIB3BAFHVAW
  Sequence         264125967419700139
  Fee bid          0.0024167 XLM (24167 stroops)
  Fee charged      0.0013355 XLM (13355 stroops)
  Signatures       1
  Precondition     valid until 2026-08-03T13:33:39Z

Operations (1)
  ✗ [0] invoke_host_function — call harvest on contract CBGSB…KKY3 with 2 argument(s)
      invoke_host_function_trapped: The contract trapped: it panicked, or explicitly returned an error.
      → This is a failure inside the contract itself, not a protocol-level rejection. Run the call through simulation and read the diagnostic events to find the contract-level cause.

Outcome
  tx_fee_bump_inner_failed: The fee-bump wrapper was valid, but the inner transaction it paid for failed.
    → The fee was still charged to the fee-bump source. Inspect the inner result for the real cause.
  inner transaction:
  tx_failed: The transaction was well-formed and authorised, but at least one of its operations failed.
    → Check the per-operation results: the transaction itself is fine, the failure is inside an operation.
```

All 208 protocol result codes have a written explanation. Use `--fail-on-error`
to exit non-zero when the transaction failed, which makes this usable as a CI gate.

### `lens decode` — read the structure

```console
$ lens decode --depth 3 --file envelope.xdr
TransactionEnvelope
├─ Type: EnvelopeTypeEnvelopeTypeTx
└─ V1
   ├─ Tx
   │  ├─ SourceAccount: GCJNHFXJEQEY54NPKU4DBMT5WOBJUHMYSFBDAQF4OMTEGJN4BQU7ZST6
   │  ├─ Fee: 58002
   │  ├─ SeqNum: 259021488982594615
   │  ├─ Cond
   │  │  ... 1 more level(s) hidden
   │  ├─ Memo
   │  │  ... 1 more level(s) hidden
   │  ├─ Operations  [1]
   │  │  ... 1 more level(s) hidden
   │  └─ Ext
   │     ... 1 more level(s) hidden
   └─ Signatures  [1]
      └─ [0]
         ... 1 more level(s) hidden
```

Accounts render as strkey, amounts as XLM, timestamps as RFC 3339, and enums as
their constant names — not raw byte arrays and integers.

#### Alternative Formats and Interactive Explorer

```console
# Terse single-line summary suitable for grep and shell pipelines
$ lens decode --summary --file envelope.xdr
# or: lens decode --format summary --file envelope.xdr

# SEP-0011 TxRep human-readable format
$ lens decode --txrep --file envelope.xdr
# or: lens decode --format txrep --file envelope.xdr

# Flat CSV row of the high-level transaction fields
$ lens decode --csv --file envelope.xdr
# or: lens decode --format csv --file envelope.xdr

# Interactive terminal tree explorer (TUI)
$ lens decode -i --file envelope.xdr
# or use the dedicated command:
$ lens explore --file envelope.xdr

# Contract spec-aware ScVal decoding from contract WASM or spec XDR
$ lens decode --spec contract.wasm --file invocation.xdr
```

#### CSV export

`lens decode --csv` prints one fixed-schema row per value, which is what a
batch export over thousands of transactions wants:

```csv
type,variant,source,fee_bump_source,seq_num,fee,memo,signatures,operation_count,operation_types,fee_charged,success,result_code,inner_result_code,failed_operations,detail
TransactionEnvelope,fee_bump,GAPB7YMUQ6MPQ23PTFDNBZP7HGGTZPI2DT5JJURUOIXNPLGHW2O3WYXZ,GA2JRQOF6EA3HQWDCEDBPPMLYPJCFLDDGYZLEQGMS5SOBQIB3BAFHVAW,264125967419700139,24167,,1,1,invoke_host_function,,,,,,
```

Every row carries the same number of fields, so rows from different
invocations concatenate and still parse: a column that does not apply — the
fee bid of a bare `TransactionResult`, say — is an empty field rather than a
missing one. `Type` is the XDR type name, `variant` is the envelope shape
(`v0`, `v1`, `fee_bump`), and non-transaction types set `detail` instead of
the transaction columns. Output is RFC 4180, so memos containing commas,
quotes or newlines round-trip through any CSV reader.

Each `lens decode --csv` call emits the header once. To export a batch under a
single header, use the library, which is also how the pairing works:

```go
summaries := make([]*lens.Summary, 0, len(rows))
for _, row := range rows {
	s, err := lens.ExplainPair(row.envelope, row.result)
	if err != nil {
		return err
	}
	summaries = append(summaries, s)
}

f := format.NewCSVFormatter()
if err := f.FormatSummaries(os.Stdout, summaries); err != nil {
	return err
}
```

`WithCSVHeader(false)` drops the header for appends, and `WithCSVDelimiter`
switches the field delimiter — the column order, and the names scripts address
by position, are pinned by tests and are a breaking change to alter.

### `lens diff` — what changed?

```console
$ lens diff before.xdr after.xdr
TransactionResult

~ FeeCharged
    - 35438 0.0035438 (stroops: 35438)
    + 13355 0.0013355 (stroops: 13355)
~ Result.Code
    - TransactionResultCodeTxFeeBumpInnerSuccess
    + TransactionResultCodeTxFeeBumpInnerFailed
~ Result.InnerResultPair.Result.Result.Results[0].Tr.InvokeHostFunctionResult.Code
    - InvokeHostFunctionResultCodeInvokeHostFunctionSuccess
    + InvokeHostFunctionResultCodeInvokeHostFunctionTrapped
- Result.InnerResultPair.Result.Result.Results[0].Tr.InvokeHostFunctionResult.Success = ea29da78…

5 changed, 1 removed
```

Use `--exit-code` to exit non-zero when the values differ, like `diff(1)`.

### `lens asset` — derive a Stellar Asset Contract id

Classic assets have deterministic Stellar Asset Contract ids: the network
passphrase and the asset are hashed into a `C…` address. `lens` computes it
offline, so you can go from an asset to its contract without an RPC call:

```console
$ lens asset "AAAAAVVTREMAAAAAO5kROA7+mIugqJAOsc/kTzZvfb6Ua+0HckD39iTfFcU="
Network      mainnet
Type         credit_alphanum4
Code         USDC
Issuer       GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN
Canonical    USDC:GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN
Contract ID  CCW67TSZV3SSS2HXMBQ5JFGCKJNXKZM7UQUWUZPUTHXSTZLEO7SJMI75
```

`--network` accepts `mainnet`, `testnet`, `futurenet`, or a raw passphrase —
the same asset has a different contract address on each network. `--json`
emits the details with camelCase keys.

### `lens types` — what can it decode?

`lens types` lists the registered type names. `--json` adds the category, a
one-line description, the detection priority and the minimum encoded size:

```console
$ lens types --json | jq -r '.[] | select(.category == "overlay") | .name'
Auth
AuthCert
DontHave
Error
Hello
PeerAddress
StellarMessage
```

### Piping and scripting

`lens` reads from an argument, a `--file`, or stdin, so it composes:

```sh
# Explain a live transaction straight from Horizon
curl -s "https://horizon.stellar.org/transactions/$HASH" \
  | jq -r '.envelope_xdr' \
  | lens explain

# Pull one field out with jq
lens decode --json "$XDR" | jq -r '.value.V1.Tx.Fee'

# Gate a deployment on the transaction having succeeded
lens explain "$ENV" --result "$RES" --fail-on-error || exit 1
```

The `--json` output has a documented, stable shape:

```json
{
  "type": "TransactionResult",
  "value": { "FeeCharged": 13355, "Result": { "Code": "TransactionResultCodeTxFeeBumpInnerFailed" } }
}
```

Absent union arms are omitted rather than emitted as `null`, so `jq` paths stay short.

`explain --json` has its own documented shape, in camelCase to match Horizon
and Soroban RPC:

```json
{
  "kind": "transaction",
  "headline": "Transaction failed at operation 0, invoke_host_function …",
  "fee": 24167,
  "feeCharged": 13355,
  "operations": [
    { "index": 0, "type": "invoke_host_function",
      "result": { "code": "invoke_host_function_trapped", "summary": "…", "hint": "…" } }
  ],
  "outcome": {
    "success": false,
    "reason": { "code": "tx_fee_bump_inner_failed", "…": "…" },
    "innerReason": { "code": "tx_failed", "…": "…" },
    "failedOperations": [0]
  }
}
```

These field names are part of the public interface — sibling tools and scripts
address them with `jq`, so renaming one is a breaking change. The CSV column
names in [`CSVColumns`](#csv-export) are part of the same contract.

## Library use

The CLI is a thin shell over `pkg/lens`:

```go
package main

import (
	"fmt"
	"log"

	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens"
)

func main() {
	env, err := lens.Decode(envelopeB64)
	if err != nil {
		log.Fatal(err)
	}
	res, err := lens.Decode(resultB64)
	if err != nil {
		log.Fatal(err)
	}

	summary, err := lens.ExplainPair(env, res)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(summary.Headline)
	for _, op := range summary.Operations {
		if op.Result != nil && !op.Result.Success {
			fmt.Printf("op %d (%s) failed: %s\n", op.Index, op.Type, op.Result.Summary)
		}
	}
}
```

See the [API reference](https://pkg.go.dev/github.com/stellar-optics/stellar-xdr-lens/pkg/lens)
and [docs/architecture.md](docs/architecture.md).

## A note on type detection

`lens decode` detects the XDR type by trying each known type and keeping the ones
that decode cleanly, relying on the SDK rejecting input it does not fully consume.
On transaction-sized payloads this is reliable — the test suite asserts that every
real fixture matches exactly one type.

Short values are genuinely ambiguous, though. An empty `Memo` and a native `Asset`
are both `AAAAAA==`, because they are the same four bytes. The overlay `Auth`
flags and a void `ScVal` collide the same way, and short messages like `DontHave`
also decode as a `LedgerKey` or TTL entry. Some longer aliases are inherent to the
wire format too: `HotArchiveBucketEntry` is byte-identical to the matching
`BucketEntry` arm, and `BucketMetadata` collides with some `ScVal` encodings.
`lens` reports these rather than guessing silently:

```console
$ lens decode --candidates "AAAAAA=="
* Memo
  Asset
  Auth

3 types matched; the starred one is used by default. Use --type to choose.
```

Pass `--type` when you need certainty.

## Project status

**Early but real.** v0.1 does what this README says, with tests over real captured
mainnet XDR. The library API may still shift before v1.0 — it is not yet frozen.

- Supports 58 XDR types out of the ~400 in the protocol, chosen to cover what
  developers actually paste into a terminal: transactions, ledger entries, SCP,
  overlay messages (including time-sliced surveys), history-archive sets and
  Soroban — including the interface spec embedded in a contract's WASM.
  Adding more is a one-line change; see [docs/architecture.md](docs/architecture.md).
- All 208 protocol result codes are explained.
- No network access anywhere in the library or the tests.

See [docs/roadmap.md](docs/roadmap.md) for what is planned and what is deliberately
out of scope.

## Contributing

Contributions are welcome — especially new XDR type handlers and better failure
explanations, both of which are designed to be one-file changes.
See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

Apache-2.0. See [LICENSE](LICENSE).
