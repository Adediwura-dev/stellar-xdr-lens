package lens_test

// Corpus synchronizer for the testdata regression suite (roadmap #81).
//
// The offline-first rule of this repository holds everywhere in the default
// `go test` run: the always-running tests below (the manifest checks, the
// selection and dedupe logic, the fixture decode sweep) read only the local
// testdata directory and never open a network socket. Reaching public
// Testnet is deliberately opt-in, as the roadmap requires for any optional
// network step, and is confined to this test file:
//
//	LENS_CORPUS_SYNC=1      verify the fetch pipeline end to end against
//	                        Testnet; nothing on disk changes
//	LENS_CORPUS_SYNC=write  additionally capture new fixtures into
//	                        testdata/ and record them in manifest.json
//
// The synchronizer keeps the two invariants the manifest of record demands
// of every captured fixture: the payload must decode with this library, and
// the entry must carry the transaction hash so it traces back to the ledger.
// Selection keeps one fixture per unseen transaction, sorted by hash, which
// makes repeated runs deterministic: plan output depends only on the page
// content and the current manifest.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	lens "github.com/stellar-optics/stellar-xdr-lens/pkg/lens"
)

// corpusTestnetHorizon is the collection the synchronizer reads from. It is
// pinned in source so the scope of any run is visible in the code.
const corpusTestnetHorizon = "https://horizon-testnet.stellar.org"

// corpusSyncEnv gates the opt-in run: unset/empty means skip, "1" is a
// dry run, and corpusSyncWriteMode additionally records new fixtures on
// disk.
const corpusSyncEnv = "LENS_CORPUS_SYNC"

// corpusSyncWriteMode is the corpusSyncEnv value that authorizes writing.
const corpusSyncWriteMode = "write"

// corpusManifest mirrors testdata/manifest.json. The JSON field names are
// the on-disk format of record, so they are fixed here even where Go
// conventions would suggest renaming.
type corpusManifest struct {
	Source    string                         `json:"source"`
	Captured  string                         `json:"captured,omitempty"`
	Note      string                         `json:"note,omitempty"`
	Fixtures  map[string]corpusFixtureRecord `json:"fixtures"`
	Synthetic *struct {
		Generated string                            `json:"generated,omitempty"`
		Note      string                            `json:"note,omitempty"`
		Fixtures  map[string]syntheticFixtureRecord `json:"fixtures"`
	} `json:"synthetic_fixtures,omitempty"`
}

// corpusFixtureRecord is one captured transaction entry in the manifest.
type corpusFixtureRecord struct {
	TxHash         string `json:"tx_hash"`
	Description    string `json:"description"`
	Successful     *bool  `json:"successful"`
	OperationCount int    `json:"operation_count"`
	FeeBump        bool   `json:"fee_bump"`
	EnvelopeFile   string `json:"envelope_file"`
	ResultFile     string `json:"result_file"`
}

// syntheticFixtureRecord is a constructed (non-captured) fixture entry. The
// file name is the manifest key plus ".txt".
type syntheticFixtureRecord struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

// horizonTxRecord is the part of a Horizon transaction summary the plan
// reads.
type horizonTxRecord struct {
	Hash           string `json:"hash"`
	EnvelopeXdr    string `json:"envelope_xdr"`
	ResultXdr      string `json:"result_xdr"`
	Successful     bool   `json:"successful"`
	OperationCount int    `json:"operation_count"`
}

// horizonTxPage is the page envelope of the /transactions collection.
type horizonTxPage struct {
	Embedded struct {
		Records []horizonTxRecord `json:"records"`
	} `json:"_embedded"`
}

// corpusClient fetches a URL and returns its body. The indirection exists
// so the selection and dedupe logic is testable with no network at all:
// unit tests inject a slice-based stub; the gated run injects HTTP.
type corpusClient interface {
	Get(ctx context.Context, url string) ([]byte, error)
}

// httpCorpusClient is the production implementation: a bounded HTTP client
// used only inside the environment-gated run.
type httpCorpusClient struct{ hc *http.Client }

func newHTTPCorpusClient() *httpCorpusClient {
	return &httpCorpusClient{hc: &http.Client{Timeout: 30 * time.Second}}
}

