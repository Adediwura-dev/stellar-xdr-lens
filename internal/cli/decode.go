package cli

import (
	"encoding/base64"
	"errors"
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
		asJSON     bool
		typeName   string
		maxDepth   int
		showTypes  bool
		annotate   bool
		listCands  bool
		compactOut bool
		stream     bool
		canonical  bool
	)

	cmd := &cobra.Command{
		Use:   "decode [base64]",
		Short: "Decode XDR and print it as a tree or as JSON",
		Long: `Decode base64-encoded XDR.

The XDR type is detected automatically by trying each known type and keeping
the ones that decode cleanly. Detection is reliable for transaction-sized
payloads, but short values are genuinely ambiguous — an empty Memo and a
native Asset are both "AAAAAA==" — so use --type when you need certainty and
--candidates to see what else matched.`,
		Example: `  lens decode "$XDR"
  lens decode --json "$XDR" | jq '.value.V1.Tx.Fee'
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
	cmd.Flags().BoolVar(&compactOut, "compact", false, "with --json, emit a single line")
	cmd.Flags().BoolVar(&annotate, "annotate", false,
		`with --json, include enriched renderings as {"value":..., "display":...}`)
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

func runStream(stdout io.Writer, g *globalFlags, arg string, stdin io.Reader, typeName string, asJSON, compactOut, annotate bool, maxDepth int, showTypes, canonical bool) error {
	var r io.Reader
	if g.file != "" {
		f, err := os.Open(g.file)
		if err != nil {
			return err
		}
		defer f.Close()
		r = f
	} else if arg != "" && arg != stdinMarker {
		r = strings.NewReader(arg)
	} else {
		r = stdin
	}

	parser := lens.NewStreamParser(r)
	for {
		raw, err := parser.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}

		if canonical {
			if typeName != "" {
				if err := lens.ValidateTypeCanonicalXDR(raw, typeName); err != nil {
					return fmt.Errorf("chunk [%d] canonical error: %w", parser.ChunkIndex()-1, err)
				}
			} else {
				if err := lens.ValidateCanonicalXDR(raw); err != nil {
					return fmt.Errorf("chunk [%d] canonical error: %w", parser.ChunkIndex()-1, err)
				}
			}
		}

		b64 := base64.StdEncoding.EncodeToString(raw)
		val, err := decodeValue(b64, typeName)
		if err != nil {
			return fmt.Errorf("chunk [%d]: %w", parser.ChunkIndex()-1, err)
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
			if err := f.Format(stdout, val); err != nil {
				return err
			}
			continue
		}

		f := format.NewTreeFormatter(
			format.WithPalette(format.PaletteFor(stdout, g.color)),
			format.WithMaxDepth(maxDepth),
			format.WithTypeNames(showTypes),
		)
		if err := f.Format(stdout, val); err != nil {
			return err
		}
	}
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
