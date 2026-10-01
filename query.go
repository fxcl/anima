package anima

import (
	"context"
	"database/sql"
	"iter"
	"reflect"
	"slices"
	"strings"

	"github.com/fxcl/anima/dialect"
)

// qMode is the query mode.
type qMode int

const (
	modeSelect qMode = iota
	modeUpdate
	modeDelete
)

// Query is the query builder + executor: one object serves the
// select/update/delete modes, chained conditions apply in any mode, select is
// terminated by All/One/Count, update/delete by Execute/UpdateById.
// A Query is single-use: Select[T]() / UpdateFrom[T]() / DeleteFrom[T]()
// each return a fresh object.
type Query[T any] struct {
	a         *Anima
	m         *entityMeta
	err       error
	mode      qMode
	cond      string // WHERE body (internal AND/OR connectors included, no leading connector)
	params    []any
	cols      string // explicit select columns (empty = *)
	order     string // ORDER BY body (leading space), e.g. " id desc"
	limitN    int
	limitSet  bool                  // Limit() was called explicitly (distinguishes limitN==0 from unset)
	excluded  []string              // exclude columns (already resolved to column names)
	customSQL string                // raw SQL pass-through
	setCols   []dialect.ColumnValue // ordered SET pairs in update mode
	joins     []*JoinSpec           // association back-fills
}

// Select starts a query.
func Select[T any]() *Query[T] { return newQuery[T](modeSelect) }

// UpdateFrom starts an update DSL.
func UpdateFrom[T any]() *Query[T] { return newQuery[T](modeUpdate) }

// DeleteFrom starts a delete DSL.
func DeleteFrom[T any]() *Query[T] { return newQuery[T](modeDelete) }

func newQuery[T any](mode qMode) *Query[T] {
	q := &Query[T]{mode: mode}
	a := Of()
	if a == nil {
		q.err = errNotInit()
		return q
	}
	var zero T
	t := reflect.TypeOf(zero)
	if t == nil {
		q.err = newError(ErrorCodeEmptyFromClause, "cannot resolve entity type")
		return q
	}
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		q.err = newError(ErrorCodeEmptyFromClause, "%s is not a struct entity", t)
		return q
	}
	q.a = a
	q.m = a.metaOf(t)
	return q
}

// checkErr returns the aggregated build-time/environment error.
// newQuery guarantees a/m are ready when err==nil, so no second nil check is
// needed.
func (q *Query[T]) checkErr() error {
	return q.err
}

func (q *Query[T]) addCond(sep, frag string, args ...any) *Query[T] {
	if q.cond == "" {
		q.cond = frag
	} else {
		q.cond += " " + sep + " " + frag
	}
	q.params = append(q.params, args...)
	return q
}

// Where appends a raw condition fragment; multiple fragments join with AND.
func (q *Query[T]) Where(sqlFrag string, args ...any) *Query[T] {
	return q.addCond("AND", sqlFrag, args...)
}

// And is the same as Where.
func (q *Query[T]) And(sqlFrag string, args ...any) *Query[T] {
	return q.addCond("AND", sqlFrag, args...)
}

// Or joins a condition with OR; the whole fragment is parenthesized so
// mixing with AND keeps the intended precedence.
func (q *Query[T]) Or(sqlFrag string, args ...any) *Query[T] {
	return q.addCond("OR", "("+sqlFrag+")", args...)
}

// WhereCol starts a field condition.
// name accepts a Go field name ("Age") or a column name ("age"); unknown
// fields are recorded as errors and surface at execution.
func (q *Query[T]) WhereCol(name string) *Cond[T] {
	if q.checkErr() != nil {
		return &Cond[T]{q: q}
	}
	f, err := fieldOf(q.m, name)
	if err != nil {
		q.err = err
		return &Cond[T]{q: q}
	}
	return &Cond[T]{q: q, col: f.Column}
}

// Cond is the field condition operator set.
type Cond[T any] struct {
	q   *Query[T]
	col string
}

func (c *Cond[T]) op(op string, v any) *Query[T] {
	if c.q.checkErr() != nil {
		return c.q
	}
	return c.q.addCond("AND", c.col+op+"?", v)
}