func (c httpCorpusClient) Get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching %s: status %d", url, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// corpusPlan is the deterministic result of selection: the retained entries
// in hash order, with the manifest records that would be written for them.
type corpusPlan struct {
	Entries  []corpusEntry
	Manifest corpusManifest
}

// corpusEntry is one retained candidate together with the fixture names
// planCorpus assigned to it.
type corpusEntry struct {
	Record      horizonTxRecord
	EnvelopeKey string
	Envelope    string
	Result      string
}

// fetchCorpusPage reads the most recent `limit` transactions from the
// Horizon transactions collection. limit <= 0 is an argument error, which
// keeps the builder honest about its own inputs.
func fetchCorpusPage(ctx context.Context, c corpusClient, limit int) ([]horizonTxRecord, error) {
	if c == nil {
		return nil, lens.ErrEmptyInput
	}
	if limit <= 0 {
		return nil, fmt.Errorf("corpus page limit must be positive, got %d: %w", limit, lens.ErrInvalidArgument)
	}
	raw, err := c.Get(ctx, corpusTestnetHorizon+"/transactions?order=desc&limit="+strconv.Itoa(limit))
	if err != nil {
		return nil, err
	}
	var page horizonTxPage
	if err := json.Unmarshal(raw, &page); err != nil {
		return nil, fmt.Errorf("parsing transaction page: %w", err)
	}
	return page.Embedded.Records, nil
}

// planCorpus turns Horizon records into the deterministic plan of new
// fixtures. Every candidate is decoded before selection — a fixture that
// does not decode would be dead weight in the regression suite — duplicates
// already in the manifest or repeated in the batch are dropped, and names
// derive from the transaction hash. It never writes; it only allocates the
// plan. The existing manifest is not mutated.
func planCorpus(records []horizonTxRecord, existing corpusManifest) (*corpusPlan, int, int, error) {
	if existing.Fixtures == nil {
		existing.Fixtures = map[string]corpusFixtureRecord{}
	}

	knownHash := map[string]bool{}
	for _, rec := range existing.Fixtures {
		if rec.TxHash != "" {
			knownHash[strings.ToLower(rec.TxHash)] = true
		}
	}

	// Pass 1: validate every unseen record and deduplicate by envelope
	// bytes, keeping the smallest hash of each group.
	kept, dropped := 0, 0
	bestByEnvelope := map[string]horizonTxRecord{}
	for _, rec := range records {
		if rec.Hash == "" || rec.EnvelopeXdr == "" {
			dropped++
			continue
		}
		hash := strings.ToLower(rec.Hash)
		if knownHash[hash] {
			dropped++
			continue
		}
		envelope := strings.TrimSpace(rec.EnvelopeXdr)
		value, decErr := lens.Decode(envelope)
		if decErr != nil || value.Type != "TransactionEnvelope" {
			// Not representative XDR: not fatal, just not a fixture.
			dropped++
			continue
		}
		if result := strings.TrimSpace(rec.ResultXdr); result != "" {
			if _, decErr := lens.Decode(result); decErr != nil {
				dropped++
				continue
			}
		}
		if prev, seen := bestByEnvelope[envelope]; seen {
			// The same envelope under a second hash: only the smaller hash
			// stays, and it supplies the counts no matter which record the
			// page listed first.
			if hash < strings.ToLower(prev.Hash) {
				bestByEnvelope[envelope] = rec
			}
			dropped++
			continue
		}
		bestByEnvelope[envelope] = rec
	}
	kept = len(bestByEnvelope)

	// Pass 2: name the retained entries. Sorting by hash before attaching
	// fixture names keeps the plan deterministic run to run.
	plan := &corpusPlan{Manifest: corpusManifest{
		Source:   corpusTestnetHorizon,
		Captured: time.Now().UTC().Format("2006-01-02"),
		Fixtures: map[string]corpusFixtureRecord{},
	}}
	candidates := make([]horizonTxRecord, 0, len(bestByEnvelope))
	for _, rec := range bestByEnvelope {
		candidates = append(candidates, rec)
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Hash < candidates[j].Hash
	})

	for _, rec := range candidates {
		hash := strings.ToLower(rec.Hash)
		envelope := strings.TrimSpace(rec.EnvelopeXdr)
		result := strings.TrimSpace(rec.ResultXdr)
		value, _ := lens.Decode(envelope)
		key := corpusFixtureKey(hash)
		plan.Entries = append(plan.Entries, corpusEntry{
			Record:      rec,
			EnvelopeKey: key,
			Envelope:    envelope,
			Result:      result,
		})
		plan.Manifest.Fixtures[key] = corpusFixtureRecord{
			TxHash:         hash,
			Description:    fmt.Sprintf("captured testnet TransactionEnvelope with %d operation(s)", countEnvelopeOperations(value)),
			Successful:     boolPtr(rec.Successful),
			OperationCount: countEnvelopeOperations(value),
			FeeBump:        decodedTreeHasNodeNamed(value, "FeeBump"),
			EnvelopeFile:   key + ".env.txt",
			ResultFile:     corpusResultFieldName(result, key),
		}
	}
	return plan, kept, dropped, nil
}

