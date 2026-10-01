package anima

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestConvertAssignNullAndBranchPaths(t *testing.T) {
	// NULL into pointer/slice/map/interface kinds zero-sets them
	var p *int
	pv := reflect.ValueOf(&p).Elem()
	if err := convertAssign(pv, nil, nil); err != nil || p != nil {
		t.Fatalf("NULL into **int: err=%v p=%v", err, p)
	}

	// NULL into a non-nullable scalar must fail, not silently zero
	var s string
	sv := reflect.ValueOf(&s).Elem()
	err := convertAssign(sv, nil, nil)
	var aerr *Error
	if !errors.As(err, &aerr) || aerr.Code != ErrorCodeMappingFailure {
		t.Fatalf("NULL into string: got %v, want ErrorCodeMappingFailure", err)
	}

	// []byte source lands as a Go string
	if err := convertAssign(sv, []byte("abc"), nil); err != nil || s != "abc" {
		t.Fatalf("[]byte->string: err=%v s=%q", err, s)
	}

	// bool accepts int64 truthiness
	var b bool
	bv := reflect.ValueOf(&b).Elem()
	if err := convertAssign(bv, int64(1), nil); err != nil || !b {
		t.Fatalf("int64->bool: err=%v b=%v", err, b)
	}

	// time.Time target: success and failure with type name in the message
	var ts time.Time
	tv := reflect.ValueOf(&ts).Elem()
	now := time.Now()
	if err := convertAssign(tv, now, nil); err != nil {
		t.Fatalf("time.Time<-time.Time: %v", err)
	}
	err = convertAssign(tv, int64(42), nil)
	if !errors.As(err, &aerr) || aerr.Code != ErrorCodeMappingFailure {
		t.Fatalf("time.Time<-int64: got %v, want ErrorCodeMappingFailure", err)
	}

	// pointer field: allocates the element and recurses
	var pi *int
	piv := reflect.ValueOf(&pi).Elem()
	if err := convertAssign(piv, int64(5), nil); err != nil || pi == nil || *pi != 5 {
		t.Fatalf("int64->*int: err=%v pi=%v", err, pi)
	}
}

type convProbe int

type probeConverter struct{ mode int }

func (probeConverter) Target() reflect.Type { return reflect.TypeOf(convProbe(0)) }

func (probeConverter) ToDatabase(v any) (any, error) { return int64(v.(convProbe)), nil }

func (c probeConverter) FromDatabase(v any) (any, error) {
	switch c.mode {
	case 0:
		return nil, errors.New("probe boom")
	case 1:
		return nil, nil
	default:
		return int64(42), nil
	}
}

func TestConvertAssignConverterPaths(t *testing.T) {
	var x convProbe
	xv := reflect.ValueOf(&x).Elem()

	AddConverter(probeConverter{mode: 0})
	err := convertAssign(xv, int64(1), nil)
	if err == nil || err.Error() != "probe boom" {
		t.Fatalf("converter error not propagated: %v", err)
	}

	AddConverter(probeConverter{mode: 1})
	var aerr *Error
	err = convertAssign(xv, int64(1), nil)
	if !errors.As(err, &aerr) || aerr.Code != ErrorCodeMappingFailure {
		t.Fatalf("nil converter output should recurse to NULL rule: %v", err)
	}
}
func TestConvertAssignConverterCrossTypeConverges(t *testing.T) {
	// Regression pin for the converter recursion defect: FromDatabase returning
	// a type other than dst.Type() used to recurse straight back into the same
	// registry entry and exhaust the stack. The fix routes the cross-type hop
	// through the built-in scalar rules instead, so it must now converge.
	AddConverter(probeConverter{mode: 2})

	var x convProbe
	xv := reflect.ValueOf(&x).Elem()
	if err := convertAssign(xv, int64(1), nil); err != nil {
		t.Fatal(err)
	}
	if x != 42 {
		t.Fatalf("cross-type converter hop landed wrong value: x=%d, want 42", x)
	}
}