func (c *Cond[T]) Eq(v any) *Query[T]   { return c.op(" = ", v) }
func (c *Cond[T]) Ne(v any) *Query[T]   { return c.op(" != ", v) }
func (c *Cond[T]) Gt(v any) *Query[T]   { return c.op(" > ", v) }
func (c *Cond[T]) Ge(v any) *Query[T]   { return c.op(" >= ", v) }
func (c *Cond[T]) Lt(v any) *Query[T]   { return c.op(" < ", v) }
func (c *Cond[T]) Le(v any) *Query[T]   { return c.op(" <= ", v) }
func (c *Cond[T]) Like(v any) *Query[T] { return c.op(" like ", v) }

// In is a set condition. Empty arguments skip the whole condition instead of
// generating an "IN ()" that breaks the query.
func (c *Cond[T]) In(values ...any) *Query[T] {
	if c.q.checkErr() != nil {
		return c.q
	}
	if len(values) == 0 {
		return c.q
	}
	marks := make([]byte, 0, len(values)*3)
	for i := range values {
		if i > 0 {
			marks = append(marks, ", "...)
		}
		marks = append(marks, '?')
	}
	frag := c.col + " IN (" + string(marks) + ")"
	return c.q.addCond("AND", frag, values...)
}

// Between is a range condition, joining the bounds with a lowercase "and".
func (c *Cond[T]) Between(a, b any) *Query[T] {
	if c.q.checkErr() != nil {
		return c.q
	}
	return c.q.addCond("AND", c.col+" BETWEEN ? and ?", a, b)
}

// IsNull / NotNull
func (c *Cond[T]) IsNull() *Query[T] {
	if c.q.checkErr() != nil {
		return c.q
	}
	return c.q.addCond("AND", c.col+" IS NULL")
}

func (c *Cond[T]) NotNull() *Query[T] {
	if c.q.checkErr() != nil {
		return c.q
	}
	return c.q.addCond("AND", c.col+" IS NOT NULL")
}

// NotEmpty appends the Java notEmpty shape: the column followed by an
// "not equal to the empty string" literal.
func (c *Cond[T]) NotEmpty() *Query[T] {
	if c.q.checkErr() != nil {
		return c.q
	}
	return c.q.addCond("AND", c.col+" != ''")
}

// Cols sets the select columns (Go field names or column names). It can be
// called only once.
func (q *Query[T]) Cols(names ...string) *Query[T] {
	if q.checkErr() != nil {
		return q
	}
	if q.cols != "" {
		q.err = newError(ErrorCodeInvalidArgument, "Cols method can only be called once")
		return q
	}
	cols := make([]string, 0, len(names))
	for _, n := range names {
		f, err := fieldOf(q.m, n)
		if err != nil {
			q.err = err
			return q
		}
		cols = append(cols, f.Column)
	}
	q.cols = strings.Join(cols, ", ")
	return q
}

// Exclude removes columns from the select list. Column names accept a Go
// field name or a column name; Cols takes priority when both are used.
func (q *Query[T]) Exclude(names ...string) *Query[T] {
	if q.checkErr() != nil {
		return q
	}
	for _, n := range names {
		f, err := fieldOf(q.m, n)
		if err != nil {
			q.err = err
			return q
		}
		q.excluded = append(q.excluded, f.Column)
	}
	return q
}

// excludedColumns builds the excluded column list: all columns in field order
// minus the excluded ones; falls back to "*" when the result is empty.
func excludedColumns(m *entityMeta, excluded []string) string {
	// Linear scan instead of a lookup map: the exclude list is typically one
	// to three names, so building a map costs more (an allocation) than it
	// saves.
	cols := make([]string, 0, len(m.fields))
	for _, f := range m.fields {
		skip := false
		for _, c := range excluded {
			if c == f.Column {
				skip = true
				break
			}
		}
		if !skip {
			cols = append(cols, f.Column)
		}
	}
	if len(cols) == 0 {
		return "*"
	}
	return strings.Join(cols, ", ")
}

// WhereModel uses the entity's non-null fields as conditions: nil fields are
// skipped, empty strings are skipped, the rest append as "col = ?".
func (q *Query[T]) WhereModel(model *T) *Query[T] {
	if q.checkErr() != nil {
		return q
	}
	rv := reflect.ValueOf(model)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		q.err = newError(ErrorCodeEmptyFromClause, "WhereModel needs a non-nil entity pointer")
		return q
	}
	rv = rv.Elem()
	for _, f := range q.m.fields {
		fv := rv.Field(f.Index)
		if isNilValue(fv) {
			continue
		}
		if fv.Kind() == reflect.String && fv.String() == "" {
			continue
		}
		q.addCond("AND", f.Column+" = ?", unwrap(fv))
	}
	return q
}

