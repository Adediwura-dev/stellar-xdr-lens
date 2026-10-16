package cli

import (
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
		asSummary   bool
		asTxRep     bool
		interactive bool
		specPath    string
		formatName  string
		network     string
		typeName    string
		maxDepth    int
		showTypes   bool
		annotate    bool
		listCands   bool
		compactOut  bool
	)

	cmd := &cobra.Command{
		Use:   "decode [base64]",
		Short: "Decode XDR and print it as a tree, JSON, summary, or txrep",
		Long: `Decode base64-encoded XDR.

The XDR type is detected automatically by trying each known type and keeping
the ones that decode cleanly. Detection is reliable for transaction-sized
payloads, but short values are genuinely ambiguous — an empty Memo and a
native Asset are both "AAAAAA==" — so use --type when you need certainty and
--candidates to see what else matched.

Use --summary for single-line output suited for grep and shell pipelines.
Use --txrep for SEP-0011 human-readable representation of transaction envelopes.
Use --spec to supply a contract WASM or spec file for decoded Soroban contract fields.
Use --interactive (or 'lens explore') for an interactive terminal tree viewer.`,
		Example: `  lens decode "$XDR"
  lens decode --json "$XDR" | jq '.value.V1.Tx.Fee'
  lens decode --summary "$XDR"
  lens decode --txrep "$XDR"
  lens decode --spec contract.wasm "$XDR"
  lens decode --interactive "$XDR"
  lens decode --type TransactionResult "$XDR"
  lens decode --candidates "AAAAAA=="
  lens decode --stream --file records.xdr
  lens decode --canonical "$XDR"
  echo "$XDR" | lens decode --depth 3`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if stream {
				return runStream(stdout, g, firstArg(args), stdin, typeName, asJSON, compactOut, annotate, maxDepth, showTypes, canonical)
			}

			payload, err := resolveInput(firstArg(args), g.file, stdin)
			if err != nil {
				return err
			}

			if canonical {
				raw, decErr := base64.StdEncoding.DecodeString(payload)
				if decErr != nil {
					return fmt.Errorf("decoding base64: %w", decErr)
				}
				if typeName != "" {
					if err := lens.ValidateTypeCanonicalXDR(raw, typeName); err != nil {
						return fmt.Errorf("canonical validation failed: %w", err)
					}
				} else {
					if err := lens.ValidateCanonicalXDR(raw); err != nil {
						return fmt.Errorf("canonical validation failed: %w", err)
					}
				}
			}

			if listCands {
				return runCandidates(stdout, payload)
			}

			value, err := decodeValue(payload, typeName)
			if err != nil {
				return err
			}

			if specPath != "" {
				spec, err := lens.LoadContractSpecFile(specPath)
				if err != nil {
					return fmt.Errorf("loading contract spec: %w", err)
				}
				spec.ApplyToValue(value)
			}

			if interactive {
				tui := NewTUITree(value, stdin, stdout, format.PaletteFor(stdout, g.color))
				return tui.Run()
			}

			if asSummary || formatName == "summary" {
				f := format.NewCompactFormatter(
					format.WithCompactPalette(format.PaletteFor(stdout, g.color)),
				)
				return f.Format(stdout, value)
			}

			if asTxRep || formatName == "txrep" {
				f := format.NewTxRepFormatter(
					format.WithTxRepPalette(format.PaletteFor(stdout, g.color)),
					format.WithTxRepNetwork(network),
				)
				return f.Format(stdout, value)
			}

			if asJSON || formatName == "json" {
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
	cmd.Flags().BoolVar(&compactOut, "compact", false, "with --json, emit a single line")
	cmd.Flags().BoolVar(&asSummary, "summary", false, "emit a terse single-line summary suitable for grep and pipelines")
	cmd.Flags().BoolVar(&asTxRep, "txrep", false, "emit SEP-0011 TxRep format for transaction envelopes")
	cmd.Flags().BoolVarP(&interactive, "interactive", "i", false, "explore the decoded tree interactively")
	cmd.Flags().StringVar(&specPath, "spec", "", "path to contract WASM or ScSpecEntry file for spec-aware decoding")
	cmd.Flags().StringVar(&formatName, "format", "", `output format: "tree", "json", "summary", or "txrep"`)
	cmd.Flags().StringVar(&network, "network", "", "network passphrase for txrep formatting")
	cmd.Flags().StringVarP(&typeName, "type", "t", "",
		"decode as this XDR type instead of detecting it (see `lens types`)")
	cmd.Flags().IntVar(&maxDepth, "depth", 0, "limit tree depth (0 means unlimited)")
	cmd.Flags().BoolVar(&showTypes, "show-types", false, "annotate tree nodes with their XDR type")
	cmd.Flags().BoolVar(&listCands, "candidates", false,
		"list every XDR type the input decodes as, best first")
	cmd.Flags().BoolVar(&stream, "stream", false, "stream and decode framed XDR chunks (RFC 5531 or length-prefixed)")
	cmd.Flags().BoolVar(&canonical, "canonical", false, "verify that input adheres strictly to canonical XDR serialization")

	return cmd
}

func newExploreCmd(g *globalFlags, stdout io.Writer, stdin io.Reader) *cobra.Command {
	var (
		typeName string
		specPath string
	)

	cmd := &cobra.Command{
		Use:     "explore [base64]",
		Aliases: []string{"tui"},
		Short:   "Interactively explore an XDR structure as an expandable tree",
		Long: `Interactively explore an XDR structure in an expandable terminal tree viewer.

Use arrow keys or j/k to navigate, Enter or Space to toggle node expansion,
'e' to expand all, 'c' to collapse all, and 'q' to quit.`,
		Example: `  lens explore "$XDR"
  lens tui --file tx.env.txt
  lens explore --spec contract.wasm "$XDR"`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			payload, err := resolveInput(firstArg(args), g.file, stdin)
			if err != nil {
				return err
			}

			value, err := decodeValue(payload, typeName)
			if err != nil {
				return err
			}

			if specPath != "" {
				spec, err := lens.LoadContractSpecFile(specPath)
				if err != nil {
					return fmt.Errorf("loading contract spec: %w", err)
				}
				spec.ApplyToValue(value)
			}

			tui := NewTUITree(value, stdin, stdout, format.PaletteFor(stdout, g.color))
			return tui.Run()
		},
	}

	cmd.Flags().StringVarP(&typeName, "type", "t", "",
		"decode as this XDR type instead of detecting it")
	cmd.Flags().StringVar(&specPath, "spec", "",
		"path to contract WASM or ScSpecEntry file for spec-aware decoding")

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
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "types",
		Short: "List the XDR types lens can decode",
		Long: `List every XDR type lens can decode.

With --json the command emits the introspection index: the name, category,
one-line description, detection priority and minimum encoded size of each type.
The shape is stable and intended for scripts.`,
		Example: `  lens types
  lens types --json | jq '.[] | select(.category == "overlay") | .name'`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if asJSON {
				enc := json.NewEncoder(stdout)
				enc.SetIndent("", "  ")
				if err := enc.Encode(lens.RegistryIndex()); err != nil {
					return fmt.Errorf("encoding type index: %w", err)
				}
				return nil
			}
			_, err := fmt.Fprintln(stdout, strings.Join(lens.TypeNames(), "\n"))
			return err
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "emit the introspection index as JSON")

	return cmd
}

func firstArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}
