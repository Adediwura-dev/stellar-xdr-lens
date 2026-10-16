package lens_test

import (
	"strings"
	"testing"

	"github.com/stellar/go-stellar-sdk/xdr"

	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens"
)

const (
	node1Addr       = "GBIFCUSTKRKVMV2YLFNFWXC5LZPWAYLCMNSGKZTHNBUWU23MNVXG6NJZ"
	node2Addr       = "GBYHC4TTOR2XM53YPF5HW7D5PZ7YBAMCQOCILBUHRCEYVC4MRWHI66TB"
	node3Addr       = "GCIJDEUTSSKZNF4YTGNJXHE5T2P2BINCUOSKLJVHVCU2VK5MVWXK6VGZ"
	commitHashNote  = "22232425262728292a2b2c2d2e2f303132333435363738393a3b3c3d3e3f4041"
	commitValueNote = "1112131415161718191a1b1c1d1e1f202122232425262728292a2b2c2d2e2f30"
)

func TestDecodeScpEnvelope(t *testing.T) {
	t.Parallel()

	v, err := lens.DecodeAs(loadFixture(t, "scp_envelope"), "ScpEnvelope")
	if err != nil {
		t.Fatalf("DecodeAs() error = %v", err)
	}
	if v.Type != "ScpEnvelope" {
		t.Errorf("type = %q, want ScpEnvelope", v.Type)
	}
	raw, ok := v.Raw.(*xdr.ScpEnvelope)
	if !ok {
		t.Fatalf("Raw is %T, want *xdr.ScpEnvelope", v.Raw)
	}
	if raw.Statement.SlotIndex != 55278000 {
		t.Errorf("SlotIndex = %d, want 55278000", raw.Statement.SlotIndex)
	}

	// The validator identity is a raw ed25519 key on the wire; the strkey
	// address is what makes it recognisable.
	nodeID := v.Node.Find("Statement.NodeId")
	if nodeID == nil {
		t.Fatal("could not find Statement.NodeId")
	}
	if got, ok := nodeID.Value.(string); !ok || got != node1Addr {
		t.Errorf("Statement.NodeId = %v, want %q", nodeID.Value, node1Addr)
	}

	slot := v.Node.Find("Statement.SlotIndex")
	if slot == nil || slot.Note != "ledger slot 55278000" {
		t.Errorf("Statement.SlotIndex = %v, want a ledger slot note", slot)
	}

	pledge := v.Node.Find("Statement.Pledges.Type")
	if pledge == nil || pledge.Value != "externalize" {
		t.Errorf("Statement.Pledges.Type = %v, want %q", pledge, "externalize")
	}
	counter := v.Node.Find("Statement.Pledges.Externalize.Commit.Counter")
	if counter == nil || counter.Value != uint64(3) {
		t.Errorf("Commit.Counter = %v, want 3", counter)
	}
	commit := v.Node.Find("Statement.Pledges.Externalize.Commit.Value")
	if commit == nil || commit.Value != commitValueNote {
		t.Errorf("Commit.Value = %v, want %q", commit, commitValueNote)
	}
	quorumHash := v.Node.Find("Statement.Pledges.Externalize.CommitQuorumSetHash")
	if quorumHash == nil || !strings.Contains(quorumHash.Note, commitHashNote) {
		t.Errorf("CommitQuorumSetHash = %v, want it to contain %q", quorumHash, commitHashNote)
	}
}

func TestDecodeScpStatement(t *testing.T) {
	t.Parallel()

	v, err := lens.DecodeAs(loadFixture(t, "scp_statement"), "ScpStatement")
	if err != nil {
		t.Fatalf("DecodeAs() error = %v", err)
	}
	if v.Type != "ScpStatement" {
		t.Errorf("type = %q, want ScpStatement", v.Type)
	}
	if _, ok := v.Raw.(*xdr.ScpStatement); !ok {
		t.Fatalf("Raw is %T, want *xdr.ScpStatement", v.Raw)
	}
	if pledge := v.Node.Find("Pledges.Type"); pledge == nil || pledge.Value != "externalize" {
		t.Errorf("Pledges.Type = %v, want %q", pledge, "externalize")
	}
}

