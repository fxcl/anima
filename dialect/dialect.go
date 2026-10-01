package dialect

import "strings"

// Dialect is a database SQL dialect. Base provides the full CRUD SQL
// generation; a concrete dialect only overrides Paginate.
// Quote wraps identifiers (table names, column names, aliases) in SQL
// generation, using each dialect's native characters.
type Dialect interface {
	Select(p *SQLParams) string
	Count(p *SQLParams) string
	Insert(p *SQLParams) string
	Update(p *SQLParams) string
	Delete(p *SQLParams) string
	Paginate(p *SQLParams) string
	Quote(s string) string
}

// Base is the generic SQL generator.
type Base struct{}

func (Base) Select(p *SQLParams) string { return selectSQL(p) }
func (Base) Count(p *SQLParams) string  { return countSQL(p) }
func (Base) Insert(p *SQLParams) string { return insertSQL(p) }
func (Base) Update(p *SQLParams) string { return updateSQL(p) }
func (Base) Delete(p *SQLParams) string { return deleteSQL(p) }

// Paginate does not paginate on Base — a concrete dialect overrides this
// method. Calling Paginate on Base directly is a programming error; returning
// selectSQL instead of panicking only serves to make Base satisfy the Dialect
// interface for nil fallbacks.
func (Base) Paginate(p *SQLParams) string { return selectSQL(p) }

// Quote wraps identifiers in backticks by default (same as MySQL), with
// dialects overriding as needed.
// Embedded backticks are doubled, so untrusted identifier content cannot
// break the SQL structure.
func (Base) Quote(s string) string { return "`" + escapeBacktick(s) + "`" }

func selectSQL(p *SQLParams) string {
	var sb strings.Builder
	d := p.Dialect // injected by the caller; falls back to Base (default quote char) when nil
	if d == nil {
		d = Base{}
	}
	// One Grow up front: the statement is assembled from 3-5 fragments and
	// the builder would otherwise regrow (and reallocate) between each.
	sb.Grow(48 + len(p.SelectColumns) + len(p.TableName) + len(p.ConditionSQL) + len(p.OrderBy))
	if p.CustomSQL != "" {
		sb.WriteString(p.CustomSQL)
	} else {
		sb.WriteString("SELECT")
		if p.SelectColumns != "" {
			// SelectColumns is a SQL fragment (a column list, an expression
			// with DISTINCT, etc.) that the dialect layer does not parse;
			// quirks like SqlServer paginate's "SELECT distinct top(n)user_name"
			// rely on verbatim output, so it is not quoted.
			sb.WriteString(" " + p.SelectColumns + " ")
		} else {
			sb.WriteString(" * ")
		}
		sb.WriteString("FROM " + d.Quote(p.TableName))
		if p.ConditionSQL != "" {
			sb.WriteString(" WHERE " + p.ConditionSQL)
		}
	}
	if p.OrderBy != "" {
		sb.WriteString(" ORDER BY" + p.OrderBy)
	}
	if p.SQLLimit {
		sb.WriteString(" LIMIT ?")
	}
	return sb.String()
}

func countSQL(p *SQLParams) string {
	d := p.Dialect
	if d == nil {
		d = Base{}
	}
	if p.CustomSQL != "" {
		return "SELECT COUNT(*) FROM (" + p.CustomSQL + ") tmp"
	}
	sql := "SELECT COUNT(*) FROM " + d.Quote(p.TableName)
	if p.ConditionSQL != "" {
		sql += " WHERE " + p.ConditionSQL
	}
	return sql
}

func insertSQL(p *SQLParams) string {
	d := p.Dialect
	if d == nil {
		d = Base{}
	}
	// Written straight into a pre-sized builder: the previous form built two
	// []string slices and ran strings.Join twice, which cost five extra
	// allocations per statement.
	var sb strings.Builder
	sb.Grow(48 + len(p.TableName) + 12*len(p.Columns))
	sb.WriteString("INSERT INTO ")
	sb.WriteString(d.Quote(p.TableName))
	if len(p.Columns) > 0 {
		sb.WriteString(" (")
		for i, c := range p.Columns {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(d.Quote(c.Name))
		}
		sb.WriteString(") VALUES (")
		for i := range p.Columns {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteByte('?')
		}
		sb.WriteByte(')')
	}
	return sb.String()
}

func updateSQL(p *SQLParams) string {
	d := p.Dialect
	if d == nil {
		d = Base{}
	}
	// Pre-sized builder instead of a []string plus strings.Join: the join
	// form allocated the slice, one string per "col = ?" fragment, and the
	// joined result.
	var sb strings.Builder
	sb.Grow(32 + len(p.TableName) + 12*len(p.SetColumns) + len(p.ConditionSQL))
	sb.WriteString("UPDATE ")
	sb.WriteString(d.Quote(p.TableName))
	sb.WriteString(" SET ")
	for i, c := range p.SetColumns {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(d.Quote(c.Name))
		sb.WriteString(" = ?")
	}
	if p.ConditionSQL != "" {
		sb.WriteString(" WHERE ")
		sb.WriteString(p.ConditionSQL)
	}
	return sb.String()
}

func deleteSQL(p *SQLParams) string {
	d := p.Dialect
	if d == nil {
		d = Base{}
	}
	var sb strings.Builder
	sb.Grow(32 + len(p.TableName) + 12*len(p.Columns) + len(p.ConditionSQL))
	sb.WriteString("DELETE FROM ")
	sb.WriteString(d.Quote(p.TableName))
	if p.ConditionSQL != "" {
		sb.WriteString(" WHERE ")
		sb.WriteString(p.ConditionSQL)
		return sb.String()
	}
	// With no condition but entity columns given, build the WHERE from the
	// entity's non-null columns.
	if len(p.Columns) > 0 {
		sb.WriteString(" WHERE ")
		for i, c := range p.Columns {
			if i > 0 {
				sb.WriteString(" and ")
			}
			sb.WriteString(d.Quote(c.Name))
			sb.WriteString(" = ?")
		}
	}
	return sb.String()
}
