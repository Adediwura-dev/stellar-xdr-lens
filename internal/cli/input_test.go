package cli

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens"
)

func TestResolveBatchFilesDirectoryIsSortedAndSkipsDirectories(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	for _, name := range []string{"b.xdr", "a.xdr", "c.xdr"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := resolveBatchFiles(dir)
	if err != nil {
		t.Fatalf("resolveBatchFiles() error = %v", err)
	}
	want := []string{
		filepath.Join(dir, "a.xdr"),
		filepath.Join(dir, "b.xdr"),
		filepath.Join(dir, "c.xdr"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("files = %v, want %v", got, want)
	}
}

func TestResolveBatchFilesGlobIsSorted(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	for _, name := range []string{"z.env.txt", "a.env.txt", "skip.xdr"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got, err := resolveBatchFiles(filepath.Join(dir, "*.env.txt"))
	if err != nil {
		t.Fatalf("resolveBatchFiles() error = %v", err)
	}
	want := []string{
		filepath.Join(dir, "a.env.txt"),
		filepath.Join(dir, "z.env.txt"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("files = %v, want %v", got, want)
	}
}

func TestResolveBatchFilesErrors(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	emptyDir := filepath.Join(dir, "empty")
	if err := os.Mkdir(emptyDir, 0o755); err != nil {
		t.Fatal(err)
	}

	tests := []string{
		"",
		filepath.Join(dir, "*.none"),
		filepath.Join(dir, "["),
		emptyDir,
	}
	for _, pattern := range tests {
		if _, err := resolveBatchFiles(pattern); !errors.Is(err, lens.ErrInvalidArgument) {
			t.Errorf("resolveBatchFiles(%q) error = %v, want ErrInvalidArgument", pattern, err)
		}
	}
}
