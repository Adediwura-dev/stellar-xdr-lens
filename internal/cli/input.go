package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens"
)

// errNoInput is returned when no payload could be resolved from any source.
var errNoInput = errors.New("no XDR input given: pass a base64 value, use --file, or pipe to stdin")

// stdinMarker is the conventional argument meaning "read from stdin".
const stdinMarker = "-"

// resolveInput returns an XDR payload from, in order of precedence:
//
//  1. the --file flag, when set;
//  2. the positional argument, unless it is "-";
//  3. standard input, when it is not a terminal or the argument was "-".
//
// Reading from stdin lets the tool sit in a pipeline, which is the point of
// having a CLI rather than a web form:
//
//	curl ... | jq -r .envelope_xdr | lens explain
func resolveInput(arg, file string, stdin io.Reader) (string, error) {
	if file != "" {
		b, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("reading %s: %w", file, err)
		}
		payload := strings.TrimSpace(string(b))
		if payload == "" {
			return "", fmt.Errorf("%s is empty", file)
		}
		return payload, nil
	}

	if arg != "" && arg != stdinMarker {
		return strings.TrimSpace(arg), nil
	}

	if arg == stdinMarker || !isTerminal(os.Stdin) {
		// lens.ReadStdinPayload is the buffered stdin pipe: one pre-sized
		// pass over the stream, no second copy. Its ErrEmptyInput models
		// "the pipeline handed us nothing", which stays errNoInput.
		payload, err := lens.ReadStdinPayload(stdin)
		if errors.Is(err, lens.ErrEmptyInput) {
			return "", errNoInput
		}
		if err != nil {
			return "", fmt.Errorf("reading stdin: %w", err)
		}
		return payload, nil
	}

	return "", errNoInput
}

// isTerminal reports whether f is attached to a character device, which is
// how we tell an interactive shell from a pipe without pulling in a
// dependency for it.
func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// resolveBatchFiles expands a --batch argument into a deterministic, ordered
// list of files. The argument is either a directory (every regular file in it,
// sorted by name) or a glob pattern (every matching regular file, sorted).
func resolveBatchFiles(pattern string) ([]string, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return nil, fmt.Errorf("%w: --batch needs a directory or glob pattern", lens.ErrInvalidArgument)
	}

	if info, err := os.Stat(pattern); err == nil && info.IsDir() {
		entries, err := os.ReadDir(pattern)
		if err != nil {
			return nil, fmt.Errorf("reading batch directory %s: %w", pattern, err)
		}
		files := make([]string, 0, len(entries))
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			files = append(files, filepath.Join(pattern, entry.Name()))
		}
		if len(files) == 0 {
			return nil, fmt.Errorf("%w: batch directory %s contains no files", lens.ErrInvalidArgument, pattern)
		}
		sort.Strings(files)
		return files, nil
	}

	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid batch pattern %q: %w", lens.ErrInvalidArgument, pattern, err)
	}
	files := make([]string, 0, len(matches))
	for _, match := range matches {
		info, statErr := os.Stat(match)
		if statErr != nil {
			return nil, fmt.Errorf("reading batch match %s: %w", match, statErr)
		}
		if info.IsDir() {
			continue
		}
		files = append(files, match)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("%w: batch pattern %q matched no files", lens.ErrInvalidArgument, pattern)
	}
	sort.Strings(files)
	return files, nil
}

// batchFailure records one file that could not be decoded in batch mode.
type batchFailure struct {
	path string
	err  error
}

// batchError aggregates per-file failures. It unwraps to the first failure
// only when every failure is an invalid-XDR error, so the CLI can still exit
// with code 3 for a batch of unreadable payloads without misclassifying a
// read or formatting failure as invalid XDR.
type batchError struct {
	total  int
	failed []batchFailure
	cause  error
}

func newBatchError(total int, failed []batchFailure) error {
	allInvalid := len(failed) > 0
	for _, failure := range failed {
		if !isInvalidXDRError(failure.err) {
			allInvalid = false
			break
		}
	}
	e := &batchError{total: total, failed: failed}
	if allInvalid {
		e.cause = failed[0].err
	}
	return e
}

func (e *batchError) Error() string {
	names := make([]string, 0, len(e.failed))
	for _, failure := range e.failed {
		names = append(names, filepath.Base(failure.path))
	}
	return fmt.Sprintf("batch: %d of %d file(s) failed: %s",
		len(e.failed), e.total, strings.Join(names, ", "))
}

func (e *batchError) Unwrap() error { return e.cause }
