package lens

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/stellar/go-stellar-sdk/xdr"
)

var (
	// ErrNoContractSpecSection is returned when a WASM binary contains no contractspecv0 section.
	ErrNoContractSpecSection = errors.New("contract WASM does not contain a contractspecv0 section")
	// ErrInvalidWasm is returned when the data does not start with the WASM magic header.
	ErrInvalidWasm = errors.New("invalid WebAssembly binary header")
	// ErrTypeNotFound is returned when a spec type name is not defined in the contract spec.
	ErrTypeNotFound = errors.New("spec type not found")
)

// ContractSpec holds parsed Soroban contract specifications.
type ContractSpec struct {
	Functions  map[string]xdr.ScSpecFunctionV0
	Structs    map[string]xdr.ScSpecUdtStructV0
	Unions     map[string]xdr.ScSpecUdtUnionV0
	Enums      map[string]xdr.ScSpecUdtEnumV0
	ErrorEnums map[string]xdr.ScSpecUdtErrorEnumV0
	Events     map[string]xdr.ScSpecEventV0
	Entries    []xdr.ScSpecEntry
}

// NewContractSpec allocates an empty ContractSpec.
func NewContractSpec() *ContractSpec {
	return &ContractSpec{
		Functions:  make(map[string]xdr.ScSpecFunctionV0),
		Structs:    make(map[string]xdr.ScSpecUdtStructV0),
		Unions:     make(map[string]xdr.ScSpecUdtUnionV0),
		Enums:      make(map[string]xdr.ScSpecUdtEnumV0),
		ErrorEnums: make(map[string]xdr.ScSpecUdtErrorEnumV0),
		Events:     make(map[string]xdr.ScSpecEventV0),
	}
}

// AddEntry adds an ScSpecEntry into the specification index.
func (cs *ContractSpec) AddEntry(e xdr.ScSpecEntry) {
	cs.Entries = append(cs.Entries, e)
	switch e.Kind {
	case xdr.ScSpecEntryKindScSpecEntryFunctionV0:
		if fn, ok := e.GetFunctionV0(); ok {
			cs.Functions[string(fn.Name)] = fn
		}
	case xdr.ScSpecEntryKindScSpecEntryUdtStructV0:
		if st, ok := e.GetUdtStructV0(); ok {
			cs.Structs[st.Name] = st
		}
	case xdr.ScSpecEntryKindScSpecEntryUdtUnionV0:
		if un, ok := e.GetUdtUnionV0(); ok {
			cs.Unions[un.Name] = un
		}
	case xdr.ScSpecEntryKindScSpecEntryUdtEnumV0:
		if en, ok := e.GetUdtEnumV0(); ok {
			cs.Enums[en.Name] = en
		}
	case xdr.ScSpecEntryKindScSpecEntryUdtErrorEnumV0:
		if errEn, ok := e.GetUdtErrorEnumV0(); ok {
			cs.ErrorEnums[errEn.Name] = errEn
		}
	case xdr.ScSpecEntryKindScSpecEntryEventV0:
		if ev, ok := e.GetEventV0(); ok {
			cs.Events[string(ev.Name)] = ev
		}
	}
}

// LoadContractSpecFile loads and parses a contract specification from a local file.
// The file can be a contract WASM (.wasm) or base64/raw XDR ScSpecEntry data.
func LoadContractSpecFile(path string) (*ContractSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading spec file %s: %w", path, err)
	}
	return ParseContractSpec(data)
}

