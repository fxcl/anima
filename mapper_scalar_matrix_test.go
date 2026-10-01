package anima

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

// assign/convert helpers for the scalar matrix tests.
func assign[T any](t *testing.T, src any) T {
	t.Helper()
	var out T
	dst := reflect.ValueOf(&out).Elem()
	if err := convertAssign(dst, src, nil); err != nil {
		t.Fatalf("convertAssign(%T -> %T): %v", src, out, err)
	}
	return out
}

func assignErr(t *testing.T, src any, target any) error {
	t.Helper()
	dst := reflect.ValueOf(target).Elem()
	return convertAssign(dst, src, nil)
}

// NULL must be rejected for non-nullable scalar fields instead of silently
// becoming the zero value; nullable kinds keep their SetZero contract.
func TestConvertAssignNullIntoNonNullableErrors(t *testing.T) {
	var i int
	err := assignErr(t, nil, &i)
	if err == nil {
		t.Fatal("NULL into a non-nullable int must fail")
	}
	var aErr *Error
	if !errors.As(err, &aErr) || aErr.Code != ErrorCodeMappingFailure {
		t.Fatalf("NULL-into-scalar must be ErrorCodeMappingFailure, got %v", err)
	}

	var p *int
	if err := assignErr(t, nil, &p); err != nil {
		t.Fatalf("NULL into *int must set nil, got %v", err)
	}
	if p != nil {
		t.Fatal("NULL into *int must leave nil")
	}
	var s []string
	if err := assignErr(t, nil, &s); err != nil {
		t.Fatalf("NULL into a slice must set nil, got %v", err)
	}
	var m map[string]int
	if err := assignErr(t, nil, &m); err != nil {
		t.Fatalf("NULL into a map must set nil, got %v", err)
	}
	var iface any
	if err := assignErr(t, nil, &iface); err != nil {
		t.Fatalf("NULL into an interface must set nil, got %v", err)
	}
	if iface != nil {
		t.Fatal("NULL into an interface must leave nil")
	}
}

// Unsigned destinations accept int64 (non-negative), uint64, and numeric
// strings; negative and out-of-range sources are range errors.
func TestConvertAssignUintMatrix(t *testing.T) {
	if got := assign[uint8](t, int64(200)); got != 200 {
		t.Fatalf("uint8 <- int64: got %d", got)
	}
	if got := assign[uint32](t, uint64(4000000000)); got != 4000000000 {
		t.Fatalf("uint32 <- uint64: got %d", got)
	}
	if got := assign[uint16](t, "65535"); got != 65535 {
		t.Fatalf("uint16 <- string: got %d", got)
	}
	if err := assignErr(t, int64(-1), new(uint8)); err == nil {
		t.Fatal("uint8 <- negative int64 must be a range error")
	}
	if err := assignErr(t, uint64(300), new(uint8)); err == nil {
		t.Fatal("uint8 <- 300 must overflow")
	}
	if err := assignErr(t, "-1", new(uint)); err == nil {
		t.Fatal("uint <- \"-1\" must fail")
	}
	if err := assignErr(t, "nope", new(uint)); err == nil {
		t.Fatal("uint <- garbage string must fail")
	}
}

// Float destinations accept float64 and int64 sources; fractional floats
// into ints are rejected but whole floats convert by bit width (database/sql
// semantics).
func TestConvertAssignFloatAndIntEdgeMatrix(t *testing.T) {
	if got := assign[float64](t, int64(42)); got != 42 {
		t.Fatalf("float64 <- int64: got %v", got)
	}
	if got := assign[float32](t, float64(1.5)); got != 1.5 {
		t.Fatalf("float32 <- float64: got %v", got)
	}
	if err := assignErr(t, 15.9, new(int64)); err == nil {
		t.Fatal("int64 <- fractional float must be a range error")
	}
	if got := assign[int32](t, float64(15.0)); got != 15 {
		t.Fatalf("int32 <- whole float must convert, got %d", got)
	}
	if err := assignErr(t, float64(300), new(int8)); err == nil {
		t.Fatal("int8 <- 300.0 must overflow")
	}
	if err := assignErr(t, "12x", new(int)); err == nil {
		t.Fatal("int <- garbage string must fail")
	}
	if err := assignErr(t, int64(200), new(int8)); err == nil {
		t.Fatal("int8 <- 200 must overflow")
	}
}

// Strings are strict: only string sources land in string fields; []byte
// destinations copy bytes (mutating the source afterwards must not leak into
// the destination) and accept strings; non-uint8 slices are unsupported.
func TestConvertAssignStringAndBytesMatrix(t *testing.T) {
	if got := assign[string](t, "hello"); got != "hello" {
		t.Fatalf("string <- string: got %q", got)
	}
	if err := assignErr(t, int64(5), new(string)); err == nil {
		t.Fatal("string <- int64 must be unsupported (strict typing)")
	}

	bs := assign[[]byte](t, []byte("raw"))
	if string(bs) != "raw" {
		t.Fatalf("[]byte <- []byte: got %q", bs)
	}
	src := []byte("mutate-me")
	bs = assign[[]byte](t, src)
	src[0] = 'X'
	if string(bs) != "mutate-me" {
		t.Fatalf("[]byte assignment must copy, src mutation leaked: %q", bs)
	}
	if got := assign[[]byte](t, "from-string"); string(got) != "from-string" {
		t.Fatalf("[]byte <- string: got %q", got)
	}

	if err := assignErr(t, []byte("x"), new([]int)); err == nil {
		t.Fatal("[]int <- bytes must be unsupported")
	}
}

// time.Time only accepts time.Time sources; everything else is a mapping
// failure naming both types.
func TestConvertAssignTimeMatrix(t *testing.T) {
	now := time.Now()
	if got := assign[time.Time](t, now); !got.Equal(now) {
		t.Fatalf("time.Time <- time.Time: got %v", got)
	}
	err := assignErr(t, int64(5), new(time.Time))
	if err == nil {
		t.Fatal("time.Time <- int64 must fail")
	}
	var aErr *Error
	if !errors.As(err, &aErr) || aErr.Code != ErrorCodeMappingFailure {
		t.Fatalf("time mismatch must be ErrorCodeMappingFailure, got %v", err)
	}
}
