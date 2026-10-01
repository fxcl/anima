package anima_test

import (
	"context"

	"github.com/fxcl/anima"
)

type ExampleUser struct {
	Id   int64
	Name string
	Age  int
}

func ExampleOpenWith() {
	// Point anima at a MySQL-compatible server, size the pool for the
	// expected concurrency, and make it the process-wide default.
	a, err := anima.OpenWith("mysql", "root@tcp(127.0.0.1:3306)/app?parseTime=true")
	if err != nil {
		panic(err)
	}
	a.SetPool(64, 32)
	anima.SetDefault(a)
}

func ExampleSelect() {
	ctx := context.Background()
	users, err := anima.Select[ExampleUser]().
		WhereCol("Age").Ge(18).
		OrderCol("Id", "asc").
		All(ctx)
	if err != nil {
		panic(err)
	}
	_ = users
}

func ExampleSave() {
	ctx := context.Background()
	u := &ExampleUser{Name: "ada", Age: 36}
	if _, err := anima.Save(ctx, u); err != nil {
		panic(err)
	}
	// Save fills u.Id from the auto-increment key.
}

func ExampleUpdateFrom() {
	ctx := context.Background()
	if _, err := anima.UpdateFrom[ExampleUser]().
		Set("Name", "grace").
		UpdateById(ctx, 1); err != nil {
		panic(err)
	}
}

func ExampleQuery_Page() {
	ctx := context.Background()
	page, err := anima.Select[ExampleUser]().Page(ctx, 2, 20)
	if err != nil {
		panic(err)
	}
	_ = page
}

func ExampleAtomic() {
	// Any returned error (or panic) rolls the whole callback back.
	ctx := context.Background()
	ar := anima.Atomic(ctx, func(ctx context.Context) error {
		if _, err := anima.Save(ctx, &ExampleUser{Name: "nested"}); err != nil {
			return err
		}
		return nil
	})
	if err := ar.Error(); err != nil {
		panic(err)
	}
}