// ParseContractSpec parses a contract specification from raw bytes or base64 text.
// If the payload is a WASM binary, it extracts the contractspecv0 section first.
func ParseContractSpec(data []byte) (*ContractSpec, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty contract spec data")
	}

	// If it's a WASM binary, extract custom section
	if isWasm(data) {
		specBytes, err := ExtractWasmContractSpec(data)
		if err != nil {
			return nil, err
		}
		data = specBytes
	} else if isBase64(data) {
		// Tolerates base64 string
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
		if err == nil && len(decoded) > 0 {
			if isWasm(decoded) {
				specBytes, err := ExtractWasmContractSpec(decoded)
				if err != nil {
					return nil, err
				}
				data = specBytes
			} else {
				data = decoded
			}
		}
	}

	cs := NewContractSpec()
	buf := bytes.NewReader(data)

	// Decode repeated ScSpecEntry until EOF
	for buf.Len() > 0 {
		var entry xdr.ScSpecEntry
		_, err := xdr.Unmarshal(buf, &entry)
		if err != nil {
			if len(cs.Entries) > 0 {
				// We already decoded at least one entry
				break
			}
			return nil, fmt.Errorf("xdr unmarshal ScSpecEntry: %w", err)
		}
		cs.AddEntry(entry)
	}

	if len(cs.Entries) == 0 {
		return nil, fmt.Errorf("no ScSpecEntry definitions found")
	}

	return cs, nil
}

// ExtractWasmContractSpec extracts the "contractspecv0" custom section from WebAssembly binary.
func ExtractWasmContractSpec(wasmData []byte) ([]byte, error) {
	if len(wasmData) < 8 || !isWasm(wasmData) {
		return nil, ErrInvalidWasm
	}

	r := bytes.NewReader(wasmData[8:]) // Skip 4-byte magic + 4-byte version

	for {
		sectionID, err := r.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("reading wasm section id: %w", err)
		}

		sectionLen, err := readVarUint32(r)
		if err != nil {
			return nil, fmt.Errorf("reading section length: %w", err)
		}

		if sectionID == 0 { // Custom section
			startPos, err := r.Seek(0, io.SeekCurrent)
			if err != nil {
				return nil, fmt.Errorf("seeking section: %w", err)
			}
			nameLen, err := readVarUint32(r)
			if err != nil {
				return nil, fmt.Errorf("reading custom section name len: %w", err)
			}
			nameBytes := make([]byte, nameLen)
			if _, err := io.ReadFull(r, nameBytes); err != nil {
				return nil, fmt.Errorf("reading custom section name: %w", err)
			}

			afterNamePos, err := r.Seek(0, io.SeekCurrent)
			if err != nil {
				return nil, fmt.Errorf("seeking section: %w", err)
			}
			nameHeaderSize := uint32(afterNamePos - startPos)

			if string(nameBytes) == "contractspecv0" {
				payloadLen := sectionLen - nameHeaderSize
				payload := make([]byte, payloadLen)
				if _, err := io.ReadFull(r, payload); err != nil {
					return nil, fmt.Errorf("reading contractspecv0 payload: %w", err)
				}
				return payload, nil
			}

			// Skip remaining custom section
			remaining := int64(sectionLen - nameHeaderSize)
			if _, err := r.Seek(remaining, io.SeekCurrent); err != nil {
				return nil, err
			}
		} else {
			// Skip other sections
			if _, err := r.Seek(int64(sectionLen), io.SeekCurrent); err != nil {
				return nil, err
			}
		}
	}

	return nil, ErrNoContractSpecSection
}

func isWasm(b []byte) bool {
	return len(b) >= 4 && b[0] == 0x00 && b[1] == 'a' && b[2] == 's' && b[3] == 'm'
}

func isBase64(b []byte) bool {
	s := strings.TrimSpace(string(b))
	if len(s) == 0 || len(s)%4 != 0 {
		return false
	}
	for _, r := range s {
		if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '+' && r != '/' && r != '=' {
			return false
		}
	}
	return true
}

func readVarUint32(r io.ByteReader) (uint32, error) {
	var result uint32
	var shift uint
	for {
		b, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		result |= uint32(b&0x7f) << shift
		if b&0x80 == 0 {
			break
		}
		shift += 7
		if shift > 35 {
			return 0, fmt.Errorf("varuint32 overflow")
		}
	}
	return result, nil
}

// DecodeScValNamed decodes an ScVal using a named UDT type from the spec.
func (cs *ContractSpec) DecodeScValNamed(val *xdr.ScVal, typeName string) (*Node, error) {
	if val == nil {
		return nil, fmt.Errorf("nil ScVal")
	}

	if st, ok := cs.Structs[typeName]; ok {
		return cs.decodeStruct(val, st), nil
	}
	if en, ok := cs.Enums[typeName]; ok {
		return cs.decodeEnum(val, en), nil
	}
	if un, ok := cs.Unions[typeName]; ok {
		return cs.decodeUnion(val, un), nil
	}

	return nil, fmt.Errorf("%w: %q", ErrTypeNotFound, typeName)
}

