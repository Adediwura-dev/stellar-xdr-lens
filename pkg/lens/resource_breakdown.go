package lens

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/stellar/go-stellar-sdk/xdr"
)

// ResourceLimits describes the per-transaction Soroban resource limits a
// breakdown is compared against. Values are network settings that validators
// can change; DefaultResourceLimits returns the Mainnet Protocol 27 snapshot
// documented by the Stellar developer docs.
type ResourceLimits struct {
	Instructions     int64 `json:"instructions"`
	MemoryBytes      int64 `json:"memoryBytes"`
	DiskReadBytes    int64 `json:"diskReadBytes"`
	WriteBytes       int64 `json:"writeBytes"`
	DiskReadEntries  int64 `json:"diskReadEntries"`
	WriteEntries     int64 `json:"writeEntries"`
	FootprintEntries int64 `json:"footprintEntries"`
}

// DefaultResourceLimits returns the current Mainnet per-transaction limits.
// They are a deterministic, offline reference snapshot, not a live network
// query: callers that know the target network's settings can supply their own
// ResourceLimits instead.
func DefaultResourceLimits() ResourceLimits {
	return ResourceLimits{
		Instructions:     400_000_000,
		MemoryBytes:      40 * 1024 * 1024,
		DiskReadBytes:    200_000,
		WriteBytes:       132_096,
		DiskReadEntries:  200,
		WriteEntries:     200,
		FootprintEntries: 400,
	}
}

// ResourceUsageRow is one line of a Soroban resource breakdown. Used is in
// Unit; Limit is zero when the resource has no fixed protocol cap. Declared
// is false for values the transaction XDR does not carry (memory usage), so a
// missing measurement is never rendered as zero usage.
type ResourceUsageRow struct {
	Resource string  `json:"resource"`
	Unit     string  `json:"unit"`
	Used     int64   `json:"used"`
	Limit    int64   `json:"limit,omitempty"`
	Declared bool    `json:"declared"`
	HasLimit bool    `json:"hasLimit"`
	Exceeded bool    `json:"exceeded"`
	Percent  float64 `json:"percent,omitempty"`
	Note     string  `json:"note,omitempty"`
}

// ResourceBreakdown is a Soroban transaction's declared resource allocation
// compared with per-transaction protocol limits. It is derived only from the
// supplied XDR (and, optionally, transaction metadata for charged fees).
type ResourceBreakdown struct {
	Source   string             `json:"source"`
	Rows     []ResourceUsageRow `json:"rows"`
	Exceeded []string           `json:"exceeded,omitempty"`
	Limits   ResourceLimits     `json:"limits"`
}

// HasExceeded reports whether any declared resource is above its limit.
func (b *ResourceBreakdown) HasExceeded() bool { return b != nil && len(b.Exceeded) > 0 }

// ExplainResources extracts the Soroban resource declaration from a
// TransactionEnvelope or SorobanTransactionData value and compares it with
// limits. Transaction results alone carry no resource allocation and return
// ErrUnsupportedType.
func ExplainResources(value *Value, limits ResourceLimits) (*ResourceBreakdown, error) {
	return ExplainResourcesWithMeta(value, nil, limits)
}

// ExplainResourcesWithMeta is ExplainResources plus optional TransactionMeta.
// The metadata supplies the rent, refundable and non-refundable fees actually
// charged; without it the breakdown reports the envelope's declared resource
// fee only. A zero-value limits is replaced with DefaultResourceLimits. The
// function reads only the supplied values and makes no network calls.
func ExplainResourcesWithMeta(value, meta *Value, limits ResourceLimits) (*ResourceBreakdown, error) {
	if value == nil {
		return nil, fmt.Errorf("%w: nil value", ErrInvalidArgument)
	}
	if limits == (ResourceLimits{}) {
		limits = DefaultResourceLimits()
	}

	var (
		data   *xdr.SorobanTransactionData
		source string
	)
	switch raw := value.Raw.(type) {
	case *xdr.TransactionEnvelope:
		data = sorobanDataFromEnvelope(raw)
		source = "TransactionEnvelope"
	case *xdr.SorobanTransactionData:
		data = raw
		source = "SorobanTransactionData"
	default:
		return nil, fmt.Errorf("%w: cannot explain resources for %s; expected TransactionEnvelope or SorobanTransactionData",
			ErrUnsupportedType, value.Type)
	}
	if data == nil {
		return nil, fmt.Errorf("%w: the transaction declares no Soroban resource data", ErrUnsupportedType)
	}

	readOnly := int64(len(data.Resources.Footprint.ReadOnly))
	readWrite := int64(len(data.Resources.Footprint.ReadWrite))
	breakdown := &ResourceBreakdown{
		Source: source,
		Limits: limits,
		Rows: []ResourceUsageRow{
			resourceRow("Instructions (CPU)", "instructions", int64(data.Resources.Instructions), limits.Instructions, true, ""),
			{
				Resource: "Memory",
				Unit:     "bytes",
				Limit:    limits.MemoryBytes,
				Declared: false,
				HasLimit: true,
				Note:     "not declared in transaction XDR; simulation reports actual memory use",
			},
			resourceRow("Disk read bytes", "bytes", int64(data.Resources.DiskReadBytes), limits.DiskReadBytes, true, ""),
			resourceRow("Write bytes", "bytes", int64(data.Resources.WriteBytes), limits.WriteBytes, true, ""),
			resourceRow("Disk read entries", "entries", readOnly, limits.DiskReadEntries, true, ""),
			resourceRow("Write entries", "entries", readWrite, limits.WriteEntries, true, ""),
			resourceRow("Footprint entries", "entries", readOnly+readWrite, limits.FootprintEntries, true, ""),
			{
				Resource: "Resource fee (declared)",
				Unit:     "stroops",
				Used:     int64(data.ResourceFee),
				Declared: true,
				Note:     "envelope cap for rent, events and compute; charged amounts are in transaction meta",
			},
		},
	}
	if meta != nil {
		appendChargedFees(breakdown, meta)
	}
	for _, row := range breakdown.Rows {
		if row.Exceeded {
			breakdown.Exceeded = append(breakdown.Exceeded, row.Resource)
		}
	}
	return breakdown, nil
}

