package dialect

import (
	"strings"
	"testing"
)

// All expected values are derived verbatim from Java anima dialect/Dialect.java and each dialect's
// paginate() (identifiers wrapped by this dialect's Quote(); MySQL/SQLite use backticks, the rest use double quotes).

func params() *SQLParams {
	return &SQLParams{
		TableName:    "users",
		ConditionSQL: "age > ?",
		OrderBy:      " id desc",
		Params:       []any{18},
		Page:         PageRow{PageNum: 2, PageSize: 10},
		Dialect:      MySQL{},
	}
}

func TestSelectSQL(t *testing.T) {
	cases := []struct {
		name string
		p    *SQLParams
		want string
	}{
		{"basic", params(), "SELECT * FROM `users` WHERE age > ? ORDER BY id desc"},
		{"cols", &SQLParams{TableName: "users", SelectColumns: "user_name, age", Dialect: MySQL{}}, "SELECT user_name, age FROM `users`"},
		{"limit", func() *SQLParams {
			p := params()
			p.SQLLimit = true
			return p
		}(), "SELECT * FROM `users` WHERE age > ? ORDER BY id desc LIMIT ?"},
		{"custom", &SQLParams{TableName: "users", CustomSQL: "select * from users limit 1", OrderBy: " id", Dialect: MySQL{}}, "select * from users limit 1 ORDER BY id"},
		{"count", params(), "SELECT COUNT(*) FROM `users` WHERE age > ?"},
	}
	for _, c := range cases {
		var got string
		if c.name == "count" {
			got = Base{}.Count(c.p)
		} else {
			got = Base{}.Select(c.p)
		}
		if got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
	if got := (Base{}).Count((&SQLParams{CustomSQL: "select a from t", Dialect: MySQL{}})); got != "SELECT COUNT(*) FROM (select a from t) tmp" {
		t.Errorf("count custom: %q", got)
	}
}

func TestInsertUpdateDeleteSQL(t *testing.T) {
	d := MySQL{}
	ins := &SQLParams{TableName: "users", Columns: []ColumnValue{{"user_name", "b"}, {"age", 3}}, Dialect: d}
	if got := (Base{}).Insert(ins); got != "INSERT INTO `users` (`user_name`, `age`) VALUES (?, ?)" {
		t.Errorf("insert: %q", got)
	}
	if got := (Base{}).Insert((&SQLParams{TableName: "users", Dialect: d})); got != "INSERT INTO `users`" {
		t.Errorf("insert empty: %q", got)
	}
	upd := &SQLParams{TableName: "users", SetColumns: []ColumnValue{{"user_name", "x"}, {"age", 1}}, ConditionSQL: "id = ?", Dialect: d}
	if got := (Base{}).Update(upd); got != "UPDATE `users` SET `user_name` = ?, `age` = ? WHERE id = ?" {
		t.Errorf("update: %q", got)
	}
	del := &SQLParams{TableName: "users", Columns: []ColumnValue{{"id", 7}, {"age", 1}}, Dialect: d}
	if got := (Base{}).Delete(del); got != "DELETE FROM `users` WHERE `id` = ? and `age` = ?" {
		t.Errorf("delete by model: %q", got)
	}
	if got := (Base{}).Delete((&SQLParams{TableName: "users", ConditionSQL: "age < ?", Dialect: d})); got != "DELETE FROM `users` WHERE age < ?" {
		t.Errorf("delete by cond: %q", got)
	}
}

func TestPaginateAllDialects(t *testing.T) {
	cases := []struct {
		name string
		d    Dialect
		want string
	}{
		{"mysql", MySQL{}, "SELECT * FROM `users` WHERE age > ? ORDER BY id desc LIMIT 10,10"},
		{"sqlite", SQLite{}, "SELECT * FROM `users` WHERE age > ? ORDER BY id desc LIMIT 10,10"},
		{"postgres", PostgreSQL{}, "SELECT * FROM \"users\" WHERE age > ? ORDER BY id desc LIMIT 10 OFFSET 10"},
		{"oracle", Oracle{}, "SELECT * FROM ( SELECT row_.*, rownum rownum_ FROM (  SELECT * FROM \"users\" WHERE age > ? ORDER BY id desc ) row_ where rownum <= 20) table_alias WHERE table_alias.rownum_ >= 11"},
		{"db2", DB2{}, "SELECT * FROM ( SELECT row_.*, rownum rownum_ FROM (  SELECT * FROM \"users\" WHERE age > ? ORDER BY id desc ) row_ where rownum <= 20) table_alias WHERE table_alias.rownum_ >= 11"},
		{"sqlserver", SqlServer{}, "with query as ( select inner_query.*, row_number() over (order by current_timestamp) as temprownumber from ( SELECT top(20) * FROM \"users\" WHERE age > ? ORDER BY id desc ) inner_query ) select * from query where temprownumber between 11 and 20"},
		{"sqlserver2012", SqlServer2012{}, "SELECT * FROM \"users\" WHERE age > ? ORDER BY id desc OFFSET 10 ROWS FETCH NEXT 10 ROWS ONLY "},
	}
	for _, c := range cases {
		p := params()
		p.Dialect = c.d
		if got := c.d.Paginate(p); got != c.want {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, c.want)
		}
	}
}

