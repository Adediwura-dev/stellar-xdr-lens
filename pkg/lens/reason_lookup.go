package lens

import (
	"sort"
	"strings"
	"sync"
)

var (
	lookupMu     sync.RWMutex
	lookupByCode map[string]Reason
)

func init() {
	rebuildLookupTable()
}

// rebuildLookupTable rebuilds the lookup table from the current reasons map.
func rebuildLookupTable() {
	lookupMu.Lock()
	defer lookupMu.Unlock()

	lookupByCode = make(map[string]Reason, len(reasons))
	for constant, text := range reasons {
		code := shortCode(constant)
		lookupByCode[code] = Reason{
			Code:     code,
			Constant: constant,
			Summary:  text.summary,
			Hint:     text.hint,
			Success:  text.success,
		}
	}
}

// registerReason adds or updates a reason in the reasons map and updates the lookup table.
func registerReason(constant string, text reasonText) {
	reasons[constant] = text
	code := shortCode(constant)

	lookupMu.Lock()
	defer lookupMu.Unlock()
	if lookupByCode == nil {
		lookupByCode = make(map[string]Reason)
	}
	lookupByCode[code] = Reason{
		Code:     code,
		Constant: constant,
		Summary:  text.summary,
		Hint:     text.hint,
		Success:  text.success,
	}
}

// ReasonByCode returns the plain-English Reason for a short result code, such
// as "tx_bad_seq" or "payment_underfunded". It returns false if the code is
// not known to this version of lens.
//
// Lookup is performed in O(1) time using a precomputed lookup table, designed
// for high-throughput downstream tools like focus.
func ReasonByCode(short string) (Reason, bool) {
	lookupMu.RLock()
	defer lookupMu.RUnlock()

	if r, ok := lookupByCode[short]; ok {
		return r, true
	}

	// Normalize input (trim whitespace and lowercase) to support robust queries.
	normalized := strings.ToLower(strings.TrimSpace(short))
	if r, ok := lookupByCode[normalized]; ok {
		return r, true
	}

	return Reason{}, false
}

// AllReasons returns a sorted slice of all registered Reason entries.
// Downstream tools can use this to enumerate all known diagnostic explanations.
func AllReasons() []Reason {
	lookupMu.RLock()
	defer lookupMu.RUnlock()

	out := make([]Reason, 0, len(lookupByCode))
	for _, r := range lookupByCode {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Code < out[j].Code
	})
	return out
}

// ReasonLookupTable returns a shallow copy of the lookup table mapping short
// codes to their corresponding Reason values.
func ReasonLookupTable() map[string]Reason {
	lookupMu.RLock()
	defer lookupMu.RUnlock()

	table := make(map[string]Reason, len(lookupByCode))
	for k, v := range lookupByCode {
		table[k] = v
	}
	return table
}
