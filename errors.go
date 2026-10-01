package anima

import "fmt"

// Error codes: 1000/1001 come from the upstream convention; 1002-1006 are
// extensions of this package (build-time field validation, missing primary
// key, mapping failure, mode guard, and argument validation).
const (
	ErrorCodeSql2oNotInit    = 1000
	ErrorCodeEmptyFromClause = 1001
	ErrorCodeUnknownField    = 1002
	ErrorCodeNoPrimaryKey    = 1003
	ErrorCodeMappingFailure  = 1004
	ErrorCodeModeMismatch    = 1005
	ErrorCodeInvalidArgument = 1006
)

// Error is the anima exception carrying an error code.
type Error struct {
	Code int
	Msg  string
	Err  error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("anima: %s (code %d): %v", e.Msg, e.Code, e.Err)
	}
	return fmt.Sprintf("anima: %s (code %d)", e.Msg, e.Code)
}

func (e *Error) Unwrap() error { return e.Err }

func newError(code int, format string, args ...any) *Error {
	return &Error{Code: code, Msg: fmt.Sprintf(format, args...)}
}

// errNotInit is the single constructor for the not-initialized error.
func errNotInit() error {
	return &Error{Code: ErrorCodeSql2oNotInit, Msg: "anima not initialized, call anima.Open first"}
}