// corpusFixtureKey is the deterministic manifest key for a captured
// transaction: a sanitized hash prefix. Full-hash deduplication upstream
// makes the 16-character prefix unique within the corpus.
func corpusFixtureKey(hashHex string) string {
	clean := strings.ToLower(corpusNameSanitize.ReplaceAllString(strings.ToLower(hashHex), ""))
	if len(clean) > 16 {
		clean = clean[:16]
	}
	return "testnet_" + clean
}

// corpusNameSanitize strips anything a fixture key may not contain, so a
// hash with unexpected characters cannot escape the fixture directory.
var corpusNameSanitize = regexp.MustCompile(`[^a-f0-9]`)

// corpusResultFieldName records a result fixture only when there is one.
func corpusResultFieldName(result, key string) string {
	if result == "" {
		return ""
	}
	return key + ".res.txt"
}

// countEnvelopeOperations reads the Operations list out of a decoded
// TransactionEnvelope tree, which keeps the count honest across v0, v1 and
// fee-bump shapes.
func countEnvelopeOperations(v *lens.Value) int {
	node := findNodeNamedInTree(v.Node, "Operations")
	if node == nil {
		return 0
	}
	return len(node.Children)
}

// decodedTreeHasNodeNamed reports whether the decoded tree contains a child
// with the given field name, which is how a fee-bump arm is recognised
// without SDK helpers.
func decodedTreeHasNodeNamed(v *lens.Value, name string) bool {
	return findNodeNamedInTree(v.Node, name) != nil
}

func findNodeNamedInTree(node *lens.Node, name string) *lens.Node {
	if node == nil {
		return nil
	}
	if node.Name == name {
		return node
	}
	for _, child := range node.Children {
		if hit := findNodeNamedInTree(child, name); hit != nil {
			return hit
		}
	}
	return nil
}

func boolPtr(v bool) *bool { return &v }

// loadCorpusManifest reads testdata/manifest.json from disk.
func loadCorpusManifest(t *testing.T) corpusManifest {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(fixtureDir, "manifest.json"))
	if err != nil {
		t.Fatalf("reading manifest.json: %v", err)
	}
	var m corpusManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("parsing manifest.json: %v", err)
	}
	return m
}

// writeCorpusPlan commits a plan into testdata: fixture files first, then
// the merged manifest. Existing entries are preserved and existing files
// are never overwritten. Only the write-enabled gated run calls this.
func writeCorpusPlan(t *testing.T, thePlan *corpusPlan) ([]string, error) {
	t.Helper()
	created := []string{}
	for _, entry := range thePlan.Entries {
		rec := thePlan.Manifest.Fixtures[entry.EnvelopeKey]
		envPath := filepath.Join(fixtureDir, rec.EnvelopeFile)
		if _, statErr := os.Stat(envPath); errors.Is(statErr, os.ErrNotExist) {
			if writeErr := os.WriteFile(envPath, []byte(entry.Envelope+"\n"), 0o600); writeErr != nil {
				return created, fmt.Errorf("writing %s: %w", envPath, writeErr)
			}
			created = append(created, rec.EnvelopeFile)
		}
		if entry.Result != "" && rec.ResultFile != "" {
			resPath := filepath.Join(fixtureDir, rec.ResultFile)
			if _, statErr := os.Stat(resPath); errors.Is(statErr, os.ErrNotExist) {
				if writeErr := os.WriteFile(resPath, []byte(entry.Result+"\n"), 0o600); writeErr != nil {
					return created, fmt.Errorf("writing %s: %w", resPath, writeErr)
				}
				created = append(created, rec.ResultFile)
			}
		}
	}

	// Merge into the on-disk manifest so prior entries are preserved.
	onDisk := loadCorpusManifest(t)
	if onDisk.Fixtures == nil {
		onDisk.Fixtures = map[string]corpusFixtureRecord{}
	}
	for key, rec := range thePlan.Manifest.Fixtures {
		if _, exists := onDisk.Fixtures[key]; !exists {
			onDisk.Fixtures[key] = rec
		}
	}
	manifestBytes, err := json.MarshalIndent(onDisk, "", "  ")
	if err != nil {
		return created, fmt.Errorf("marshalling manifest: %w", err)
	}
	return created, os.WriteFile(filepath.Join(fixtureDir, "manifest.json"), append(manifestBytes, '\n'), 0o600)
}