// Order sorts the result. Multiple calls append with commas (the first entry
// carries a leading space, matching the assembly shape). It is the raw escape
// hatch: the expression reaches the SQL verbatim, so unlike OrderCol it is the
// caller's job to keep it injection-free.
func (q *Query[T]) Order(expr string) *Query[T] {
	q.order = appendOrder(q.order, expr)
	return q
}

// OrderCol sorts by field; dir is "asc"/"desc" (empty means ascending).
// Multiple calls append with commas as well.
func (q *Query[T]) OrderCol(name, dir string) *Query[T] {
	if q.checkErr() != nil {
		return q
	}
	f, err := fieldOf(q.m, name)
	if err != nil {
		q.err = err
		return q
	}
	switch strings.ToUpper(dir) {
	case "", "ASC":
	case "DESC":
	default:
		q.err = newError(ErrorCodeInvalidArgument, "OrderCol: dir must be asc or desc, got %q", dir)
		return q
	}
	expr := f.Column
	if dir != "" {
		expr += " " + dir
	}
	q.order = appendOrder(q.order, expr)
	return q
}

// appendOrder appends an order expression with a comma separator:
// " expr" when empty, ", expr" when appended.
func appendOrder(cur, expr string) string {
	if cur != "" {
		return cur + ", " + expr
	}
	return " " + expr
}

// Limit caps the number of rows: 0 = empty result, negative is an error.
// Not calling it (unset) means unlimited rows.
func (q *Query[T]) Limit(n int) *Query[T] {
	if q.checkErr() != nil {
		return q
	}
	if n < 0 {
		q.err = newError(ErrorCodeInvalidArgument, "limit must be >= 0, got %d", n)
		return q
	}
	q.limitN = n
	q.limitSet = true
	return q
}

func (q *Query[T]) sqlParams() *dialect.SQLParams {
	cols := q.cols
	if cols == "" && len(q.excluded) > 0 {
		// Cols takes priority; the exclude list is resolved at the query
		// layer so the dialect layer stays unaware of entity models
		// (the SQLParams protocol carries pure data only).
		cols = excludedColumns(q.m, q.excluded)
	}
	return &dialect.SQLParams{
		TableName:     q.m.table,
		CustomSQL:     q.customSQL,
		SelectColumns: cols,
		ConditionSQL:  q.cond,
		OrderBy:       q.order,
		Params:        q.params,
		Dialect:       q.a.dialect,
	}
}

// BySQL switches to the raw SQL pass-through while still mapping rows onto T.
func (q *Query[T]) BySQL(rawSQL string, args ...any) *Query[T] {
	if q.checkErr() != nil {
		return q
	}
	if q.mode != modeSelect {
		q.err = newError(ErrorCodeModeMismatch, "BySQL() only allowed in select mode")
		return q
	}
	q.customSQL = rawSQL
	q.params = append([]any{}, args...)
	return q
}

// requireSelect is the read-terminal guard: a query in update/delete mode
// that reaches a read terminal would legitimately run SELECT and silently
// ignore the write intent — a misuse that should fail fast.
func (q *Query[T]) requireSelect() error {
	if q.mode != modeSelect {
		return newError(ErrorCodeModeMismatch, "read terminal only allowed in select mode")
	}
	return nil
}

// All runs the query and returns all entities.
func (q *Query[T]) All(ctx context.Context) ([]*T, error) {
	if err := q.checkErr(); err != nil {
		return nil, err
	}
	if err := q.requireSelect(); err != nil {
		return nil, err
	}
	if q.limitSet && q.limitN == 0 {
		return []*T{}, nil // limit(0): empty result
	}
	ex, err := q.a.sqlable(ctx)
	if err != nil {
		return nil, err
	}
	p := q.sqlParams()
	var args []any
	sqlLimit := q.limitN > 0 && q.a.useSQLLimit
	if sqlLimit {
		p.SQLLimit = true
		args = make([]any, 0, len(q.params)+1)
		args = append(args, q.params...)
		args = append(args, q.limitN)
	} else {
		args = q.params
	}
	sqlText := q.a.dialect.Select(p)

	rows, err := q.a.queryCtx(ctx, ex, sqlText, args)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list, err := scanEntities[T](rows, q.m)
	if err != nil {
		return nil, err
	}
	if !q.a.useSQLLimit && q.limitN > 0 && len(list) > q.limitN {
		list = list[:q.limitN]
	}
	if err := q.applyJoins(ctx, ex, list); err != nil {
		return nil, err
	}
	return list, nil
}

