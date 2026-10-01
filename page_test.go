package anima

import (
	"context"
	"reflect"
	"testing"
)

// NewPage calculation rules ported from Java Page.init/calcNavigatePageNumbers/judgePageBoudary

func TestPageCalc(t *testing.T) {
	// total=95, limit=10 -> totalPages=10
	p := NewPage[User](95, 1, 10)
	if p.TotalPages != 10 || p.PageNum != 1 {
		t.Fatalf("%+v", p)
	}
	if !p.IsFirstPage || !p.HasNextPage || p.NextPage != 2 {
		t.Fatalf("boundary flags: %+v", p)
	}
	if want := []int{1, 2, 3, 4, 5, 6, 7, 8}; !reflect.DeepEqual(p.NavPageNums, want) {
		t.Fatalf("nav = %v, want %v", p.NavPageNums, want)
	}

	// Last page: last flag + nav right-aligned
	p = NewPage[User](95, 20, 10) // out-of-range pageNum auto-corrects to 10
	if p.PageNum != 10 || !p.IsLastPage || p.HasNextPage {
		t.Fatalf("%+v", p)
	}
	if want := []int{3, 4, 5, 6, 7, 8, 9, 10}; !reflect.DeepEqual(p.NavPageNums, want) {
		t.Fatalf("nav = %v", p.NavPageNums)
	}

	// totalPages <= nav page count: list all
	p = NewPage[User](25, 2, 10)
	if p.TotalPages != 3 {
		t.Fatalf("%+v", p)
	}
	if want := []int{1, 2, 3}; !reflect.DeepEqual(p.NavPageNums, want) {
		t.Fatalf("nav = %v", p.NavPageNums)
	}

	// Java long-division truncation semantics: total=0 -> totalPages=1, nav=[1]
	p = NewPage[User](0, 1, 10)
	if p.TotalPages != 1 || !reflect.DeepEqual(p.NavPageNums, []int{1}) || !p.IsFirstPage {
		t.Fatalf("empty page: %+v", p)
	}
}

func TestQueryPageIntegration(t *testing.T) {
	ctx := context.Background()
	seedUsers(t) // ann(30) bob(20) cat(10)

	page, err := Select[User]().WhereCol("Age").Gt(5).Order("id asc").Page(ctx, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if page.TotalRows != 3 || page.TotalPages != 2 || page.PageNum != 2 {
		t.Fatalf("meta: %+v", page)
	}
	if len(page.Rows) != 1 || page.Rows[0].UserName != "cat" {
		t.Fatalf("rows: %+v", page.Rows)
	}

	// Pagination over the raw-SQL channel (count via tmp wrapper)
	page, err = Select[User]().BySQL("select * from users where age > ?", 5).Page(ctx, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if page.TotalRows != 3 || len(page.Rows) != 2 {
		t.Fatalf("custom page: %+v rows=%d", page, len(page.Rows))
	}

	// MapPage
	names := MapPage(page, func(u *User) string { return u.UserName })
	if names.TotalRows != 3 || *names.Rows[0] != page.Rows[0].UserName {
		t.Fatalf("map: %+v", names)
	}
}