// stubCorpusClient answers every request from an in-memory page, which is
// how the plan logic is exercised with zero network involvement.
type stubCorpusClient struct {
	raw []byte
	err error
}

func (c stubCorpusClient) Get(context.Context, string) ([]byte, error) {
	return c.raw, c.err
}

func corpusStubBody(records ...horizonTxRecord) []byte {
	var page horizonTxPage
	page.Embedded.Records = records
	raw, _ := json.Marshal(page)
	return raw
}

// TestCorpusSyncPlanKeepsRepresentativeRecords exercises selection with a
// canned page: good records are kept and named deterministically from their
// hash, everything else is dropped with the reason accounted for.
func TestCorpusSyncPlanKeepsRepresentativeRecords(t *testing.T) {
	t.Parallel()

	env, err := readFixture("tx_success.env")
	if err != nil {
		t.Fatalf("readFixture() error = %v", err)
	}
	res, err := readFixture("tx_success.res")
	if err != nil {
		t.Fatalf("readFixture() error = %v", err)
	}
	hash := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

	plan, kept, dropped, err := planCorpus([]horizonTxRecord{
		{Hash: hash, EnvelopeXdr: env, ResultXdr: res, Successful: true, OperationCount: 1},
		{Hash: hash, EnvelopeXdr: env},              // duplicate hash: already known
		{Hash: "", EnvelopeXdr: env},                // no hash: cannot anchor provenance
		{Hash: "9999", EnvelopeXdr: "!!!base64!!!"}, // undecodable
		// One good record, three rejected.
	}, loadCorpusManifest(t))
	if err != nil {
		t.Fatalf("planCorpus() error = %v", err)
	}
	if kept != 1 || dropped != 3 {
		t.Errorf("kept = %d, dropped = %d, want 1 and 3", kept, dropped)
	}
	if len(plan.Entries) != 1 {
		t.Fatalf("plan has %d entries, want 1", len(plan.Entries))
	}
	entry := plan.Entries[0]
	if entry.EnvelopeKey != corpusFixtureKey(hash) {
		t.Errorf("key = %q, want %q", entry.EnvelopeKey, corpusFixtureKey(hash))
	}
	rec := plan.Manifest.Fixtures[entry.EnvelopeKey]
	if rec.TxHash != hash {
		t.Errorf("record hash = %q, want %q", rec.TxHash, hash)
	}
	if rec.Successful == nil || !*rec.Successful {
		t.Errorf("record successful = %v, want true", rec.Successful)
	}
	if rec.EnvelopeFile != entry.EnvelopeKey+".env.txt" || rec.ResultFile != entry.EnvelopeKey+".res.txt" {
		t.Errorf("record files = %q/%q, want %q.env.txt/.res.txt", rec.EnvelopeFile, rec.ResultFile, entry.EnvelopeKey)
	}
	if rec.FeeBump {
		t.Errorf("record fee_bump = true, want false for %s", "tx_success.env")
	}
}

// TestCorpusSyncPlanDetectsFeeBumpArms uses a fee-bump fixture, the other
// envelope shape the manifest records explicitly.
func TestCorpusSyncPlanDetectsFeeBumpArms(t *testing.T) {
	t.Parallel()

	env, err := readFixture("tx_failed.env")
	if err != nil {
		t.Fatalf("readFixture() error = %v", err)
	}
	hash := "feedfacefeedfacefeedfacefeedfacefeedfacefeedfacefeedfacedeadbeef"

	plan, kept, _, err := planCorpus([]horizonTxRecord{
		{Hash: hash, EnvelopeXdr: env, Successful: false, OperationCount: 1},
	}, loadCorpusManifest(t))
	if err != nil {
		t.Fatalf("planCorpus() error = %v", err)
	}
	if kept != 1 {
		t.Fatalf("kept = %d, want 1", kept)
	}
	rec := plan.Manifest.Fixtures[plan.Entries[0].EnvelopeKey]
	if !rec.FeeBump {
		t.Error("record fee_bump = false, want true for the fee-bump fixture")
	}
	if rec.Successful == nil || *rec.Successful {
		t.Error("record successful = true, want false")
	}
}

