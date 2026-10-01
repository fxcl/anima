package anima

import (
	"context"
	"reflect"

	"github.com/fxcl/anima/dialect"
)

// Save inserts an entity. Nil fields (nil pointers, etc.) are skipped; an
// auto-increment PK with a zero value is skipped so the DB assigns it.
func Save[T any](ctx context.Context, model *T) (*ResultKey, error) {
	a, m, rv, err := resolveEntity[T](model)
	if err != nil {
		return nil, err
	}
	cols, args := nonNilColumns(m, rv)
	p := &dialect.SQLParams{TableName: m.table, Columns: cols, Dialect: a.dialect}
	ex, err := a.sqlable(ctx)
	if err != nil {
		return nil, err
	}
	res, err := a.execCtx(ctx, ex, a.dialect.Insert(p), args)
	if err != nil {
		return nil, err
	}
	key, err := res.LastInsertId()
	if err != nil {
		// The driver does not support auto-increment keys (possible on PG;
		// not verified against a real server): fall back to the explicit PK
		// when the entity has one.
		if m.pkIndex >= 0 {
			pv := rv.Field(m.fields[m.pkIndex].Index)
			if k, ok := toInt64(pv); ok {
				return &ResultKey{key: k}, nil
			}
		}
		return nil, err
	}
	// key == 0 fallback: drivers disagree on what to report when there is no
	// auto-increment value (not verified per driver).
	if key == 0 && m.pkIndex >= 0 {
		if k, ok := toInt64(rv.Field(m.fields[m.pkIndex].Index)); ok {
			key = k
		}
	}
	return &ResultKey{key: key}, nil
}

// Update updates all non-nil fields of an entity by primary key.
func Update[T any](ctx context.Context, model *T) (int, error) {
	a, m, rv, err := resolveEntity[T](model)
	if err != nil {
		return 0, err
	}
	if err := m.requirePK(); err != nil {
		return 0, err
	}
	cols, args := nonNilColumns(m, rv, m.pkIndex)
	if len(cols) == 0 {
		return 0, nil
	}
	pkVal := rv.Field(m.fields[m.pkIndex].Index)
	if isNilValue(pkVal) {
		return 0, newError(ErrorCodeNoPrimaryKey, "primary key of %s is nil", m.typ.Name())
	}
	p := &dialect.SQLParams{
		TableName:    m.table,
		SetColumns:   cols,
		ConditionSQL: m.pk + " = ?",
		Params:       append(args, unwrap(pkVal)),
		Dialect:      a.dialect,
	}
	ex, err := a.sqlable(ctx)
	if err != nil {
		return 0, err
	}
	res, err := a.execCtx(ctx, ex, a.dialect.Update(p), p.Params)
	if err != nil {
		return 0, err
	}
	return affected(res)
}

// Delete deletes by the entity's non-nil fields: without a custom condition,
// all non-nil fields form the WHERE clause.
func Delete[T any](ctx context.Context, model *T) (int, error) {
	a, m, rv, err := resolveEntity[T](model)
	if err != nil {
		return 0, err
	}
	cols, args := nonNilColumns(m, rv)
	if len(cols) == 0 {
		return 0, newError(ErrorCodeEmptyFromClause, "delete %s: all fields are nil", m.typ.Name())
	}
	p := &dialect.SQLParams{TableName: m.table, Columns: cols, Dialect: a.dialect}
	ex, err := a.sqlable(ctx)
	if err != nil {
		return 0, err
	}
	res, err := a.execCtx(ctx, ex, a.dialect.Delete(p), args)
	if err != nil {
		return 0, err
	}
	return affected(res)
}

// DeleteById deletes by primary key.
func DeleteById[T any](ctx context.Context, id any) (int, error) {
	q := DeleteFrom[T]()
	if err := q.checkErr(); err != nil {
		return 0, err
	}
	if err := q.m.requirePK(); err != nil {
		return 0, err
	}
	return q.WhereCol(q.m.pk).Eq(id).Execute(ctx)
}

// SaveBatch inserts a batch inside one transaction; any failure rolls back
// the whole batch. Element errors are wrapped with Unchecked so the default
// RollbackWhen triggers that rollback; the returned error still unwraps to
// the original (*Error, io.EOF, ...).
// It returns the underlying error rather than an AtomicResult (Go idiom); use
// anima.Atomic directly when observable rollback state is needed.
func SaveBatch[T any](ctx context.Context, models []*T) error {
	r := Atomic(ctx, func(ctx context.Context) error {
		for _, m := range models {
			if _, err := Save(ctx, m); err != nil {
				return Unchecked(err)
			}
		}
		return nil
	})
	return r.Error()
}

// DeleteBatch deletes a batch by primary keys. An empty id list is a no-op;
// multiple ids render a single IN statement.
func DeleteBatch[T any](ctx context.Context, ids ...any) (int, error) {
	q := DeleteFrom[T]()
	if err := q.checkErr(); err != nil {
		return 0, err
	}
	if err := q.m.requirePK(); err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	return q.WhereCol(q.m.pk).In(ids...).Execute(ctx)
}

func resolveEntity[T any](model *T) (*Anima, *entityMeta, reflect.Value, error) {
	a := Of()
	if a == nil {
		return nil, nil, reflect.Value{}, errNotInit()
	}
	rv := reflect.ValueOf(model).Elem()
	if rv.Kind() != reflect.Struct {
		return nil, nil, reflect.Value{}, newError(ErrorCodeEmptyFromClause, "entity must be a pointer to struct")
	}
	return a, a.metaOf(rv.Type()), rv, nil
}

// nonNilColumns collects non-nil columns; skipIdx lists extra field indices to
// skip (e.g. the PK column on Update).
func nonNilColumns(m *entityMeta, rv reflect.Value, skipIdx ...int) ([]dialect.ColumnValue, []any) {
	// len(m.fields) is the upper bound for both slices; pre-sizing avoids the
	// append-growth chain.
	cols := make([]dialect.ColumnValue, 0, len(m.fields))
	args := make([]any, 0, len(m.fields))
	for i, f := range m.fields {
		if containsInt(skipIdx, i) {
			continue
		}
		fv := rv.Field(f.Index)
		if isNilValue(fv) {
			continue
		}
		// Skip an auto-increment PK with a zero value: the Go zero value
		// cannot express SQL NULL.
		if f.IsPK && isZeroValue(fv) {
			continue
		}
		// unwrap boxes the field value into an any, which allocates; call it
		// once and share the result between the column list and the args.
		v := unwrap(fv)
		cols = append(cols, dialect.ColumnValue{Name: f.Column, Value: v})
		args = append(args, v)
	}
	return cols, args
}

func isNilValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Ptr, reflect.Interface, reflect.Slice, reflect.Map:
		return v.IsNil()
	}
	return false
}

func containsInt(list []int, v int) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

func isZeroValue(v reflect.Value) bool {
	return v.IsZero()
}

func unwrap(v reflect.Value) any {
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return nil
		}
		return v.Elem().Interface()
	}
	return v.Interface()
}

func toInt64(v reflect.Value) (int64, bool) {
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return 0, false
		}
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int(), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int64(v.Uint()), true
	}
	return 0, false
}