func TestDecodeScpQuorumSet(t *testing.T) {
	t.Parallel()

	v, err := lens.DecodeAs(loadFixture(t, "scp_quorum_set"), "ScpQuorumSet")
	if err != nil {
		t.Fatalf("DecodeAs() error = %v", err)
	}
	if v.Type != "ScpQuorumSet" {
		t.Errorf("type = %q, want ScpQuorumSet", v.Type)
	}
	raw, ok := v.Raw.(*xdr.ScpQuorumSet)
	if !ok {
		t.Fatalf("Raw is %T, want *xdr.ScpQuorumSet", v.Raw)
	}
	if raw.Threshold != 2 {
		t.Errorf("Threshold = %d, want 2", raw.Threshold)
	}
	if len(raw.Validators) != 3 {
		t.Errorf("len(Validators) = %d, want 3", len(raw.Validators))
	}

	threshold := v.Node.Find("Threshold")
	if threshold == nil || threshold.Value != uint64(2) {
		t.Errorf("Threshold = %v, want 2", threshold)
	}
	validators := v.Node.Find("Validators")
	if validators == nil {
		t.Fatal("could not find Validators")
	}
	wantAddrs := []string{node1Addr, node2Addr, node3Addr}
	if len(validators.Children) != len(wantAddrs) {
		t.Fatalf("Validators has %d children, want %d", len(validators.Children), len(wantAddrs))
	}
	for i, want := range wantAddrs {
		got, ok := validators.Children[i].Value.(string)
		if !ok || got != want {
			t.Errorf("Validators[%d] = %v, want %q", i, validators.Children[i].Value, want)
		}
	}

	// Quorum sets nest, so the inner set must render the same way.
	inner := v.Node.Find("InnerSets[0]")
	if inner == nil {
		t.Fatal("could not find InnerSets[0]")
	}
	if got := inner.Child("Threshold"); got == nil || got.Value != uint64(1) {
		t.Errorf("InnerSets[0].Threshold = %v, want 1", got)
	}
	innerValidator := inner.Find("Validators[0]")
	if innerValidator == nil || innerValidator.Value != node1Addr {
		t.Errorf("InnerSets[0].Validators[0] = %v, want %q", innerValidator, node1Addr)
	}
}

func TestDecodeScpRejectsTruncatedInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		fixture  string
		typeName string
	}{
		{"scp_envelope", "ScpEnvelope"},
		{"scp_statement", "ScpStatement"},
		{"scp_quorum_set", "ScpQuorumSet"},
	}

	for _, tc := range tests {
		t.Run(tc.fixture, func(t *testing.T) {
			t.Parallel()

			if _, err := lens.DecodeAs(loadFixtureRaw(tc.fixture)[:16], tc.typeName); err == nil {
				t.Errorf("DecodeAs(%s truncated) error = nil, want an error", tc.fixture)
			}
		})
	}
}

func TestScpDetectionIsUnambiguous(t *testing.T) {
	t.Parallel()

	tests := []struct {
		fixture  string
		wantType string
	}{
		{"scp_envelope", "ScpEnvelope"},
		{"scp_statement", "ScpStatement"},
		{"scp_quorum_set", "ScpQuorumSet"},
	}

	for _, tc := range tests {
		t.Run(tc.fixture, func(t *testing.T) {
			t.Parallel()

			cands, err := lens.Detect(loadFixture(t, tc.fixture))
			if err != nil {
				t.Fatalf("Detect() error = %v", err)
			}
			if len(cands) != 1 || cands[0].Type != tc.wantType {
				names := make([]string, 0, len(cands))
				for _, c := range cands {
					names = append(names, c.Type)
				}
				t.Errorf("Detect() = [%s], want exactly [%s]", strings.Join(names, ", "), tc.wantType)
			}
		})
	}
}
