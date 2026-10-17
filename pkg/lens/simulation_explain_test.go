package lens_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens"
)

func TestDiagnoseSimulationSuccess(t *testing.T) {
	t.Parallel()
	d, err := lens.DiagnoseSimulationError("")
	if err != nil {
		t.Fatalf("DiagnoseSimulationError() error = %v", err)
	}
	if !d.Success || d.Kind != lens.SimulationKindSuccess {
		t.Errorf("got success=%v kind=%q, want true/success", d.Success, d.Kind)
	}
	resp, err := lens.DiagnoseSimulationResponse(&lens.SimulationResponse{})
	if err != nil {
		t.Fatalf("DiagnoseSimulationResponse(empty) error = %v", err)
	}
	if !resp.Success {
		t.Errorf("empty response success = false, want true")
	}
	if _, err := lens.DiagnoseSimulationResponse(nil); !errors.Is(err, lens.ErrInvalidArgument) {
		t.Errorf("nil response error = %v, want ErrInvalidArgument", err)
	}
}

func TestDiagnoseSimulationKinds(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		errMsg   string
		want     lens.SimulationKind
		wantHint string
	}{
		{"missing footprint", "Error: missing footprint key: contract data key not in footprint", lens.SimulationKindMissingFootprint, "footprint"},
		{"footprint key variant", "footprint key not found in readWrite footprint", lens.SimulationKindMissingFootprint, "footprint"},
		{"cpu exceeded", "host error: exceeded CPU budget: instructions used exceed limit", lens.SimulationKindCPUExceeded, "CPU"},
		{"resource limit", "invoke host function resource limit exceeded", lens.SimulationKindCPUExceeded, "CPU"},
		{"memory exceeded", "host error: memory budget exceeded: allocation too large", lens.SimulationKindMemoryExceeded, "memory"},
		{"insufficient fee", "insufficient refundable fee to cover rent: min resource fee 50000", lens.SimulationKindInsufficientFee, "refundable"},
		{"archived entry", "entry is archived: restore footprint before access", lens.SimulationKindArchivedEntry, "restore"},
		{"frozen key", "frozen key accessed: entry expired and needs restore", lens.SimulationKindArchivedEntry, "restore"},
		{"auth missing", "missing authorization: contract requires auth entry from GABC", lens.SimulationKindAuthMissing, "auth"},
		{"contract trapped", "contract trapped: panicked with error 9", lens.SimulationKindContractTrapped, "contract"},
		{"host error", "scError: host object error", lens.SimulationKindContractTrapped, "contract"},
		{"invalid input", "malformed simulation request: invalid footprint", lens.SimulationKindInvalidInput, "schema"},
		{"unknown", "something completely unexpected happened", lens.SimulationKindUnknown, "verbose"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d, err := lens.DiagnoseSimulationError(tc.errMsg)
			if err != nil {
				t.Fatalf("DiagnoseSimulationError() error = %v", err)
			}
			if d.Kind != tc.want {
				t.Errorf("Kind = %q, want %q", d.Kind, tc.want)
			}
			if d.Success {
				t.Error("Success = true, want false")
			}
			if d.Summary == "" {
				t.Error("Summary is empty")
			}
			if d.Hint == "" {
				t.Error("Hint is empty")
			}
			if !strings.Contains(strings.ToLower(d.Hint), strings.ToLower(tc.wantHint)) {
				t.Errorf("Hint = %q, want mention of %q", d.Hint, tc.wantHint)
			}
			if d.Detail == "" {
				t.Error("Detail is empty, want raw error text")
			}
		})
	}
}

func TestDiagnoseSimulationCaseInsensitive(t *testing.T) {
	t.Parallel()
	d, err := lens.DiagnoseSimulationError("MISSING FOOTPRINT KEY")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if d.Kind != lens.SimulationKindMissingFootprint {
		t.Errorf("Kind = %q, want missing_footprint", d.Kind)
	}
}

