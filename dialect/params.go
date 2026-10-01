package dialect

// PageRow is the paginate input.
type PageRow struct {
	PageNum  int
	PageSize int
}

// ColumnValue is an ordered column/value pair.
type ColumnValue struct {
	Name  string
	Value any
}

// SQLParams is the only parameter protocol between the query builder layer
// and the SQL generation layer.
// It carries pure data: the dialect layer does no reflection and knows
// nothing about entity models.
type SQLParams struct {
	TableName     string
	CustomSQL     string // raw SQL pass-through; when non-empty, select uses it directly
	SelectColumns string // explicit column list (already resolved to column names); empty means *
	ConditionSQL  string // WHERE body with the first connector stripped; may be empty
	OrderBy       string // order text with leading space, e.g. " id desc" from " ORDER BY x"; may be empty
	Params        []any  // parameters matching the ? placeholders in the SQL one-to-one

	Columns    []ColumnValue // insert columns / delete-by-model condition columns (nulls filtered out)
	SetColumns []ColumnValue // update SET ordered pairs

	Page PageRow // paginate input

	SQLLimit bool // when true, select keeps a trailing " LIMIT ?" placeholder with the limit value at the end of Params

	Dialect Dialect // used by the SQL generation layer to wrap identifiers via Quote(); falls back to Base (backticks) when nil
}