// DecodeInvocation decodes function arguments for a named contract function invocation.
func (cs *ContractSpec) DecodeInvocation(funcName string, args []xdr.ScVal) (*Node, error) {
	fn, ok := cs.Functions[funcName]
	if !ok {
		return nil, fmt.Errorf("function %q not found in contract spec", funcName)
	}

	node := &Node{
		Name:     funcName,
		Kind:     KindStruct,
		TypeName: "InvokeContractArgs",
	}

	for i, arg := range args {
		var argNode *Node
		argName := fmt.Sprintf("arg[%d]", i)
		if i < len(fn.Inputs) {
			input := fn.Inputs[i]
			argName = input.Name
			argNode = cs.DecodeTypeDef(&arg, input.Type)
		} else {
			argVal := arg
			argNode = cs.decodeGenericScVal(&argVal)
		}
		argNode.Name = argName
		node.Children = append(node.Children, argNode)
	}

	return node, nil
}

// ApplyToValue enriches any Soroban nodes in v using the contract specification.
func (cs *ContractSpec) ApplyToValue(v *Value) {
	if v == nil || v.Node == nil || cs == nil {
		return
	}
	cs.ApplyToNode(v.Node)
}

// ApplyToNode recursively searches for Soroban structures (such as InvokeContract or ScVal)
// and decorates them with real field names, function arguments, and enum variant names.
func (cs *ContractSpec) ApplyToNode(n *Node) {
	if n == nil {
		return
	}

	// Look for InvokeContract nodes
	if n.Name == "InvokeContract" {
		funcNode := n.Child("FunctionName")
		argsNode := n.Child("Args")
		if funcNode != nil && argsNode != nil {
			funcName := fmt.Sprintf("%v", funcNode.Value)
			if fn, ok := cs.Functions[funcName]; ok {
				funcNode.Note = "contract method"
				for i, argChild := range argsNode.Children {
					if i < len(fn.Inputs) {
						input := fn.Inputs[i]
						argChild.Name = input.Name
						typeName := specTypeName(input.Type)
						if argChild.Note == "" {
							argChild.Note = typeName
						} else {
							argChild.Note = typeName + " (" + argChild.Note + ")"
						}
						// If the input type is a UDT, enrich inner fields
						if input.Type.Type == xdr.ScSpecTypeScSpecTypeUdt && input.Type.Udt != nil {
							cs.enrichUdtChild(argChild, input.Type.Udt.Name)
						}
					}
				}
			}
		}
	}

	// Look for U32 nodes that might match an enum
	if n.TypeName == "ScVal" || strings.HasPrefix(n.Name, "Scv") || n.Kind == KindScalar {
		if valUint, ok := toUint32(n.Value); ok {
			for enumName, en := range cs.Enums {
				for _, c := range en.Cases {
					if uint32(c.Value) == valUint {
						n.Note = fmt.Sprintf("%s::%s", enumName, c.Name)
						break
					}
				}
			}
		}
	}

	for _, child := range n.Children {
		cs.ApplyToNode(child)
	}
}

func (cs *ContractSpec) enrichUdtChild(n *Node, udtName string) {
	if st, ok := cs.Structs[udtName]; ok {
		n.TypeName = st.Name
		for i, field := range st.Fields {
			if i < len(n.Children) {
				n.Children[i].Name = field.Name
			}
		}
	}
}

