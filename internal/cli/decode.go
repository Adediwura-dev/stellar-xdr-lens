package cli

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens"
	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens/format"
)

func newDecodeCmd(g *globalFlags, stdout io.Writer, stdin io.Reader) *cobra.Command {
	var (
		asJSON      bool
		asYAML      bool
		isHex       bool
		isMulti     bool
		inspectWASM bool
		typeName    string
		maxDepth    int
		showTypes   bool
		annotate    bool
		listCands   bool
		compactOut  bool
	)

	cmd := &cobra.Command{
		Use:   "decode [base64|hex]",
		Short: "Decode XDR and print it as a tree, JSON, or YAML",
		Long: `Decode base64 or hexadecimal encoded XDR.

The XDR type is detected automatically by trying each known type and keeping
the ones that decode cleanly. Detection is reliable for transaction-sized
payloads, but short values are genuinely ambiguous — an empty Memo and a
native Asset are both "AAAAAA==" — so use --type when you need certainty and
--candidates to see what else matched.`,
		Example: `  lens decode "$XDR"
  lens decode --hex "$HEX"
  lens decode --multi -f envelopes.txt
  lens decode --yaml "$XDR"
  lens decode --wasm "$WASM_OR_ENVELOPE"
  lens decode --json "$XDR" | jq '.value.V1.Tx.Fee'
  lens decode --type TransactionResult "$XDR"
  lens decode --candidates "AAAAAA=="
  echo "$XDR" | lens decode --depth 3`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if isMulti {
				var r io.Reader
				if g.file != "" {
					f, err := os.Open(g.file)
					if err != nil {
						return fmt.Errorf("opening %s: %w", g.file, err)
					}
					defer f.Close()
					r = f
				} else if firstArg(args) != "" && firstArg(args) != stdinMarker {
					r = strings.NewReader(firstArg(args))
				} else {
					r = stdin
				}

				return runMultiStream(r, stdout, g, typeName, isHex, asJSON, asYAML, compactOut, annotate, maxDepth, showTypes)
			}

			payload, err := resolveInput(firstArg(args), g.file, stdin)
			if err != nil {
				return err
			}

			if isHex || strings.HasPrefix(payload, "0x") || strings.HasPrefix(payload, "0X") {
				b64, convErr := lens.HexToBase64(payload)
				if convErr == nil {
					payload = b64
				} else if isHex {
					return fmt.Errorf("decoding hex input: %w", convErr)
				}
			}

			if listCands {
				return runCandidates(stdout, payload)
			}

			value, err := decodeValue(payload, typeName)
			if err != nil {
				if inspectWASM {
					// Attempt direct WASM decode from payload
					return runDirectWASM(stdout, payload, asJSON)
				}
				return err
			}

			if inspectWASM {
				return runInspectWASM(stdout, value, payload, asJSON)
			}

			if asYAML {
				yamlBytes, err := lens.ValueToYAML(value, annotate)
				if err != nil {
					return fmt.Errorf("formatting YAML: %w", err)
				}
				_, err = stdout.Write(yamlBytes)
				return err
			}

			if asJSON {
				indent := "  "
				if compactOut {
					indent = ""
				}
				f := format.NewJSONFormatter(
					format.WithIndent(indent),
					format.WithAnnotations(annotate),
				)
				return f.Format(stdout, value)
			}

			f := format.NewTreeFormatter(
				format.WithPalette(format.PaletteFor(stdout, g.color)),
				format.WithMaxDepth(maxDepth),
				format.WithTypeNames(showTypes),
			)
			return f.Format(stdout, value)
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "emit stable JSON instead of a tree")
	cmd.Flags().BoolVar(&asYAML, "yaml", false, "emit formatted YAML instead of a tree")
	cmd.Flags().BoolVar(&isHex, "hex", false, "parse input as hexadecimal XDR instead of base64")
	cmd.Flags().BoolVar(&isMulti, "multi", false, "decode multiple newline-separated XDR payloads")
	cmd.Flags().BoolVar(&inspectWASM, "wasm", false, "extract and inspect embedded WebAssembly bytecode")
	cmd.Flags().BoolVar(&compactOut, "compact", false, "with --json, emit a single line")
	cmd.Flags().BoolVar(&annotate, "annotate", false,
		`with --json or --yaml, include enriched renderings as {"value":..., "display":...}`)
	cmd.Flags().StringVarP(&typeName, "type", "t", "",
		"decode as this XDR type instead of detecting it (see `lens types`)")
	cmd.Flags().IntVar(&maxDepth, "depth", 0, "limit tree depth (0 means unlimited)")
	cmd.Flags().BoolVar(&showTypes, "show-types", false, "annotate tree nodes with their XDR type")
	cmd.Flags().BoolVar(&listCands, "candidates", false,
		"list every XDR type the input decodes as, best first")

	return cmd
}

// decodeValue decodes a payload, honouring an explicit type when given.
func decodeValue(payload, typeName string) (*lens.Value, error) {
	if typeName != "" {
		v, err := lens.DecodeAs(payload, typeName)
		if err != nil {
			return nil, err
		}
		return v, nil
	}
	v, err := lens.Decode(payload)
	if err != nil {
		return nil, err
	}
	return v, nil
}

