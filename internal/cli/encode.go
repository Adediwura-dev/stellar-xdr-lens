package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens"
)

func newEncodeCmd(g *globalFlags, stdout io.Writer, stdin io.Reader) *cobra.Command {
	var (
		typeName string
		asHex    bool
	)

	cmd := &cobra.Command{
		Use:   "encode [spec.yaml|spec.json|-]",
		Short: "Encode a YAML or JSON specification into base64 or hexadecimal XDR",
		Long: `Encode a YAML or JSON specification into base64 or hexadecimal XDR.

Supports human-friendly configuration files in YAML or JSON, as well as
raw base64/hex strings in structured document fields.`,
		Example: `  lens encode spec.yaml
  lens encode -f spec.yaml --hex
  echo 'type: Memo\nvalue: test' | lens encode
  lens encode --type TransactionEnvelope tx.yaml`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			input, err := resolveInput(firstArg(args), g.file, stdin)
			if err != nil {
				return err
			}

			_, b64, err := lens.YAMLToXDR([]byte(input), typeName)
			if err != nil {
				return fmt.Errorf("encoding input to XDR: %w", err)
			}

			if asHex {
				hexStr, err := lens.Base64ToHex(b64)
				if err != nil {
					return fmt.Errorf("converting XDR to hex: %w", err)
				}
				_, err = fmt.Fprintln(stdout, hexStr)
				return err
			}

			_, err = fmt.Fprintln(stdout, b64)
			return err
		},
	}

	cmd.Flags().StringVarP(&typeName, "type", "t", "", "XDR type name to encode")
	cmd.Flags().BoolVar(&asHex, "hex", false, "emit hexadecimal XDR instead of base64")

	return cmd
}