func (cs *ContractSpec) decodeStruct(val *xdr.ScVal, st xdr.ScSpecUdtStructV0) *Node {
	node := &Node{
		Kind:     KindStruct,
		TypeName: st.Name,
	}

	switch val.Type {
	case xdr.ScValTypeScvMap:
		if val.Map != nil && *val.Map != nil {
			m := **val.Map
			for _, entry := range m {
				keySym := ""
				if entry.Key.Type == xdr.ScValTypeScvSymbol && entry.Key.Sym != nil {
					keySym = string(*entry.Key.Sym)
				}
				// Find matching field
				var matchedField *xdr.ScSpecUdtStructFieldV0
				for _, f := range st.Fields {
					if f.Name == keySym {
						matchedField = &f
						break
					}
				}

				var childNode *Node
				if matchedField != nil {
					childNode = cs.DecodeTypeDef(&entry.Val, matchedField.Type)
					childNode.Name = matchedField.Name
				} else {
					valCopy := entry.Val
					childNode = cs.decodeGenericScVal(&valCopy)
					childNode.Name = keySym
				}
				node.Children = append(node.Children, childNode)
			}
		}

	case xdr.ScValTypeScvVec:
		if val.Vec != nil && *val.Vec != nil {
			v := **val.Vec
			for i, elem := range v {
				var childNode *Node
				if i < len(st.Fields) {
					field := st.Fields[i]
					childNode = cs.DecodeTypeDef(&elem, field.Type)
					childNode.Name = field.Name
				} else {
					elemCopy := elem
					childNode = cs.decodeGenericScVal(&elemCopy)
					childNode.Name = fmt.Sprintf("[%d]", i)
				}
				node.Children = append(node.Children, childNode)
			}
		}
	}

	return node
}

func (cs *ContractSpec) decodeEnum(val *xdr.ScVal, en xdr.ScSpecUdtEnumV0) *Node {
	u32Val := uint32(0)
	if val.Type == xdr.ScValTypeScvU32 && val.U32 != nil {
		u32Val = uint32(*val.U32)
	}

	caseName := fmt.Sprintf("%d", u32Val)
	for _, c := range en.Cases {
		if uint32(c.Value) == u32Val {
			caseName = c.Name
			break
		}
	}

	return &Node{
		Kind:     KindScalar,
		TypeName: en.Name,
		Value:    caseName,
		Note:     fmt.Sprintf("%s::%s (%d)", en.Name, caseName, u32Val),
	}
}

func (cs *ContractSpec) decodeUnion(val *xdr.ScVal, un xdr.ScSpecUdtUnionV0) *Node {
	node := &Node{
		Kind:     KindStruct,
		TypeName: un.Name,
	}

	// Union encoded as vec with [case_symbol, tuple_values...] or map
	if val.Type == xdr.ScValTypeScvVec && val.Vec != nil && *val.Vec != nil && len(**val.Vec) > 0 {
		vec := **val.Vec
		caseName := ""
		if vec[0].Type == xdr.ScValTypeScvSymbol && vec[0].Sym != nil {
			caseName = string(*vec[0].Sym)
		}

		node.Name = caseName
		for _, c := range un.Cases {
			if c.Kind == xdr.ScSpecUdtUnionCaseV0KindScSpecUdtUnionCaseVoidV0 && c.VoidCase != nil {
				if c.VoidCase.Name == caseName {
					node.Note = "void"
					return node
				}
			}
			if c.Kind == xdr.ScSpecUdtUnionCaseV0KindScSpecUdtUnionCaseTupleV0 && c.TupleCase != nil {
				if c.TupleCase.Name == caseName {
					for j, tType := range c.TupleCase.Type {
						if j+1 < len(vec) {
							childVal := vec[j+1]
							cNode := cs.DecodeTypeDef(&childVal, tType)
							cNode.Name = fmt.Sprintf("[%d]", j)
							node.Children = append(node.Children, cNode)
						}
					}
					return node
				}
			}
		}
	}

	valCopy := *val
	return cs.decodeGenericScVal(&valCopy)
}

// DecodeTypeDef decodes an ScVal according to an ScSpecTypeDef definition.
func (cs *ContractSpec) DecodeTypeDef(val *xdr.ScVal, td xdr.ScSpecTypeDef) *Node {
	if td.Type == xdr.ScSpecTypeScSpecTypeUdt && td.Udt != nil {
		if decoded, err := cs.DecodeScValNamed(val, td.Udt.Name); err == nil {
			return decoded
		}
	}

	valCopy := *val
	node := cs.decodeGenericScVal(&valCopy)
	node.Note = specTypeName(td)
	return node
}

