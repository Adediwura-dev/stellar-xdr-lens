package cli

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens"
)

type encodeOptions struct {
	targetType string
	strict     bool
	scval      bool
	envelope   bool
}

func newEncodeCmd(g *globalFlags, stdout io.Writer, stdin io.Reader) *cobra.Command {
	opts := &encodeOptions{}

	cmd := &cobra.Command{
		Use:   "encode [JSON]",
		Short: "Encode simplified JSON templates into base64 XDR",
		Long: `Encode JSON into base64-encoded XDR offline.

The command is symmetric with 'lens decode': it accepts the stable JSON
document that decode emits ({"type": ..., "value": ...}) and returns the
canonical base64 XDR for that type. Use --type to encode a bare value as a
registered XDR type.

TransactionEnvelope and ScVal also accept the simplified JSON dialects the
decoder has always understood:

  - TransactionEnvelope: full envelopes from simplified templates
  - ScVal: Soroban values from intuitive JSON literals

The input can be passed as a positional argument, read from a file with
--file / -f, or piped in via standard input.`,
		Example: `  # Round-trip a decode document straight back to XDR
  lens decode --json "$XDR" | lens encode

  # Encode a bare value as a named registered type
  lens encode --type ContractCodeEntry --file value.json

  # Encode a transaction envelope from a JSON template
  lens encode --file tx.json

  # Strict validation of transaction JSON schema
  lens encode --strict --file tx.json

  # Construct an ScVal from an intuitive JSON literal
  lens encode --scval '{"symbol": "hello"}'`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			arg := ""
			if len(args) > 0 {
				arg = args[0]
			}
			raw, err := resolveInput(arg, g.file, stdin)
			if err != nil {
				return err
			}
			jsonBytes := []byte(strings.TrimSpace(raw))
			if len(jsonBytes) == 0 {
				return errors.New("empty JSON input")
			}

			// Determine target type
			target := opts.targetType
			if opts.scval {
				target = "ScVal"
			} else if opts.envelope {
				target = "TransactionEnvelope"
			}

			// A full stable document carries its own type. Route it through
			// the symmetric encoder before shape detection can mistake it
			// for an ScVal literal.
			if target == "" && looksLikeDocument(jsonBytes) {
				encoded, err := lens.EncodeJSON(jsonBytes)
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(stdout, encoded)
				return err
			}

			if target == "" {
				target = detectEncodeType(jsonBytes)
			}

			switch strings.ToLower(target) {
			case "scval":
				if opts.strict {
					if err := lens.ValidateJSON(jsonBytes, lens.StrictScValSpecSchema()); err != nil {
						return fmt.Errorf("schema validation: %w", err)
					}
				}
				scVal, err := lens.ParseScValJSON(jsonBytes)
				if err != nil {
					return fmt.Errorf("encode scval: %w", err)
				}
				bin, err := scVal.MarshalBinary()
				if err != nil {
					return fmt.Errorf("marshal scval binary: %w", err)
				}
				_, err = fmt.Fprintln(stdout, base64.StdEncoding.EncodeToString(bin))
				return err

			case "transactionenvelope", "tx", "envelope":
				if opts.strict {
					env, err := lens.BuildTransactionEnvelopeFromJSONStrict(jsonBytes)
					if err != nil {
						return fmt.Errorf("strict encode transaction: %w", err)
					}
					bin, err := env.MarshalBinary()
					if err != nil {
						return fmt.Errorf("marshal envelope binary: %w", err)
					}
					_, err = fmt.Fprintln(stdout, base64.StdEncoding.EncodeToString(bin))
					return err
				}

				env, err := lens.BuildTransactionEnvelopeFromJSON(jsonBytes)
				if err != nil {
					return fmt.Errorf("encode transaction: %w", err)
				}
				bin, err := env.MarshalBinary()
				if err != nil {
					return fmt.Errorf("marshal envelope binary: %w", err)
				}
				_, err = fmt.Fprintln(stdout, base64.StdEncoding.EncodeToString(bin))
				return err

			default:
				// Any other registered type goes through the symmetric
				// structural encoder. An unknown name surfaces as
				// lens.ErrUnknownType.
				encoded, err := lens.EncodeJSONAs(target, jsonBytes)
				if err != nil {
					return fmt.Errorf("encode %s: %w", target, err)
				}
				_, err = fmt.Fprintln(stdout, encoded)
				return err
			}
		},
	}

	cmd.Flags().StringVarP(&opts.targetType, "type", "t", "",
		"target XDR type (a registered type name, TransactionEnvelope, or ScVal)")
	cmd.Flags().BoolVar(&opts.strict, "strict", false,
		"validate JSON input against strict schema definitions before encoding")
	cmd.Flags().BoolVar(&opts.scval, "scval", false,
		"shortcut to encode input as ScVal")
	cmd.Flags().BoolVar(&opts.envelope, "tx", false,
		"shortcut to encode input as TransactionEnvelope")

	return cmd
}

// looksLikeDocument reports whether data is a stable decode document: a JSON
// object carrying both a type name and a value. It is deliberately shallow so
// a malformed document falls through to the normal error paths.
func looksLikeDocument(data []byte) bool {
	var probe struct {
		Type  *string         `json:"type"`
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return false
	}
	return probe.Type != nil && *probe.Type != "" && len(probe.Value) > 0
}

func detectEncodeType(data []byte) string {
	var m map[string]any
	if err := json.Unmarshal(data, &m); err == nil {
		if _, ok := m["source_account"]; ok {
			return "TransactionEnvelope"
		}
		if _, ok := m["sourceAccount"]; ok {
			return "TransactionEnvelope"
		}
		if _, ok := m["operations"]; ok {
			return "TransactionEnvelope"
		}
		if _, ok := m["fee_bump"]; ok {
			return "TransactionEnvelope"
		}
	}
	return "ScVal"
}
