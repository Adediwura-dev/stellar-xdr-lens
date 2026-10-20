package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stellar-optics/stellar-xdr-lens/internal/cli"
)

func writeBatchFile(t *testing.T, dir, name, contents string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
	return path
}

func TestDecodeBatchDirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	valid := writeBatchFile(t, dir, "a-valid.env.txt", fixture(t, "tx_soroban.env"))
	bad := writeBatchFile(t, dir, "b-invalid.env.txt", "!!!! not base64 !!!!")
	empty := writeBatchFile(t, dir, "c-empty.env.txt", "")

	stdout, _, err := run(t, "", "decode", "--json", "--batch", dir)
	if err == nil {
		t.Fatal("Execute() error = nil, want aggregate batch error")
	}
	code, printable := cli.ExitCode(err)
	if code != cli.ExitError || !printable {
		t.Errorf("exit = (%d, printable=%v), want (%d, true); err=%v", code, printable, cli.ExitError, err)
	}
	for _, want := range []string{
		"=== " + valid + " ===",
		"=== " + bad + " ===",
		"=== " + empty + " ===",
		`"type": "TransactionEnvelope"`,
		"Batch summary: 3 file(s) processed, 1 succeeded, 2 failed",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("batch output missing %q\n%s", want, stdout)
		}
	}
	if strings.Index(stdout, valid) > strings.Index(stdout, bad) {
		t.Error("batch files are not reported in sorted order")
	}
}

func TestDecodeBatchGlobAllInvalidExitsInvalidXDR(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeBatchFile(t, dir, "one.xdr", "!!!!")
	writeBatchFile(t, dir, "two.xdr", "AAAAAA==")
	writeBatchFile(t, dir, "ignored.env.txt", fixture(t, "tx_soroban.env"))

	_, _, err := run(t, "", "decode", "--batch", filepath.Join(dir, "*.xdr"))
	if err == nil {
		t.Fatal("Execute() error = nil, want aggregate batch error")
	}
	code, _ := cli.ExitCode(err)
	if code != cli.ExitInvalidXDR {
		t.Errorf("exit = %d, want %d (all failures are invalid XDR); err=%v", code, cli.ExitInvalidXDR, err)
	}
}

func TestDecodeBatchRejectsConflictingInputs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeBatchFile(t, dir, "one.xdr", fixture(t, "tx_soroban.env"))

	tests := []struct {
		name string
		args []string
	}{
		{"with stream", []string{"decode", "--batch", dir, "--stream"}},
		{"with multi", []string{"decode", "--batch", dir, "--multi"}},
		{"with interactive", []string{"decode", "--batch", dir, "--interactive"}},
		{"with file", []string{"decode", "--batch", dir, "--file", fixturePath("tx_soroban.env")}},
		{"with payload", []string{"decode", "--batch", dir, "AAAAAA=="}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := run(t, "", tc.args...)
			if err == nil {
				t.Fatal("Execute() error = nil, want a usage error")
			}
			if code, _ := cli.ExitCode(err); code != cli.ExitError {
				t.Errorf("exit = %d, want %d; err=%v", code, cli.ExitError, err)
			}
		})
	}
}

func TestQuietSuppressesOutputButKeepsExitStatus(t *testing.T) {
	t.Parallel()

	stdout, _, err := run(t, "", "--quiet", "decode", fixture(t, "tx_soroban.env"))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stdout != "" {
		t.Errorf("--quiet stdout = %q, want empty", stdout)
	}

	stdout, _, err = run(t, "", "--quiet", "diff",
		fixture(t, "tx_soroban.env"), fixture(t, "tx_failed.env"))
	if err == nil {
		t.Fatal("diff --quiet error = nil, want values differ")
	}
	if code, _ := cli.ExitCode(err); code != cli.ExitDiffFound {
		t.Errorf("diff --quiet exit = %d, want %d; err=%v", code, cli.ExitDiffFound, err)
	}
	if stdout != "" {
		t.Errorf("diff --quiet stdout = %q, want empty", stdout)
	}
}

func TestOutputWritesFileAtomically(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "decoded.txt")
	stdout, _, err := run(t, "", "decode", "--output", path, fixture(t, "tx_soroban.env"))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty when --output is used", stdout)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading --output file: %v", err)
	}
	if !strings.Contains(string(written), "TransactionEnvelope") {
		t.Errorf("--output file does not contain the report:\n%s", written)
	}
	if _, err := os.Stat(filepath.Join(dir, "."+filepath.Base(path)+".tmp-")); err == nil {
		t.Error("temporary output file was left behind")
	}

	// A failing command must not replace an existing good file.
	if err := os.WriteFile(path, []byte("previous contents"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err = run(t, "", "diff", "--exit-code", "--output", path,
		fixture(t, "tx_soroban.env"), fixture(t, "tx_failed.env"))
	if err == nil {
		t.Fatal("diff --exit-code error = nil, want values differ")
	}
	kept, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(kept) != "previous contents" {
		t.Errorf("failed command replaced output with %q", kept)
	}
}

func TestOutputDashMeansStdout(t *testing.T) {
	t.Parallel()

	stdout, _, err := run(t, "", "decode", "-o", "-", fixture(t, "tx_soroban.env"))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout, "TransactionEnvelope") {
		t.Errorf("-o - stdout does not contain the report:\n%s", stdout)
	}
}

func TestExplainResources(t *testing.T) {
	t.Parallel()

	stdout, _, err := run(t, "", "explain", "--resources", fixture(t, "tx_soroban.env"))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, want := range []string{
		"Soroban resource breakdown",
		"Instructions (CPU)",
		"6,688,817",
		"400,000,000",
		"Disk read entries",
		"All declared resources are within",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("--resources output missing %q\n%s", want, stdout)
		}
	}
}

func TestExplainResourcesJSON(t *testing.T) {
	t.Parallel()

	stdout, _, err := run(t, "", "explain", "--json", "--resources", fixture(t, "tx_soroban.env"))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	var doc struct {
		Kind      string `json:"kind"`
		Resources *struct {
			Rows []struct {
				Resource string `json:"resource"`
				Used     int64  `json:"used"`
			} `json:"rows"`
		} `json:"resources"`
	}
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("--resources --json output is not valid JSON: %v\n%s", err, stdout)
	}
	if doc.Kind == "" {
		t.Error("JSON summary lost its kind field")
	}
	if doc.Resources == nil || len(doc.Resources.Rows) == 0 {
		t.Fatalf("JSON summary has no resource rows:\n%s", stdout)
	}
	found := false
	for _, row := range doc.Resources.Rows {
		if row.Resource == "Instructions (CPU)" && row.Used == 6_688_817 {
			found = true
		}
	}
	if !found {
		t.Errorf("JSON resource rows do not report 6,688,817 instructions:\n%s", stdout)
	}
}