// runMultiStream decodes and formats multiple concatenated records.
func runMultiStream(r io.Reader, stdout io.Writer, g *globalFlags, typeName string, isHex, asJSON, asYAML, compactOut, annotate bool, maxDepth int, showTypes bool) error {
	opts := []lens.MultiStreamOption{
		lens.WithMultiStreamHex(isHex),
		lens.WithMultiStreamStopOnError(true),
	}
	if typeName != "" {
		opts = append(opts, lens.WithMultiStreamType(typeName))
	}

	decoder := lens.NewMultiStreamDecoder(r, opts...)
	records, err := decoder.DecodeAll()
	if err != nil {
		return err
	}

	if asYAML {
		for i, rec := range records {
			if i > 0 {
				if _, err := fmt.Fprintln(stdout, "---"); err != nil {
					return err
				}
			}
			y, err := lens.ValueToYAML(rec.Value, annotate)
			if err != nil {
				return err
			}
			if _, err := stdout.Write(y); err != nil {
				return err
			}
		}
		return nil
	}

	if asJSON {
		if compactOut {
			// NDJSON emission
			for _, rec := range records {
				var buf bytes.Buffer
				f := format.NewJSONFormatter(format.WithIndent(""), format.WithAnnotations(annotate))
				if err := f.Format(&buf, rec.Value); err != nil {
					return err
				}
				if _, err := fmt.Fprintf(stdout, "%s\n", strings.TrimSpace(buf.String())); err != nil {
					return err
				}
			}
			return nil
		}

		type multiJSONItem struct {
			Index      int    `json:"index"`
			LineNumber int    `json:"line_number"`
			Type       string `json:"type"`
			Value      any    `json:"value"`
		}
		items := make([]multiJSONItem, len(records))
		for i, rec := range records {
			items[i] = multiJSONItem{
				Index:      rec.Index,
				LineNumber: rec.LineNumber,
				Type:       rec.Value.Type,
				Value:      lens.NodeToGeneric(rec.Value.Node, annotate),
			}
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(items)
	}

	tf := format.NewTreeFormatter(
		format.WithPalette(format.PaletteFor(stdout, g.color)),
		format.WithMaxDepth(maxDepth),
		format.WithTypeNames(showTypes),
	)

	for _, rec := range records {
		if _, err := fmt.Fprintf(stdout, "=== Record %d (line %d) Type: %s ===\n", rec.Index, rec.LineNumber, rec.Value.Type); err != nil {
			return err
		}
		if err := tf.Format(stdout, rec.Value); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(stdout); err != nil {
			return err
		}
	}

	return nil
}

// runInspectWASM extracts and disassembles WebAssembly bytecode.
func runInspectWASM(stdout io.Writer, val *lens.Value, payload string, asJSON bool) error {
	wasmBytes, err := lens.ExtractWASMFromXDR(val)
	if err != nil {
		return runDirectWASM(stdout, payload, asJSON)
	}

	mod, err := lens.ParseWASM(wasmBytes)
	if err != nil {
		return fmt.Errorf("parsing extracted WebAssembly: %w", err)
	}

	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(mod)
	}

	_, err = fmt.Fprint(stdout, mod.Inspect())
	return err
}

// runDirectWASM attempts to parse a raw/hex/base64 payload directly as WASM.
func runDirectWASM(stdout io.Writer, payload string, asJSON bool) error {
	var wasmBytes []byte
	if lens.IsHexString(payload) {
		b, err := lens.HexToBytes(payload)
		if err == nil {
			wasmBytes = b
		}
	}
	if len(wasmBytes) == 0 {
		b, err := base64.StdEncoding.DecodeString(payload)
		if err == nil {
			wasmBytes = b
		} else {
			wasmBytes = []byte(payload)
		}
	}

	mod, err := lens.ParseWASM(wasmBytes)
	if err != nil {
		return fmt.Errorf("decoding WebAssembly payload: %w", err)
	}

	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(mod)
	}

	_, err = fmt.Fprint(stdout, mod.Inspect())
	return err
}

// runCandidates prints every type the payload decodes as, which is the honest
// answer for ambiguous input.
func runCandidates(stdout io.Writer, payload string) error {
	cands, err := lens.Detect(payload)
	if err != nil {
		return err
	}
	for i, c := range cands {
		marker := " "
		if i == 0 {
			marker = "*"
		}
		if _, err := fmt.Fprintf(stdout, "%s %s\n", marker, c.Type); err != nil {
			return fmt.Errorf("writing candidates: %w", err)
		}
	}
	if len(cands) > 1 {
		if _, err := fmt.Fprintf(stdout,
			"\n%d types matched; the starred one is used by default. Use --type to choose.\n",
			len(cands)); err != nil {
			return fmt.Errorf("writing candidates: %w", err)
		}
	}
	return nil
}

func newTypesCmd(stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "types",
		Short: "List the XDR types lens can decode",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintln(stdout, strings.Join(lens.TypeNames(), "\n"))
			return err
		},
	}
}

func firstArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}
