package anima

import (
	"database/sql"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"time"
)

// timeType is the reflect.Type of time.Time, hoisted so both the typed-destination
// check and convertAssign compare against one value.
var timeType = reflect.TypeOf(time.Time{})

// scanEntities maps result rows into []*T. T may name the element directly
// (Select[User]) or the pointer (Select[*User]); scanReflect always builds
// *element rows, so the pointer form needs one extra boxing level to satisfy
// *T. Without it the type assertion panics on every row instead of erroring.
func scanEntities[T any](rows *sql.Rows, m *entityMeta) ([]*T, error) {
	vals, err := scanReflect(rows, m)
	if err != nil {
		return nil, err
	}
	out := make([]*T, 0, len(vals))
	var zero *T
	box := reflect.TypeOf(zero).Elem().Kind() == reflect.Ptr
	for _, v := range vals {
		if box {
			p := reflect.New(v.Type())
			p.Elem().Set(v)
			out = append(out, p.Interface().(*T))
			continue
		}
		out = append(out, v.Interface().(*T))
	}
	return out, nil
}

// scanReflect is the runtime-typed row mapping used by scenarios without a
// compile-time type (e.g. join back-filling).
// Unknown columns are skipped without an error.
//
// Scan boxes (one slot per column) are allocated once outside the row loop
// in a single contiguous backing array and reused across rows. database/sql
// contract requires the consumer to extract values before the next Scan, so a
// fresh box per row is unnecessary; the previous row's interface values are
// read and consumed before the next iteration overwrites them.
func scanReflect(rows *sql.Rows, m *entityMeta) ([]reflect.Value, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	targets := make([]*fieldMeta, len(cols))
	for i, col := range cols {
		if f, ok := m.byColumn[col]; ok {
			targets[i] = f
		} else if f, ok := m.byRef[toCamelName(col)]; ok {
			targets[i] = f
		}
	}
	// Typed scan destinations let database/sql convert the driver value
	// straight into the field type. For the text-protocol []byte cell that
	// costs one allocation (database/sql's own asString) instead of the two the
	// *any box path costs (bytes.Clone into the box, then string(b) in
	// convertAssign); for a time.Time cell it costs none at all, because
	// convertAssignRows copies the value directly into a *time.Time.
	//
	// Only fields database/sql can target on its own qualify: numeric, bool,
	// string and time.Time leaves, with or without pointers. Pointer fields
	// match convertAssign's own recursion — database/sql allocates the element,
	// recurses, and maps NULL to a nil pointer exactly as convertAssign does.
	// Fields carrying a Converter or a DBEnumScanner/sql.Scanner must still
	// route through convertAssign, which owns those semantics.
	typed := make([]reflect.Value, len(cols))
	box := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i, f := range targets {
		if typedScanDest(f) {
			typed[i] = reflect.New(f.Type)
			ptrs[i] = typed[i].Interface()
		} else {
			ptrs[i] = &box[i]
		}
	}
	out := make([]reflect.Value, 0, 16)
	for rows.Next() {
		v := reflect.New(m.typ)
		rv := v.Elem()
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		for i, f := range targets {
			if f == nil {
				continue
			}
			if typed[i].IsValid() {
				rv.Field(f.Index).Set(typed[i].Elem())
				continue
			}
			src := box[i]
			if err := convertAssign(rv.Field(f.Index), src, f); err != nil {
				return nil, &Error{Code: ErrorCodeMappingFailure, Msg: "column " + cols[i] + " -> field " + f.Name, Err: err}
			}
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// typedScanDest reports whether a column may be scanned directly into a typed
// destination (reflect.New(f.Type)) instead of an *any box, letting
// database/sql perform the conversion itself.
//
// It excludes fields whose value must still pass through convertAssign: a
// registered Converter at any pointer level, a DBEnumScanner/sql.Scanner
// implementation, or a leaf kind database/sql cannot target (interface, slice,
// map, and every struct except time.Time).
func typedScanDest(f *fieldMeta) bool {
	if f == nil {
		return false
	}
	if f.hasDBEnumScanner || f.hasSQLScanner {
		return false
	}
	leaf := f.Type
	for {
		if _, ok := lookupConverter(leaf); ok {
			return false
		}
		if leaf.Kind() != reflect.Ptr {
			break
		}
		leaf = leaf.Elem()
	}
	switch leaf.Kind() {
	case reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	case reflect.Struct:
		return leaf == timeType
	}
	return false
}

// convertAssign maps a driver value onto a struct field. The caller
// (scanReflect) always passes addressable fields from reflect.New, so Addr()
// interface assertions are safe here.
// NULL can only land on pointer/slice/map/interface targets or types
// implementing DBEnumScanner/sql.Scanner.
// convertAssign assigns src (a driver value or an already-converted value) to
// the settable reflect.Value dst, following anima's conversion priority.
//
// Priority: DBEnumScanner -> Converter registry -> sql.Scanner -> built-in
// scalar rules.
//
// The f argument carries the precomputed hasDBEnumScanner/hasSQLScanner hints
// so we can skip dst.Addr().Interface() boxing for fields that can't possibly
// match the corresponding interface (the common case for primitive columns).
// f may be nil for direct callers (e.g. white-box tests) — in that case all
// three branches are attempted, matching the pre-optimization behavior.
func convertAssign(dst reflect.Value, src any, f *fieldMeta) error {
	return assignValue(dst, src, f, false)
}

// applyNull lands a NULL on dst, which is only legal for targets that
// implement DBEnumScanner/sql.Scanner or are inherently nullable kinds
// (pointer/slice/map/interface); everything else is a mapping failure.
func applyNull(dst reflect.Value, f *fieldMeta) error {
	if f == nil || f.hasDBEnumScanner {
		if e, ok := dst.Addr().Interface().(DBEnumScanner); ok {
			return e.SetDatabaseValue(nil)
		}
	}
	if f == nil || f.hasSQLScanner {
		if sc, ok := dst.Addr().Interface().(sql.Scanner); ok {
			return sc.Scan(nil)
		}
	}
	switch dst.Kind() {
	case reflect.Ptr, reflect.Slice, reflect.Map, reflect.Interface:
		dst.Set(reflect.Zero(dst.Type()))
		return nil
	}
	return &Error{Code: ErrorCodeMappingFailure, Msg: "NULL into non-nullable field"}
}

// assignScalar applies the built-in conversion rules once no interface or
// registered converter claimed the value: []byte degrades to string, time.Time
// accepts only time.Time, and the scalar switch mirrors database/sql's width
// and strictness semantics. This is a flat dispatch table by design — every
// branch returns, and nothing here recurses.
func assignScalar(dst reflect.Value, src any) error {
	if b, ok := src.([]byte); ok {
		src = string(b)
	}
	if dst.Type() == timeType {
		if t, ok := src.(time.Time); ok {
			dst.Set(reflect.ValueOf(t))
			return nil
		}
		return &Error{Code: ErrorCodeMappingFailure, Msg: "cannot scan " + typeName(src) + " into time.Time"}
	}

	switch dst.Kind() {
	case reflect.String:
		if s, ok := src.(string); ok {
			dst.SetString(s)
			return nil
		}
	case reflect.Bool:
		switch v := src.(type) {
		case bool:
			dst.SetBool(v)
			return nil
		case int64:
			dst.SetBool(v != 0)
			return nil
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		switch v := src.(type) {
		case int64:
			if dst.OverflowInt(v) {
				return rangeError(v, dst.Type())
			}
			dst.SetInt(v)
			return nil
		case int:
			if dst.OverflowInt(int64(v)) {
				return rangeError(v, dst.Type())
			}
			dst.SetInt(int64(v))
			return nil
		case float64:
			// database/sql semantics: fractional (15.9) and out-of-range
			// values are errors; whole values (15.0) convert by bit width.
			if v != math.Trunc(v) || dst.OverflowInt(int64(v)) {
				return rangeError(v, dst.Type())
			}
			dst.SetInt(int64(v))
			return nil
		case string:
			n, err := parseIntStrict(v, dst.Type().Bits())
			if err != nil {
				return err
			}
			dst.SetInt(n)
			return nil
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		switch v := src.(type) {
		case int64:
			if v < 0 || dst.OverflowUint(uint64(v)) {
				return rangeError(v, dst.Type())
			}
			dst.SetUint(uint64(v))
			return nil
		case uint64:
			if dst.OverflowUint(v) {
				return rangeError(v, dst.Type())
			}
			dst.SetUint(v)
			return nil
		case string:
			n, err := parseUintStrict(v, dst.Type().Bits())
			if err != nil {
				return err
			}
			dst.SetUint(n)
			return nil
		}
	case reflect.Float32, reflect.Float64:
		switch v := src.(type) {
		case float64:
			dst.SetFloat(v)
			return nil
		case int64:
			dst.SetFloat(float64(v))
			return nil
		}
	case reflect.Slice:
		if dst.Type().Elem().Kind() == reflect.Uint8 {
			switch v := src.(type) {
			case []byte:
				cp := make([]byte, len(v))
				copy(cp, v)
				dst.SetBytes(cp)
				return nil
			case string:
				dst.SetBytes([]byte(v))
				return nil
			}
		}
	}
	return &Error{Code: ErrorCodeMappingFailure, Msg: "unsupported scan: " + typeName(src) + " -> " + dst.Type().String()}
}

// assignValue is convertAssign's implementation. skipConverter disables the
// Converter-registry branch for this hop: it is set after a converter already
// produced a cross-type (or nil) value, because the registry is keyed by
// dst.Type() — recursing back into it would re-hit the same converter and
// never converge (stack exhaustion on any converter that doesn't eventually
// return exactly dst.Type()). At most one converter hop happens per value;
// everything past it goes through the built-in scalar rules, which always
// terminate.
func assignValue(dst reflect.Value, src any, f *fieldMeta, skipConverter bool) error {
	if src == nil {
		return applyNull(dst, f)
	}
	// Read-side enum/custom type interface
	if f == nil || f.hasDBEnumScanner {
		if e, ok := dst.Addr().Interface().(DBEnumScanner); ok {
			return e.SetDatabaseValue(src)
		}
	}
	// Registered type converter
	if !skipConverter {
		if c, ok := lookupConverter(dst.Type()); ok {
			converted, err := c.FromDatabase(src)
			if err != nil {
				return err
			}
			if converted == nil {
				return assignValue(dst, nil, f, true)
			}
			if reflect.TypeOf(converted) == dst.Type() {
				dst.Set(reflect.ValueOf(converted))
				return nil
			}
			// Cross-type hop: let the built-in scalar rules land the value.
			// Recursing with the registry enabled would re-hit this same
			// converter (see assignValue's doc) and never terminate.
			return assignValue(dst, converted, f, true)
		}
	}
	// sql.Scanner targets get the raw value
	if f == nil || f.hasSQLScanner {
		if sc, ok := dst.Addr().Interface().(sql.Scanner); ok {
			return sc.Scan(src)
		}
	}
	if dst.Kind() == reflect.Ptr {
		if dst.IsNil() {
			dst.Set(reflect.New(dst.Type().Elem()))
		}
		return assignValue(dst.Elem(), src, f, skipConverter)
	}
	return assignScalar(dst, src)
}

func typeName(v any) string {
	if v == nil {
		return "NULL"
	}
	return reflect.TypeOf(v).String()
}

func rangeError(src any, t reflect.Type) error {
	return &Error{Code: ErrorCodeMappingFailure, Msg: fmt.Sprintf("value out of range: %v -> %s", src, t)}
}

func parseIntStrict(s string, bits int) (int64, error) {
	return strconv.ParseInt(s, 10, bits)
}

func parseUintStrict(s string, bits int) (uint64, error) {
	return strconv.ParseUint(s, 10, bits)
}