// Map projects every query row (Java AnimaQuery.map). Go methods cannot
// introduce a new type parameter, so — like MapPage — it is a package-level
// function. It is eager: rows are materialized through All first.
func Map[T, R any](ctx context.Context, q *Query[T], fn func(*T) R) ([]R, error) {
	rows, err := q.All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]R, len(rows))
	for i, r := range rows {
		out[i] = fn(r)
	}
	return out, nil
}

// Filter keeps the rows matching pred (Java AnimaQuery.filter). Like the Java
// stream filter it is a post-load predicate: the SQL runs in full and pred
// narrows the materialized rows in memory.
func (q *Query[T]) Filter(ctx context.Context, pred func(*T) bool) ([]*T, error) {
	rows, err := q.All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*T, 0, len(rows))
	for _, r := range rows {
		if pred(r) {
			out = append(out, r)
		}
	}
	return out, nil
}

// Seq runs the query eagerly and returns a range-over-func iterator over the
// models (Java AnimaQuery.stream): the SQL executes immediately and iteration
// only walks the already-loaded rows, so a query error surfaces from the call
// itself, not from the iterator. An empty result yields an empty iterator
// (Java Stream.empty).
func (q *Query[T]) Seq(ctx context.Context) (iter.Seq[*T], error) {
	rows, err := q.All(ctx)
	if err != nil {
		return nil, err
	}
	return slices.Values(rows), nil
}

// One returns the first row, or (nil, nil) when there is no result.
func (q *Query[T]) One(ctx context.Context) (*T, error) {
	old, oldSet := q.limitN, q.limitSet
	q.limitN, q.limitSet = 1, true
	list, err := q.All(ctx)
	q.limitN, q.limitSet = old, oldSet
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, nil
	}
	return list[0], nil
}

// ById queries by primary key.
func (q *Query[T]) ById(ctx context.Context, id any) (*T, error) {
	if err := q.checkErr(); err != nil {
		return nil, err
	}
	if err := q.m.requirePK(); err != nil {
		return nil, err
	}
	q.order = "" // a primary-key query carries no ORDER BY
	return q.WhereCol(q.m.pk).Eq(id).One(ctx)
}

// ByIds queries by primary keys in batch.
// Empty arguments make In skip the condition, degrading to a full-table query.
func (q *Query[T]) ByIds(ctx context.Context, ids ...any) ([]*T, error) {
	if err := q.checkErr(); err != nil {
		return nil, err
	}
	if err := q.m.requirePK(); err != nil {
		return nil, err
	}
	return q.WhereCol(q.m.pk).In(ids...).All(ctx)
}

// Maps returns results as maps. Keys are column names and values are the
// driver-decoded scalars; rows are not mapped to entities, so joins do not
// trigger row back-filling.
func (q *Query[T]) Maps(ctx context.Context) ([]map[string]any, error) {
	if err := q.checkErr(); err != nil {
		return nil, err
	}
	if err := q.requireSelect(); err != nil {
		return nil, err
	}
	ex, err := q.a.sqlable(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.a.queryCtx(ctx, ex, q.a.dialect.Select(q.sqlParams()), q.params)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	// vals/ptrs are allocated once and reused across rows: the map built
	// below copies each value out before the next Scan overwrites the box,
	// which satisfies the database/sql contract.
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	out := make([]map[string]any, 0, 16)
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make(map[string]any, len(cols))
		for i, c := range cols {
			row[c] = vals[i]
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// Count counts matching rows.
func (q *Query[T]) Count(ctx context.Context) (int, error) {
	if err := q.checkErr(); err != nil {
		return 0, err
	}
	if err := q.requireSelect(); err != nil {
		return 0, err
	}
	ex, err := q.a.sqlable(ctx)
	if err != nil {
		return 0, err
	}
	p := q.sqlParams()
	p.OrderBy = ""
	rows, err := q.a.queryCtx(ctx, ex, q.a.dialect.Count(p), p.Params)
	if err != nil {
		return 0, err
	}
	n, err := scanCount(rows)
	return int(n), err
}

// scanCount scans a single-row count (shared by Count and Page's count stage).
func scanCount(rows *sql.Rows) (int64, error) {
	defer rows.Close()
	var n int64
	if rows.Next() {
		if err := rows.Scan(&n); err != nil {
			return 0, err
		}
	}
	return n, rows.Err()
}