// TestCorpusSyncPlanIsDeterministicAndRejectsSameEnvelopeBytes checks that
// re-planning the same input yields the same plan, and that two records
// with different hashes but identical envelope bytes collapse into one
// fixture: keeping both would double-count the same wire payload.
func TestCorpusSyncPlanIsDeterministicAndRejectsSameEnvelopeBytes(t *testing.T) {
	t.Parallel()

	env, err := readFixture("tx_success.env")
	if err != nil {
		t.Fatalf("readFixture() error = %v", err)
	}
	h1 := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	h2 := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	planA, keptA, droppedA, err := planCorpus([]horizonTxRecord{
		{Hash: h2, EnvelopeXdr: env}, {Hash: h1, EnvelopeXdr: env},
	}, loadCorpusManifest(t))
	if err != nil {
		t.Fatalf("planCorpus() error = %v", err)
	}
	planB, keptB, droppedB, err := planCorpus([]horizonTxRecord{
		{Hash: h2, EnvelopeXdr: env}, {Hash: h1, EnvelopeXdr: env},
	}, loadCorpusManifest(t))
	if err != nil {
		t.Fatalf("planCorpus() error = %v", err)
	}
	if keptA != 1 || keptB != 1 || droppedA != 1 || droppedB != 1 {
		t.Errorf("kept = %d/%d, dropped = %d/%d, want 1/1 and 1/1",
			keptA, keptB, droppedA, droppedB)
	}
	if len(planA.Entries) != 1 || planA.Entries[0].EnvelopeKey != planB.Entries[0].EnvelopeKey {
		t.Errorf("plan keys differ between runs: %q vs %q",
			planA.Entries[0].EnvelopeKey, planB.Entries[0].EnvelopeKey)
	}
	if planA.Entries[0].Record.Hash != h1 {
		t.Errorf("entries are not hash-sorted: kept %q, want %q", planA.Entries[0].Record.Hash, h1)
	}
}

// TestCorpusSyncFetchConfigErrors pins the builder's argument checks so a
// misconfigured run fails loudly instead of fetching nothing.
func TestCorpusSyncFetchConfigErrors(t *testing.T) {
	t.Parallel()

	if _, err := fetchCorpusPage(context.Background(), stubCorpusClient{raw: corpusStubBody()}, 0); !errors.Is(err, lens.ErrInvalidArgument) {
		t.Errorf("limit 0 error = %v, want lens.ErrInvalidArgument", err)
	}
	if _, err := fetchCorpusPage(context.Background(), nil, 10); !errors.Is(err, lens.ErrEmptyInput) {
		t.Errorf("nil client error = %v, want lens.ErrEmptyInput", err)
	}
	if _, err := fetchCorpusPage(context.Background(), stubCorpusClient{err: errors.New("boom")}, 10); err == nil {
		t.Error("client error surfaced as nil, want propagated")
	}
}

