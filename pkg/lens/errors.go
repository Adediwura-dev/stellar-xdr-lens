package lens

// ErrorKind identifies a class of errors returned by the lens library.
// Callers can inspect it with errors.As, or use the Err-prefixed sentinels
// with errors.Is when they need to match one specific failure.
type ErrorKind string

// Error kinds are stable identifiers and their text is also the default
// human-readable message for each sentinel.
const (
	ErrorKindEmptyInput               ErrorKind = "empty input"
	ErrorKindInvalidBase64            ErrorKind = "input is not valid base64"
	ErrorKindInvalidXDR               ErrorKind = "invalid XDR"
	ErrorKindNoMatch                  ErrorKind = "input did not decode as any known XDR type"
	ErrorKindUnknownType              ErrorKind = "unknown XDR type"
	ErrorKindInvalidArgument          ErrorKind = "invalid argument"
	ErrorKindUnsupportedType          ErrorKind = "unsupported XDR type"
	ErrorKindNotAsset                 ErrorKind = "value is not a classic asset"
	ErrorKindInvalidNetworkPassphrase ErrorKind = "network passphrase must not be empty"
	ErrorKindMalformedAsset           ErrorKind = "malformed asset"
	ErrorKindNotWASM                  ErrorKind = "not a version 1 WASM module"
	ErrorKindMalformedWASM            ErrorKind = "malformed WASM module"
	ErrorKindNoContractSpecSection    ErrorKind = "contract WASM does not contain a contractspecv0 section"
	ErrorKindInvalidWasm              ErrorKind = "invalid WebAssembly binary header"
	ErrorKindTypeNotFound             ErrorKind = "spec type not found"
	ErrorKindFunctionNotFound         ErrorKind = "function not found in contract spec"
	ErrorKindInvalidContractSpec      ErrorKind = "invalid contract spec"
)

func (kind ErrorKind) Error() string { return string(kind) }

// ErrEmptyInput is returned when an API receives an empty payload.
var ErrEmptyInput error = ErrorKindEmptyInput

// ErrInvalidBase64 is returned when an XDR payload is not valid base64.
var ErrInvalidBase64 error = ErrorKindInvalidBase64

// ErrInvalidXDR is returned when a payload cannot be decoded as XDR.
var ErrInvalidXDR error = ErrorKindInvalidXDR

// ErrNoMatch is returned when a payload does not decode as any known XDR type.
var ErrNoMatch error = ErrorKindNoMatch

// ErrUnknownType is returned when a caller names a type that is not registered.
var ErrUnknownType error = ErrorKindUnknownType

// ErrInvalidArgument is returned when a library function receives an invalid value.
var ErrInvalidArgument error = ErrorKindInvalidArgument

// ErrUnsupportedType is returned when an operation does not support a decoded XDR type.
var ErrUnsupportedType error = ErrorKindUnsupportedType

// ErrNotAsset is returned when a value offered for asset inspection is not a classic Asset.
var ErrNotAsset error = ErrorKindNotAsset

// ErrInvalidNetworkPassphrase is returned when an asset derivation has no network passphrase.
var ErrInvalidNetworkPassphrase error = ErrorKindInvalidNetworkPassphrase

// ErrMalformedAsset is returned when an asset contains invalid union data.
var ErrMalformedAsset error = ErrorKindMalformedAsset

// ErrNotWASM is returned when a contract's bytecode is not a version 1 WASM module.
var ErrNotWASM error = ErrorKindNotWASM

// ErrMalformedWASM is returned when a module's section structure or an embedded metadata stream is corrupt.
var ErrMalformedWASM error = ErrorKindMalformedWASM

// ErrNoContractSpecSection is returned when a WASM binary contains no contractspecv0 section.
var ErrNoContractSpecSection error = ErrorKindNoContractSpecSection

// ErrInvalidWasm is returned when the data does not start with the WASM magic header.
var ErrInvalidWasm error = ErrorKindInvalidWasm

// ErrTypeNotFound is returned when a spec type name is not defined in the contract spec.
var ErrTypeNotFound error = ErrorKindTypeNotFound

// ErrFunctionNotFound is returned when a function name is not defined in the contract spec.
var ErrFunctionNotFound error = ErrorKindFunctionNotFound

// ErrInvalidContractSpec is returned when contract spec data cannot be parsed.
var ErrInvalidContractSpec error = ErrorKindInvalidContractSpec
