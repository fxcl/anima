package anima

import (
	"regexp"

	"context"
	"reflect"
	"strings"
)

// JoinSpec declares one association load. Associations do not produce JOIN
// SQL; they are back-filled row by row after the main query.
type JoinSpec struct {
	joinType reflect.Type // associated entity struct type
	as       string       // mount field (Go field name on T)
	onLeft   string       // T-side field/column reference
	onRight  string       // J-side field/column reference
	orderBy  string
	err      error
}

// JoinWith declares one association load.
func JoinWith[J any]() *JoinSpec {
	s := &JoinSpec{}
	var zero J
	t := reflect.TypeOf(zero)
	if t == nil {
		t = reflect.TypeOf((*J)(nil)).Elem()
	}
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		s.err = newError(ErrorCodeEmptyFromClause, "JoinWith: %s is not a struct entity", t)
		return s
	}
	s.joinType = t
	return s
}

// As names the field on the main entity that receives the loaded
// association.
func (s *JoinSpec) As(field string) *JoinSpec { s.as = field; return s }

// On names the join columns: left refers to the main entity, right to the
// associated entity.
func (s *JoinSpec) On(left, right string) *JoinSpec {
	s.onLeft, s.onRight = left, right
	return s
}

// Order sorts the associated list; only affects []*J mounts.
// orderExprPattern pins Order to "identifier [ASC|DESC]": every legal column
// name passes, while parentheses, commas, semicolons and comments — the
// building blocks of ORDER BY injection — all fail the match.
var orderExprPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\s+(?i:asc|desc))?$`)

// Order sets the sort applied to the joined query. The expression must be a
// bare column identifier with an optional asc/desc direction; anything else
// poisons the spec so Join fails fast instead of assembling unsafe SQL.
func (s *JoinSpec) Order(expr string) *JoinSpec {
	if !orderExprPattern.MatchString(strings.TrimSpace(expr)) {
		s.err = newError(ErrorCodeInvalidArgument, "JoinSpec.Order: must be a column with optional asc|desc, got %q", expr)
		return s
	}
	s.orderBy = expr
	return s
}

// Join registers association loads on the query.
func (q *Query[T]) Join(specs ...*JoinSpec) *Query[T] {
	if q.checkErr() != nil {
		return q
	}
	for _, s := range specs {
		if s.err != nil {
			q.err = s.err
			return q
		}
		q.joins = append(q.joins, s)
	}
	return q
}

// applyJoins back-fills associations row by row after the main query.
func (q *Query[T]) applyJoins(ctx context.Context, ex execer, models []*T) error {
	if len(q.joins) == 0 {
		return nil
	}
	for _, m := range models {
		rv := reflect.ValueOf(m).Elem()
		for _, s := range q.joins {
			if err := q.setJoinOne(ctx, ex, rv, s); err != nil {
				return err
			}
		}
	}
	return nil
}

// setJoinOne loads one association for a single parent row. Four sequential
// stages, each owning its failure mode:
//
//  1. resolve the left key field and the mount field on the parent entity
//  2. resolve the joined entity's metadata and its right key column
//  3. build "SELECT * FROM <joined> WHERE <right> = ?" (ORDER BY appended for
//     hasMany), validating that the mount field's shape matches the spec
//  4. run the query and back-fill the mount field (slice or pointer)
func (q *Query[T]) setJoinOne(ctx context.Context, ex execer, rv reflect.Value, s *JoinSpec) error {
	leftF, ok := q.m.byRef[s.onLeft]
	if !ok {
		return newError(ErrorCodeUnknownField, "join on-left %q not found on %s", s.onLeft, q.m.typ.Name())
	}
	// The mount field is usually one tagged anima:"-".
	asIdx, ok := q.m.ignored[s.as]
	if !ok {
		if f, ok2 := q.m.byRef[s.as]; ok2 {
			asIdx = f.Index
		} else {
			return newError(ErrorCodeUnknownField, "join as-field %q not found on %s", s.as, q.m.typ.Name())
		}
	}
	jm := q.a.metaOf(s.joinType)
	rightF, ok := jm.byRef[s.onRight]
	if !ok {
		return newError(ErrorCodeUnknownField, "join on-right %q not found on %s", s.onRight, s.joinType.Name())
	}

	leftVal := unwrap(rv.Field(leftF.Index))
	// Built with a pre-sized builder: the previous four-way concatenation
	// allocated once per intermediate result, and this runs for every model
	// in the parent result set.
	var sb strings.Builder
	sb.Grow(32 + len(jm.table) + len(rightF.Column))
	sb.WriteString("SELECT * FROM ")
	sb.WriteString(jm.table)
	sb.WriteString(" WHERE ")
	sb.WriteString(rightF.Column)
	sb.WriteString(" = ?")
	sqlText := sb.String()

	field := rv.Field(asIdx)

	switch field.Kind() {
	case reflect.Slice: // hasMany ([]*J)
		if field.Type().Elem() != reflect.PtrTo(s.joinType) && field.Type().Elem() != s.joinType {
			return newError(ErrorCodeMappingFailure, "join as-field %s: want []%s, got %s", s.as, s.joinType.Name(), field.Type())
		}
		if s.orderBy != "" {
			sqlText += " ORDER BY " + s.orderBy
		}
	case reflect.Ptr: // hasOne / belongsTo
		if field.Type().Elem() != s.joinType {
			return newError(ErrorCodeMappingFailure, "join as-field %s: want *%s, got %s", s.as, s.joinType.Name(), field.Type())
		}
	default:
		return newError(ErrorCodeMappingFailure, "join as-field %s must be *%s or []%s", s.as, s.joinType.Name(), s.joinType.Name())
	}
	rows, err := q.a.queryCtx(ctx, ex, sqlText, []any{leftVal})
	if err != nil {
		return err
	}
	defer rows.Close()
	vals, err := scanReflect(rows, jm)
	if err != nil {
		return err
	}
	if field.Kind() == reflect.Slice {
		out := reflect.MakeSlice(field.Type(), len(vals), len(vals))
		for i, v := range vals {
			if field.Type().Elem() == s.joinType {
				v = v.Elem()
			}
			out.Index(i).Set(v)
		}
		field.Set(out)
		return nil
	}
	if len(vals) > 0 {
		field.Set(vals[0])
	} else {
		field.Set(reflect.Zero(field.Type()))
	}
	return nil
}
