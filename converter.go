package anima

import (
	"reflect"
	"sync"
)

// Converter converts values between the Go type and the database
// representation.
type Converter interface {
	Target() reflect.Type
	ToDatabase(v any) (any, error)
	FromDatabase(v any) (any, error)
}

// DBEnum is an optional interface for entity field types: the value written
// to the database is the result of ToDatabaseValue.
type DBEnum interface {
	ToDatabaseValue() any
}

// DBEnumScanner is the read-side fill interface: preferred over built-in
// mappings when the field type implements it with a pointer receiver.
type DBEnumScanner interface {
	SetDatabaseValue(v any) error
}

var converterRegistry sync.Map // reflect.Type -> Converter

// AddConverter registers custom type converters in the global table.
func AddConverter(cs ...Converter) {
	for _, c := range cs {
		converterRegistry.Store(c.Target(), c)
	}
}

// AddConverter registers converters in the global table and returns the
// receiver for chaining.
func (a *Anima) AddConverter(cs ...Converter) *Anima {
	AddConverter(cs...)
	return a
}

func lookupConverter(t reflect.Type) (Converter, bool) {
	c, ok := converterRegistry.Load(t)
	if !ok {
		return nil, false
	}
	return c.(Converter), true
}

// bind converts write-path parameters: DBEnum -> DB value; registered
// Converter -> ToDatabase. Conversion errors propagate to the caller.
func bind(v any) (any, error) {
	if v == nil {
		return nil, nil
	}
	if e, ok := v.(DBEnum); ok {
		return bind(e.ToDatabaseValue())
	}
	if c, ok := lookupConverter(reflect.TypeOf(v)); ok {
		return c.ToDatabase(v)
	}
	return v, nil
}

// bindParams converts a batch of parameters; the first failure aborts.
//
// The common case — no DBEnum values and no registered Converter matching any
// parameter — needs no conversion at all, so the batch is returned as-is
// instead of allocating a copy. The pre-scan below is allocation-free (type
// assertion, reflect.TypeOf, sync.Map.Load), so it costs CPU but not heap.
func bindParams(params []any) ([]any, error) {
	if len(params) == 0 {
		return params, nil
	}
	needsBind := false
	for _, p := range params {
		if p == nil {
			continue
		}
		if _, ok := p.(DBEnum); ok {
			needsBind = true
			break
		}
		if _, ok := lookupConverter(reflect.TypeOf(p)); ok {
			needsBind = true
			break
		}
	}
	if !needsBind {
		return params, nil
	}
	out := make([]any, len(params))
	for i, p := range params {
		b, err := bind(p)
		if err != nil {
			return nil, err
		}
		out[i] = b
	}
	return out, nil
}