func (cs *ContractSpec) decodeGenericScVal(val *xdr.ScVal) *Node {
	switch val.Type {
	case xdr.ScValTypeScvBool:
		if val.B != nil {
			return &Node{Kind: KindScalar, Value: *val.B}
		}
	case xdr.ScValTypeScvVoid:
		return &Node{Kind: KindScalar, Value: "void"}
	case xdr.ScValTypeScvU32:
		if val.U32 != nil {
			return &Node{Kind: KindScalar, Value: int64(*val.U32)}
		}
	case xdr.ScValTypeScvI32:
		if val.I32 != nil {
			return &Node{Kind: KindScalar, Value: int64(*val.I32)}
		}
	case xdr.ScValTypeScvU64:
		if val.U64 != nil {
			return &Node{Kind: KindScalar, Value: uint64(*val.U64)}
		}
	case xdr.ScValTypeScvI64:
		if val.I64 != nil {
			return &Node{Kind: KindScalar, Value: int64(*val.I64)}
		}
	case xdr.ScValTypeScvSymbol:
		if val.Sym != nil {
			return &Node{Kind: KindScalar, Value: string(*val.Sym)}
		}
	case xdr.ScValTypeScvString:
		if val.Str != nil {
			return &Node{Kind: KindScalar, Value: string(*val.Str)}
		}
	case xdr.ScValTypeScvAddress:
		if val.Address != nil {
			addrStr, err := val.Address.String()
			if err != nil {
				addrStr = fmt.Sprintf("%v", val.Address)
			}
			return &Node{Kind: KindScalar, Value: addrStr}
		}
	case xdr.ScValTypeScvVec:
		node := &Node{Kind: KindList}
		if val.Vec != nil && *val.Vec != nil {
			for _, elem := range **val.Vec {
				elemCopy := elem
				node.Children = append(node.Children, cs.decodeGenericScVal(&elemCopy))
			}
		}
		return node
	case xdr.ScValTypeScvMap:
		node := &Node{Kind: KindStruct}
		if val.Map != nil && *val.Map != nil {
			for _, entry := range **val.Map {
				keyStr := fmt.Sprintf("%v", entry.Key)
				if entry.Key.Type == xdr.ScValTypeScvSymbol && entry.Key.Sym != nil {
					keyStr = string(*entry.Key.Sym)
				}
				valCopy := entry.Val
				c := cs.decodeGenericScVal(&valCopy)
				c.Name = keyStr
				node.Children = append(node.Children, c)
			}
		}
		return node
	}
	return &Node{Kind: KindScalar, Value: val.String()}
}

func specTypeName(td xdr.ScSpecTypeDef) string {
	switch td.Type {
	case xdr.ScSpecTypeScSpecTypeUdt:
		if td.Udt != nil {
			return td.Udt.Name
		}
		return "UDT"
	case xdr.ScSpecTypeScSpecTypeU32:
		return "u32"
	case xdr.ScSpecTypeScSpecTypeI32:
		return "i32"
	case xdr.ScSpecTypeScSpecTypeU64:
		return "u64"
	case xdr.ScSpecTypeScSpecTypeI64:
		return "i64"
	case xdr.ScSpecTypeScSpecTypeBool:
		return "bool"
	case xdr.ScSpecTypeScSpecTypeSymbol:
		return "symbol"
	case xdr.ScSpecTypeScSpecTypeString:
		return "string"
	case xdr.ScSpecTypeScSpecTypeAddress:
		return "address"
	case xdr.ScSpecTypeScSpecTypeVec:
		return "vec"
	case xdr.ScSpecTypeScSpecTypeMap:
		return "map"
	default:
		return td.Type.String()
	}
}

func toUint32(v any) (uint32, bool) {
	switch val := v.(type) {
	case uint32:
		return val, true
	case int64:
		return uint32(val), true
	case uint64:
		return uint32(val), true
	case int:
		return uint32(val), true
	default:
		return 0, false
	}
}
