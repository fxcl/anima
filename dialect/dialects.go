package dialect

import (
	"regexp"
	"strconv"
	"strings"
)

// MySQL is the default dialect.
type MySQL struct{ Base }

func (MySQL) Paginate(p *SQLParams) string {
	limit := p.Page.PageSize
	offset := limit * (p.Page.PageNum - 1)
	return selectSQL(p) + " LIMIT " + strconv.Itoa(offset) + "," + strconv.Itoa(limit)
}

// Quote wraps identifiers in backticks (`users`), MySQL's native quoting
// character. Embedded backticks follow the SQL standard doubling escape:
// `a“b` denotes the identifier a`b.
func (MySQL) Quote(s string) string { return "`" + escapeBacktick(s) + "`" }

// escapeBacktick doubles every ` in s to “ — the identifier escaping rule
// for MySQL/SQLite.
func escapeBacktick(s string) string {
	if !strings.ContainsRune(s, '`') {
		return s
	}
	return strings.ReplaceAll(s, "`", "``")
}

// SQLite shares MySQL's paginate syntax.
type SQLite struct{ MySQL }

// PostgreSQL dialect.
type PostgreSQL struct{ Base }

func (PostgreSQL) Paginate(p *SQLParams) string {
	limit := p.Page.PageSize
	offset := limit * (p.Page.PageNum - 1)
	return selectSQL(p) + " LIMIT " + strconv.Itoa(limit) + " OFFSET " + strconv.Itoa(offset)
}

// Quote wraps identifiers in double quotes ("users"), PostgreSQL's native
// quoting character. Embedded double quotes follow the SQL standard doubling
// escape: "a""b" denotes the identifier a"b.
func (PostgreSQL) Quote(s string) string { return `"` + escapeDoubleQuote(s) + `"` }

// escapeDoubleQuote doubles every " in s to "" — shared by
// PG/Oracle/DB2/SqlServer/SqlServer2012.
func escapeDoubleQuote(s string) string {
	if !strings.ContainsRune(s, '"') {
		return s
	}
	return strings.ReplaceAll(s, `"`, `""`)
}

// Oracle dialect (three-level rownum nesting).
type Oracle struct{ Base }

func (Oracle) Paginate(p *SQLParams) string {
	limit := p.Page.PageSize
	pageNum := p.Page.PageNum
	start := (pageNum-1)*limit + 1
	end := pageNum * limit
	return "SELECT * FROM ( SELECT row_.*, rownum rownum_ FROM (  " +
		selectSQL(p) +
		" ) row_ where rownum <= " + strconv.Itoa(end) + ") table_alias" +
		" WHERE table_alias.rownum_ >= " + strconv.Itoa(start)
}

// Quote wraps identifiers in double quotes ("users") with embedded "
// doubled, same as Oracle.
func (Oracle) Quote(s string) string { return `"` + escapeDoubleQuote(s) + `"` }

// DB2 reuses Oracle's paginate.
type DB2 struct{ Oracle }

// Quote wraps identifiers in double quotes ("users"), consistent with
// Oracle, with embedded " doubled.
func (DB2) Quote(s string) string { return `"` + escapeDoubleQuote(s) + `"` }

var (
	sqlServerSelectRe = regexp.MustCompile(`(?i)select(\s+distinct\s+)?`)
	orderByRe         = regexp.MustCompile(`(?is).* ORDER BY[^)]+$`)
)

// replaceFirst replaces the first match only.
func replaceFirst(re *regexp.Regexp, src, repl string) string {
	loc := re.FindStringIndex(src)
	if loc == nil {
		return src
	}
	expanded := re.ExpandString(nil, repl, src, loc)
	return src[:loc[0]] + string(expanded) + src[loc[1]:]
}

// SqlServer dialect (TOP + row_number).
type SqlServer struct{ Base }

func (SqlServer) Paginate(p *SQLParams) string {
	limit := p.Page.PageSize
	pageNum := p.Page.PageNum
	end := pageNum * limit
	if end <= 0 {
		end = limit
	}
	begin := (pageNum-1)*limit + 1
	if begin < 1 {
		begin = 1
	}
	topSQL := replaceFirst(sqlServerSelectRe, selectSQL(p), "${0} top("+strconv.Itoa(end)+")")
	return "with query as ( select inner_query.*, row_number() over (order by current_timestamp) as temprownumber from ( " +
		topSQL +
		" ) inner_query ) select * from query where temprownumber between " +
		strconv.Itoa(begin) + " and " + strconv.Itoa(end)
}

// Quote wraps identifiers in double quotes ("users"), T-SQL's native quoting
// character, with embedded " doubled.
func (SqlServer) Quote(s string) string { return `"` + escapeDoubleQuote(s) + `"` }

// SqlServer2012 dialect (OFFSET..FETCH, appending CURRENT_TIMESTAMP when
// there is no ORDER BY).
type SqlServer2012 struct{ Base }

func (SqlServer2012) Paginate(p *SQLParams) string {
	limit := p.Page.PageSize
	offset := limit * (p.Page.PageNum - 1)
	sql := selectSQL(p)
	if !orderByRe.MatchString(sql) {
		sql += " ORDER BY CURRENT_TIMESTAMP"
	}
	return sql + " OFFSET " + strconv.Itoa(offset) + " ROWS FETCH NEXT " + strconv.Itoa(limit) + " ROWS ONLY "
}

// Quote wraps identifiers in double quotes ("users"), with embedded "
// doubled.
func (SqlServer2012) Quote(s string) string { return `"` + escapeDoubleQuote(s) + `"` }
