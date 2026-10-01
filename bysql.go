package anima

import (
	"context"
	"database/sql"
	"reflect"
)

// Execute runs arbitrary raw SQL; it reuses the transaction connection when
// called inside a transaction ctx.
func Execute(ctx context.Context, rawSQL string, args ...any) (sql.Result, error) {
	a := Of()
	if a == nil {
		return nil, errNotInit()
	}
	ex, err := a.sqlable(ctx)
	if err != nil {
		return nil, err
	}
	return a.execCtx(ctx, ex, rawSQL, args)
}

// QuerySQL reads the scalar column of a raw SQL result.
// V is a scalar type (string/int/float64/bool/time.Time) or a pointer to one
// (for nullable columns).
func QuerySQL[V any](ctx context.Context, rawSQL string, args ...any) ([]V, error) {
	a := Of()
	if a == nil {
		return nil, errNotInit()
	}
	ex, err := a.sqlable(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := a.queryCtx(ctx, ex, rawSQL, args)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []V{}
	for rows.Next() {
		var v V
		if err := scanScalarRow(&v, rows); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ScalarSQL reads a single scalar; returns the zero value of V when there is
// no result.
func ScalarSQL[V any](ctx context.Context, rawSQL string, args ...any) (V, error) {
	var zero V
	list, err := QuerySQL[V](ctx, rawSQL, args...)
	if err != nil {
		return zero, err
	}
	if len(list) == 0 {
		return zero, nil
	}
	return list[0], nil
}

func scanScalarRow[V any](v *V, rows *sql.Rows) error {
	rv := reflect.ValueOf(v).Elem()
	ptr := reflect.New(rv.Type())
	if err := rows.Scan(ptr.Interface()); err != nil {
		return err
	}
	// For a pointer V the destination handed to Scan is **V, whose
	// database/sql NULL contract is SetZero — a plain *string destination
	// cannot receive NULL ("converting NULL to string is unsupported"),
	// which is exactly why the pointer form exists for nullable columns.
	rv.Set(ptr.Elem())
	return nil
}
