package anima

import (
	"context"
	"database/sql"

	"github.com/fxcl/anima/dialect"
)

// Set sets one column (update mode; accepts a Go field name or column name).
func (q *Query[T]) Set(field string, value any) *Query[T] {
	if q.checkErr() != nil {
		return q
	}
	if q.mode != modeUpdate {
		q.err = newError(ErrorCodeModeMismatch, "Set() only allowed in update mode")
		return q
	}
	f, err := fieldOf(q.m, field)
	if err != nil {
		q.err = err
		return q
	}
	// Later writes to the same column overwrite, avoiding a repeated
	// "SET a = ?, a = ?".
	for i, cv := range q.setCols {
		if cv.Name == f.Column {
			q.setCols[i].Value = value
			return q
		}
	}
	q.setCols = append(q.setCols, dialect.ColumnValue{Name: f.Column, Value: value})
	return q
}

// Execute terminates update/delete mode.
func (q *Query[T]) Execute(ctx context.Context) (int, error) {
	if err := q.checkErr(); err != nil {
		return 0, err
	}
	switch q.mode {
	case modeUpdate:
		return q.execUpdate(ctx)
	case modeDelete:
		return q.execDelete(ctx)
	}
	return 0, newError(ErrorCodeModeMismatch, "Execute() only allowed in update/delete mode")
}

// UpdateById runs the update by primary key.
func (q *Query[T]) UpdateById(ctx context.Context, id any) (int, error) {
	if err := q.checkErr(); err != nil {
		return 0, err
	}
	if q.mode != modeUpdate {
		return 0, newError(ErrorCodeModeMismatch, "UpdateById() only allowed in update mode")
	}
	if err := q.m.requirePK(); err != nil {
		return 0, err
	}
	q.WhereCol(q.m.pk).Eq(id)
	return q.execUpdate(ctx)
}

// execUpdate is the single write exit shared by Execute() and UpdateById():
// it renders "UPDATE ... SET ... WHERE ..." through the dialect and binds the
// parameters. Placeholder order must match the SQL: SET values first, then
// the WHERE arguments collected by sqlParams().
func (q *Query[T]) execUpdate(ctx context.Context) (int, error) {
	if err := q.checkErr(); err != nil {
		return 0, err
	}
	if len(q.setCols) == 0 {
		return 0, newError(ErrorCodeInvalidArgument, "update %s: nothing to set", q.m.typ.Name())
	}
	ex, err := q.a.sqlable(ctx)
	if err != nil {
		return 0, err
	}
	p := q.sqlParams()
	p.SetColumns = q.setCols
	args := make([]any, 0, len(q.setCols)+len(p.Params))
	for _, c := range q.setCols {
		args = append(args, c.Value)
	}
	args = append(args, p.Params...)
	res, err := q.a.execCtx(ctx, ex, q.a.dialect.Update(p), args)
	if err != nil {
		return 0, err
	}
	return affected(res)
}

// execDelete is the delete-mode exit used by Execute(); the WHERE clause comes
// entirely from whatever the caller chained before Execute().
func (q *Query[T]) execDelete(ctx context.Context) (int, error) {
	if err := q.checkErr(); err != nil {
		return 0, err
	}
	ex, err := q.a.sqlable(ctx)
	if err != nil {
		return 0, err
	}
	p := q.sqlParams()
	res, err := q.a.execCtx(ctx, ex, q.a.dialect.Delete(p), p.Params)
	if err != nil {
		return 0, err
	}
	return affected(res)
}

// affected reads the number of affected rows (shared by the exec and model
// layers).
func affected(res sql.Result) (int, error) {
	n, err := res.RowsAffected()
	return int(n), err
}