func resourceRow(resource, unit string, used, limit int64, declared bool, note string) ResourceUsageRow {
	row := ResourceUsageRow{
		Resource: resource,
		Unit:     unit,
		Used:     used,
		Limit:    limit,
		Declared: declared,
		HasLimit: limit > 0,
		Note:     note,
	}
	if row.HasLimit {
		row.Exceeded = used > limit
		if limit > 0 {
			row.Percent = float64(used) / float64(limit) * 100
		}
	}
	return row
}

func appendChargedFees(breakdown *ResourceBreakdown, meta *Value) {
	ext := sorobanMetaExtFromValue(meta)
	if ext == nil || ext.V1 == nil {
		return
	}
	v1 := ext.V1
	breakdown.Rows = append(breakdown.Rows,
		ResourceUsageRow{
			Resource: "Non-refundable fee (charged)",
			Unit:     "stroops",
			Used:     int64(v1.TotalNonRefundableResourceFeeCharged),
			Declared: true,
			Note:     "compute and I/O fee charged regardless of success",
		},
		ResourceUsageRow{
			Resource: "Refundable fee (charged)",
			Unit:     "stroops",
			Used:     int64(v1.TotalRefundableResourceFeeCharged),
			Declared: true,
			Note:     "rent plus events and return value",
		},
		ResourceUsageRow{
			Resource: "Rent fee (charged)",
			Unit:     "stroops",
			Used:     int64(v1.RentFeeCharged),
			Declared: true,
			Note:     "subset of the refundable fee",
		},
	)
}

func sorobanMetaExtFromValue(value *Value) *xdr.SorobanTransactionMetaExt {
	meta, ok := value.Raw.(*xdr.TransactionMeta)
	if !ok || meta == nil {
		return nil
	}
	switch meta.V {
	case 3:
		if meta.V3 != nil && meta.V3.SorobanMeta != nil {
			return &meta.V3.SorobanMeta.Ext
		}
	case 4:
		if meta.V4 != nil && meta.V4.SorobanMeta != nil {
			return &meta.V4.SorobanMeta.Ext
		}
	}
	return nil
}

// String renders the breakdown as a stable, human-readable table. Columns are
// aligned with tabs, and an exceeded resource is called out explicitly rather
// than relying on colour.
func (b *ResourceBreakdown) String() string {
	if b == nil {
		return ""
	}
	var out strings.Builder
	fmt.Fprintf(&out, "Soroban resource breakdown (per-transaction limits)\n")
	tw := tabwriter.NewWriter(&out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "RESOURCE\tUSED\tLIMIT\tUSAGE\tSTATUS")
	for _, row := range b.Rows {
		used := formatResourceNumber(row.Used)
		if !row.Declared {
			used = "n/a"
		}
		limit := "no fixed limit"
		usage := "-"
		if row.HasLimit {
			limit = formatResourceNumber(row.Limit)
		}
		if row.Declared && row.HasLimit {
			usage = fmt.Sprintf("%.1f%%", row.Percent)
		}
		status := "ok"
		switch {
		case row.Exceeded:
			status = "EXCEEDED"
		case !row.Declared:
			status = "not declared"
		case !row.HasLimit:
			status = "informational"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", row.Resource, used, limit, usage, status)
	}
	if err := tw.Flush(); err != nil {
		return out.String()
	}
	for _, row := range b.Rows {
		if row.Note != "" {
			fmt.Fprintf(&out, "  %s: %s\n", row.Resource, row.Note)
		}
	}
	if len(b.Exceeded) > 0 {
		fmt.Fprintf(&out, "EXCEEDED: %s\n", strings.Join(b.Exceeded, ", "))
	} else {
		fmt.Fprintln(&out, "All declared resources are within the reference limits.")
	}
	return out.String()
}

// WriteTo writes the rendered breakdown and reports any write error.
func (b *ResourceBreakdown) WriteTo(w io.Writer) (int64, error) {
	n, err := io.WriteString(w, b.String())
	return int64(n), err
}

func formatResourceNumber(n int64) string {
	digits := strconv.FormatInt(n, 10)
	sign := ""
	if strings.HasPrefix(digits, "-") {
		sign = "-"
		digits = digits[1:]
	}
	if len(digits) <= 3 {
		return sign + digits
	}
	var out strings.Builder
	out.WriteString(sign)
	first := len(digits) % 3
	if first == 0 {
		first = 3
	}
	out.WriteString(digits[:first])
	for i := first; i < len(digits); i += 3 {
		out.WriteByte(',')
		out.WriteString(digits[i : i+3])
	}
	return out.String()
}