func TestDiagnoseSimulationResponseStructured(t *testing.T) {
	t.Parallel()
	// Missing keys beat prose.
	resp := &lens.SimulationResponse{Error: "unrelated boom", MissingKeys: []string{"keyA", "keyB"}}
	d, err := lens.DiagnoseSimulationResponse(resp)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if d.Kind != lens.SimulationKindMissingFootprint {
		t.Errorf("Kind = %q, want missing_footprint", d.Kind)
	}
	if len(d.MissingKeys) != 2 {
		t.Errorf("MissingKeys = %v, want 2 keys", d.MissingKeys)
	}
	// CPU counters without an error string still diagnose exhaustion.
	cpuResp := &lens.SimulationResponse{CPUInstructionsUsed: 200, CPUInstructionsLimit: 100}
	dCPU, err := lens.DiagnoseSimulationResponse(cpuResp)
	if err != nil {
		t.Fatalf("cpu error = %v", err)
	}
	if dCPU.Kind != lens.SimulationKindCPUExceeded || dCPU.Success {
		t.Errorf("cpu = %+v, want cpu_exceeded failure", dCPU)
	}
	memResp := &lens.SimulationResponse{MemoryBytesUsed: 9000, MemoryBytesLimit: 1000}
	dMem, err := lens.DiagnoseSimulationResponse(memResp)
	if err != nil {
		t.Fatalf("mem error = %v", err)
	}
	if dMem.Kind != lens.SimulationKindMemoryExceeded {
		t.Errorf("mem kind = %q, want memory_exceeded", dMem.Kind)
	}
	// Resource detail is attached when counters accompany the error text.
	fullCPU := &lens.SimulationResponse{Error: "cpu budget exceeded", CPUInstructionsUsed: 500, CPUInstructionsLimit: 100}
	dFull, err := lens.DiagnoseSimulationResponse(fullCPU)
	if err != nil {
		t.Fatalf("full cpu error = %v", err)
	}
	if !strings.Contains(dFull.Detail, "500") {
		t.Errorf("Detail = %q, want counter values", dFull.Detail)
	}
	// Fee hint carries the recommended minimum.
	feeResp := &lens.SimulationResponse{Error: "insufficient refundable fee", MinResourceFee: 75000}
	dFee, err := lens.DiagnoseSimulationResponse(feeResp)
	if err != nil {
		t.Fatalf("fee error = %v", err)
	}
	if !strings.Contains(dFee.Hint, "75000") {
		t.Errorf("Hint = %q, want recommended fee", dFee.Hint)
	}
}

func TestDiagnoseMissingFootprint(t *testing.T) {
	t.Parallel()
	d, err := lens.DiagnoseMissingFootprint([]string{"contractData:ABC"})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if d.Kind != lens.SimulationKindMissingFootprint || d.Success {
		t.Errorf("got %+v, want missing_footprint failure", d)
	}
	if len(d.MissingKeys) != 1 {
		t.Errorf("MissingKeys = %v, want 1", d.MissingKeys)
	}
	multi, err := lens.DiagnoseMissingFootprint([]string{"k1", "k2", "k3"})
	if err != nil {
		t.Fatalf("multi error = %v", err)
	}
	if !strings.Contains(multi.Summary, "3 ledger keys") {
		t.Errorf("Summary = %q, want key count", multi.Summary)
	}
	if _, err := lens.DiagnoseMissingFootprint(nil); !errors.Is(err, lens.ErrInvalidArgument) {
		t.Errorf("empty keys error = %v, want ErrInvalidArgument", err)
	}
}

func TestDiagnoseResourceExhaustion(t *testing.T) {
	t.Parallel()
	ok, err := lens.DiagnoseResourceExhaustion(50, 100, 50, 100)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if !ok.Success {
		t.Error("fitting budgets should succeed")
	}
	cpu, err := lens.DiagnoseResourceExhaustion(200, 100, 50, 100)
	if err != nil {
		t.Fatalf("cpu error = %v", err)
	}
	if cpu.Kind != lens.SimulationKindCPUExceeded || cpu.Success {
		t.Errorf("cpu = %+v, want cpu_exceeded failure", cpu)
	}
	mem, err := lens.DiagnoseResourceExhaustion(50, 100, 200, 100)
	if err != nil {
		t.Fatalf("mem error = %v", err)
	}
	if mem.Kind != lens.SimulationKindMemoryExceeded {
		t.Errorf("mem kind = %q, want memory_exceeded", mem.Kind)
	}
	both, err := lens.DiagnoseResourceExhaustion(200, 100, 200, 100)
	if err != nil {
		t.Fatalf("both error = %v", err)
	}
	if both.Kind != lens.SimulationKindCPUExceeded {
		t.Errorf("both kind = %q, want cpu (binding constraint)", both.Kind)
	}
	if !strings.Contains(both.Detail, "CPU") || !strings.Contains(both.Detail, "memory") {
		t.Errorf("Detail = %q, want both resources", both.Detail)
	}
}