// TestCorpusSyncPageParses extends the same builder to a page of records so
// the real JSON shape is exercised (always offline: the stub is in-memory).
func TestCorpusSyncPageParses(t *testing.T) {
	t.Parallel()

	records, err := fetchCorpusPage(context.Background(),
		stubCorpusClient{raw: corpusStubBody(
			horizonTxRecord{Hash: "aa11", EnvelopeXdr: "AAAAAA=="},
			horizonTxRecord{Hash: "bb22", EnvelopeXdr: "AAAAAA=="},
		)}, 2)
	if err != nil {
		t.Fatalf("fetchCorpusPage() error = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("records = %d, want 2", len(records))
	}
}

// TestCorpusManifestAnchorsEveryFileToProvenance is the default-run sweep:
// every entry in the manifest of record points at files that exist and
// decode, every fixture on disk is anchored, captured hashes are well
// formed, and constructed fixtures declare a type.
func TestCorpusManifestAnchorsEveryFileToProvenance(t *testing.T) {
	manifest := loadCorpusManifest(t)

	files := map[string]bool{}
	entries, err := os.ReadDir(fixtureDir)
	if err != nil {
		t.Fatalf("reading testdata: %v", err)
	}
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() && strings.HasSuffix(name, ".txt") {
			files[name] = true
		}
	}

	referenced := map[string]bool{}
	txHash := regexp.MustCompile(`^[a-f0-9]{64}$`)

	for key, rec := range manifest.Fixtures {
		for _, file := range []string{rec.EnvelopeFile, rec.ResultFile} {
			if file == "" {
				continue
			}
			if referenced[file] {
				t.Errorf("fixture %q: file %q referenced twice", key, file)
			}
			referenced[file] = true
			payload, err := readFixture(strings.TrimSuffix(file, ".txt"))
			if err != nil {
				t.Errorf("fixture %q: referenced file %q is unreadable: %v", key, file, err)
				continue
			}
			if _, err := lens.Decode(payload); err != nil {
				t.Errorf("fixture %q: %q does not decode: %v", key, file, err)
			}
		}
		if rec.TxHash != "" && !txHash.MatchString(rec.TxHash) {
			t.Errorf("fixture %q: hash %q is not 64 lowercase hex", key, rec.TxHash)
		}
		if rec.TxHash != "" && rec.ResultFile == "" && rec.Successful != nil {
			t.Errorf("fixture %q: records success without a result file", key)
		}
	}
	synthetic := manifest.Synthetic
	if synthetic == nil {
		t.Fatal("manifest carries no synthetic_fixtures section")
	}
	for key, rec := range synthetic.Fixtures {
		file := key + ".txt"
		if referenced[file] {
			// Already inspected via the captured section.
		} else if _, err := readFixture(key); err != nil {
			t.Errorf("synthetic fixture %q: unreadable: %v", key, err)
		} else {
			referenced[file] = true
		}
		if rec.Type == "" {
			t.Errorf("synthetic fixture %q: no type recorded", key)
		}
	}
	for file := range files {
		if !referenced[file] {
			t.Errorf("fixture file %s is not referenced anywhere in the manifest", file)
		}
	}
}

// TestCorpusSyncGatedRun is the only place the synchronizer may touch the
// network, and only when the operator asks for it. Dry-run verifies the
// pipeline end to end and writes nothing; the write level additionally
// commits new fixtures and manifest entries.
func TestCorpusSyncGatedRun(t *testing.T) {
	mode := os.Getenv(corpusSyncEnv)
	if mode == "" {
		t.Skipf("skipping Testnet corpus sync: set %s=1 (verify) or %s=write (capture) to run", corpusSyncEnv, corpusSyncEnv)
	}
	if mode != "1" && mode != corpusSyncWriteMode {
		t.Skipf("skipping Testnet corpus sync: %s=%q is neither 1 nor write", corpusSyncEnv, mode)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	records, err := fetchCorpusPage(ctx, newHTTPCorpusClient(), 25)
	if err != nil {
		t.Fatalf("fetchCorpusPage() error = %v", err)
	}
	plan, kept, dropped, err := planCorpus(records, loadCorpusManifest(t))
	if err != nil {
		t.Fatalf("planCorpus() error = %v", err)
	}
	t.Logf("testnet page: %d records, kept %d, dropped %d", len(records), kept, dropped)
	if kept == 0 && dropped == 0 {
		t.Error("the page produced neither kept nor dropped records; the synchronizer saw nothing")
	}
	for _, entry := range plan.Entries {
		if len(entry.Envelope) == 0 {
			t.Errorf("kept entry %q has an empty envelope", entry.EnvelopeKey)
		}
		if v, err := lens.Decode(entry.Envelope); err != nil || v.Type != "TransactionEnvelope" {
			t.Errorf("kept entry %q does not decode as an envelope: %v", entry.EnvelopeKey, err)
		}
	}

	if mode != corpusSyncWriteMode {
		t.Log("dry run: nothing written to testdata")
		return
	}
	created, err := writeCorpusPlan(t, plan)
	if err != nil {
		t.Fatalf("writeCorpusPlan() error = %v", err)
	}
	t.Logf("wrote %d new fixture files: %v", len(created), created)
}