func TestSqlServer2012AddsOrderBy(t *testing.T) {
	p := params()
	p.OrderBy = ""
	p.Dialect = SqlServer2012{}
	got := SqlServer2012{}.Paginate(p)
	want := "SELECT * FROM \"users\" WHERE age > ? ORDER BY CURRENT_TIMESTAMP OFFSET 10 ROWS FETCH NEXT 10 ROWS ONLY "
	if got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestSqlServerDistinctTop(t *testing.T) {
	p := params()
	p.SelectColumns = "distinct user_name"
	p.Dialect = SqlServer{}
	got := SqlServer{}.Paginate(p)
	// select(\s+distinct\s+)? matches the whole "SELECT distinct " as a unit, and " top(20)" is appended directly after it (Java has the same double-space quirk);
	// SqlServer wraps table names in double quotes ("users"), and SelectColumns is a SQL fragment so it is not quoted
	if want := "with query as ( select inner_query.*, row_number() over (order by current_timestamp) as temprownumber from ( SELECT distinct  top(20)user_name FROM \"users\" WHERE age > ? ORDER BY id desc ) inner_query ) select * from query where temprownumber between 11 and 20"; got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

// TestQuotePerDialect verifies each dialect wraps identifiers with its own dialect's identifier quoting character.
// This is an alignment extension (Java does not quote by default either, while the Go side's dial.Quote() provides safe column-name support).
func TestQuotePerDialect(t *testing.T) {
	cases := []struct {
		name string
		d    Dialect
		want string
	}{
		{"mysql", MySQL{}, "`users`"},
		{"sqlite", SQLite{}, "`users`"},
		{"postgres", PostgreSQL{}, `"users"`},
		{"oracle", Oracle{}, `"users"`},
		{"db2", DB2{}, `"users"`},
		{"sqlserver", SqlServer{}, `"users"`},
		{"sqlserver2012", SqlServer2012{}, `"users"`},
	}
	for _, c := range cases {
		if got := c.d.Quote("users"); got != c.want {
			t.Errorf("%s.Quote: got %q want %q", c.name, got, c.want)
		}
	}
}

// TestQuoteEscapesEmbeddedSpecial verifies Quote doubles the escape of embedded quote characters,
// so that uncontrolled identifier content (from code annotations, external config) cannot break the SQL structure.
func TestQuoteEscapesEmbeddedSpecial(t *testing.T) {
	cases := []struct {
		name string
		d    Dialect
		in   string
		want string
	}{
		{"mysql-backtick", MySQL{}, "we`ird", "`we``ird`"},
		{"sqlite-backtick", SQLite{}, "a`b`c", "`a``b``c`"},
		{"postgres-dquote", PostgreSQL{}, `we"ird`, `"we""ird"`},
		{"oracle-dquote", Oracle{}, `a"b`, `"a""b"`},
		{"db2-dquote", DB2{}, `a"b"c`, `"a""b""c"`},
		{"sqlserver-dquote", SqlServer{}, `we"ird`, `"we""ird"`},
		{"sqlserver2012-dquote", SqlServer2012{}, `we"ird`, `"we""ird"`},
	}
	for _, c := range cases {
		if got := c.d.Quote(c.in); got != c.want {
			t.Errorf("%s: Quote(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

// TestQuoteEmptyAndPassThrough verifies Quote's behavior on ordinary/empty/identifier-without-special-characters.
func TestQuoteEmptyAndPassThrough(t *testing.T) {
	// Empty string: the dialect layer does not defensively reject it, leaving the error to driver/DB; this only records the current behavior.
	if got := (MySQL{}).Quote(""); got != "`"+"`" {
		t.Errorf("MySQL empty: %q", got)
	}
	if got := (PostgreSQL{}).Quote(""); got != `""` {
		t.Errorf("PG empty: %q", got)
	}
	// Ordinary identifiers and reserved words are still wrapped verbatim (interpreted by the DB).
	for _, w := range []string{"select", "from", "user", "weird-name"} {
		if got := (MySQL{}).Quote(w); got != "`"+w+"`" {
			t.Errorf("MySQL(%q): %q", w, got)
		}
	}
}

// TestBaseQuoteFallback when SQLParams.Dialect is nil, the Base SQL generator uses the Base default Quote (backticks).
func TestBaseQuoteFallback(t *testing.T) {
	p := &SQLParams{TableName: "users"} // Dialect intentionally left unset
	got := (Base{}).Select(p)
	if !strings.Contains(got, "FROM `users`") {
		t.Errorf("Base fallback should use backtick: %q", got)
	}
}

// TestBasePaginateFallback: Base.Paginate is a documented programming-error
// fallback that must behave exactly like Base.Select (no pagination clause),
// so Base stays a valid Dialect for nil fallbacks.
func TestBasePaginateFallback(t *testing.T) {
	p := &SQLParams{TableName: "users"}
	got := (Base{}).Paginate(p)
	want := (Base{}).Select(p)
	if got != want {
		t.Fatalf("Base.Paginate must fall back to selectSQL, got %q want %q", got, want)
	}
	if strings.Contains(got, "LIMIT") {
		t.Fatalf("Base.Paginate must not emit a LIMIT clause, got %q", got)
	}
}
