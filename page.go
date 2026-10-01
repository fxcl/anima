package anima

import (
	"context"

	"github.com/fxcl/anima/dialect"
)

// Page is the paginated result, including navigation page numbers.
type Page[T any] struct {
	PageNum     int
	Limit       int
	PrevPage    int
	NextPage    int
	TotalPages  int
	TotalRows   int64
	Rows        []*T
	IsFirstPage bool
	IsLastPage  bool
	HasPrevPage bool
	HasNextPage bool
	NavPages    int
	NavPageNums []int
}

// NewPage builds pagination metadata from the total row count, page number,
// and page size.
func NewPage[T any](total int64, pageNum, limit int) *Page[T] {
	p := &Page[T]{
		PageNum:  1,
		Limit:    10,
		PrevPage: 1,
		NextPage: 1,
		NavPages: 8,
	}
	if limit > 0 {
		p.Limit = limit
	}
	p.TotalRows = total
	// Integer-division truncation semantics: total=0 yields totalPages=1
	p.TotalPages = int((total-1)/int64(p.Limit) + 1)

	// Auto-correct an out-of-range page number
	switch {
	case pageNum < 1:
		p.PageNum = 1
	case pageNum > p.TotalPages:
		p.PageNum = p.TotalPages
	default:
		p.PageNum = pageNum
	}
	p.calcNavigatePageNumbers(p.NavPages)
	p.judgePageBoundary()
	return p
}

func (p *Page[T]) calcNavigatePageNumbers(navPages int) {
	if p.TotalPages <= navPages {
		p.NavPageNums = make([]int, p.TotalPages)
		for i := 0; i < p.TotalPages; i++ {
			p.NavPageNums[i] = i + 1
		}
		return
	}
	p.NavPageNums = make([]int, navPages)
	startNum := p.PageNum - navPages/2
	endNum := p.PageNum + navPages/2
	switch {
	case startNum < 1:
		startNum = 1
		for i := 0; i < navPages; i++ {
			p.NavPageNums[i] = startNum
			startNum++
		}
	case endNum > p.TotalPages:
		endNum = p.TotalPages
		for i := navPages - 1; i >= 0; i-- {
			p.NavPageNums[i] = endNum
			endNum--
		}
	default:
		for i := 0; i < navPages; i++ {
			p.NavPageNums[i] = startNum
			startNum++
		}
	}
}

func (p *Page[T]) judgePageBoundary() {
	p.IsFirstPage = p.PageNum == 1
	p.IsLastPage = p.PageNum == p.TotalPages && p.PageNum != 1
	p.HasPrevPage = p.PageNum != 1
	p.HasNextPage = p.PageNum != p.TotalPages
	if p.HasNextPage {
		p.NextPage = p.PageNum + 1
	}
	if p.HasPrevPage {
		p.PrevPage = p.PageNum - 1
	}
}

// MapPage converts the page payload. Go methods cannot introduce new type
// parameters, so this is a package-level function.
func MapPage[T, R any](p *Page[T], mapper func(*T) R) *Page[R] {
	out := NewPage[R](p.TotalRows, p.PageNum, p.Limit)
	if p.Rows != nil {
		// One backing store for the values plus one pointer slice keeps the
		// allocation count flat instead of boxing every mapped row.
		vals := make([]R, len(p.Rows))
		rows := make([]*R, len(p.Rows))
		for i, r := range p.Rows {
			vals[i] = mapper(r)
			rows[i] = &vals[i]
		}
		out.Rows = rows
	}
	return out
}

// RecalcNavPages recomputes the navigation page numbers with the given
// window size.
// The Go field NavPages already occupies that name, hence the method rename.
func (p *Page[T]) RecalcNavPages(n int) *Page[T] {
	p.calcNavigatePageNumbers(n)
	return p
}

// Peek applies a side effect to every row and returns the receiver.
func (p *Page[T]) Peek(f func(*T)) *Page[T] {
	if p.Rows != nil {
		for _, r := range p.Rows {
			f(r)
		}
	}
	return p
}

// Page runs a paginated query: count via string surgery (custom SQL gets
// wrapped in tmp), data rows via the dialect paginate.
func (q *Query[T]) Page(ctx context.Context, pageNum, pageSize int) (*Page[T], error) {
	if err := q.checkErr(); err != nil {
		return nil, err
	}
	if err := q.requireSelect(); err != nil {
		return nil, err
	}
	if pageNum < 1 || pageSize < 1 {
		return nil, newError(ErrorCodeInvalidArgument, "pageNum and pageSize must be >= 1, got %d, %d", pageNum, pageSize)
	}
	ex, err := q.a.sqlable(ctx)
	if err != nil {
		return nil, err
	}
	p := q.sqlParams()

	// Count the rows that the data query would return. We hand the dialect the
	// full SQLParams (with ORDER BY stripped, since PostgreSQL rejects ORDER BY
	// inside an aggregate); Count already wraps a CustomSQL in "SELECT COUNT(*)
	// FROM (...) tmp" and otherwise emits "SELECT COUNT(*) FROM ..." with the
	// table quoted, so no string surgery is needed here.
	countParams := *p
	countParams.OrderBy = ""
	countText := q.a.dialect.Count(&countParams)

	countArgs := append([]any{}, p.Params...)
	countRows, err := q.a.queryCtx(ctx, ex, countText, countArgs)
	if err != nil {
		return nil, err
	}
	total, err := scanCount(countRows)
	if err != nil {
		return nil, err
	}

	page := NewPage[T](total, pageNum, pageSize)
	if total == 0 {
		return page, nil
	}
	// The data query must fetch the CLAMPED page: NewPage auto-corrects an
	// out-of-range page number to the last page, and reporting that page
	// number while fetching the original offset would return empty rows for
	// a page the metadata claims is full.
	p.Page = dialect.PageRow{PageNum: page.PageNum, PageSize: pageSize}
	pageText := q.a.dialect.Paginate(p)
	dataRows, err := q.a.queryCtx(ctx, ex, pageText, p.Params)
	if err != nil {
		return nil, err
	}
	defer dataRows.Close()
	list, err := scanEntities[T](dataRows, q.m)
	if err != nil {
		return nil, err
	}
	if err := q.applyJoins(ctx, ex, list); err != nil {
		return nil, err
	}
	page.Rows = list
	return page, nil
}
