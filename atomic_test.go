package anima

import (
	"context"
	"database/sql/driver"
	"errors"
	"reflect"
	"strconv"
	"testing"
)

// ---- Enum interface (the @EnumMapping equivalent) ----

type Color int

const (
	ColorNone Color = iota
	ColorRed
	ColorBlue
)

func (c Color) ToDatabaseValue() any { return int(c) }

func (c *Color) SetDatabaseValue(v any) error {
	switch n := v.(type) {
	case int64:
		*c = Color(n)
	case []byte:
		i, err := strconv.ParseInt(string(n), 10, 64)
		*c = Color(i)
		return err
	case string:
		i, err := strconv.ParseInt(n, 10, 64)
		*c = Color(i)
		return err
	default:
		return errors.New("bad color value")
	}
	return nil
}

// ---- Converter registry ----

type Cents int64

type centsConverter struct{}

func (centsConverter) Target() reflect.Type { return reflect.TypeOf(Cents(0)) }
func (centsConverter) ToDatabase(v any) (any, error) {
	return int64(v.(Cents)), nil
}
func (centsConverter) FromDatabase(v any) (any, error) {
	switch n := v.(type) {
	case int64:
		return Cents(n), nil
	case []byte:
		i, err := strconv.ParseInt(string(n), 10, 64)
		return Cents(i), err
	case string:
		i, err := strconv.ParseInt(n, 10, 64)
		return Cents(i), err
	}
	return nil, errors.New("bad cents value")
}

// ---- Test-local nullable (NULL round-trip coverage: None writes NULL, NULL reads back as None) ----

type optInt struct {
	v  int
	ok bool
}

func optSome(v int) optInt { return optInt{v: v, ok: true} }
func optNone() optInt      { return optInt{} }

func (o optInt) Value() (driver.Value, error) {
	if !o.ok {
		return nil, nil
	}
	return int64(o.v), nil
}

func (o *optInt) Scan(src any) error {
	if src == nil {
		*o = optNone()
		return nil
	}
	switch n := src.(type) {
	case int64:
		*o = optSome(int(n))
	case []byte:
		i, err := strconv.ParseInt(string(n), 10, 64)
		if err != nil {
			return err
		}
		*o = optSome(int(i))
	default:
		return errors.New("bad optInt value")
	}
	return nil
}

func (o optInt) Get() (int, bool) { return o.v, o.ok }
func (o optInt) Ok() bool         { return o.ok }

type Product struct {
	Id    int64
	Name  string
	Stock optInt // NULL <-> local nullable
	Color Color  // enum interface channel
	Price Cents  // converter channel
}

func init() {
	AddConverter(centsConverter{})
}

func TestConverterEnumOptionRoundTrip(t *testing.T) {
	ctx := context.Background()
	if _, err := Execute(ctx, "DELETE FROM products"); err != nil {
		t.Fatal(err)
	}

	if _, err := Save(ctx, &Product{Name: "pen", Stock: optSome(5), Color: ColorBlue, Price: Cents(199)}); err != nil {
		t.Fatal(err)
	}
	if _, err := Save(ctx, &Product{Name: "ink", Stock: optNone(), Color: ColorRed, Price: Cents(0)}); err != nil {
		t.Fatal(err)
	}

	list, err := Select[Product]().Order("id asc").All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("len %d", len(list))
	}
	p := list[0]
	if p.Name != "pen" || p.Color != ColorBlue || p.Price != 199 {
		t.Fatalf("%+v", p)
	}
	if v, ok := p.Stock.Get(); !ok || v != 5 {
		t.Fatalf("stock %+v", p.Stock)
	}
	// Dual Some/None semantics: None writes NULL, and NULL reads back as None
	if list[1].Stock.Ok() {
		t.Fatalf("expected none, got %+v", list[1])
	}

	// Condition arguments also go through bind (enums/converters appear in WHERE)
	one, err := Select[Product]().WhereCol("Color").Eq(ColorRed).One(ctx)
	if err != nil || one == nil || one.Name != "ink" {
		t.Fatalf("where enum: %v %+v", err, one)
	}

	// The error path for scanning NULL into a non-pointer value field is guaranteed by Scan itself, so it is skipped here
}

// ---- Transactions Atomic ----

func userCount(t *testing.T) int64 {
	t.Helper()
	n, err := ScalarSQL[int64](context.Background(), "select count(*) from users")
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAtomicCommitAndRollback(t *testing.T) {
	ctx := context.Background()
	seedUsers(t) // 3 rows
	base := userCount(t)

	// Successful commit
	r := Atomic(ctx, func(ctx context.Context) error {
		_, err := Save(ctx, &User{UserName: "tx1", Age: 1})
		if err != nil {
			return err
		}
		_, err = Save(ctx, &User{UserName: "tx2", Age: 2})
		return err
	})
	if r.Error() != nil || r.IsRollback() {
		t.Fatalf("commit: %v", r.Error())
	}
	if got := userCount(t); got != base+2 {
		t.Fatalf("count after commit = %d", got)
	}

	// Checked-exception style: a plain error commits as usual (the default RollbackWhen triggers a rollback
	// only on RuntimeException, matching Java's "checked exception -> commit and store the exception in the Atomic").
	// go-mysql-server's ROLLBACK is a no-op, so only the observable fields of AtomicResult are verified here.
	r = Atomic(ctx, func(ctx context.Context) error {
		return errors.New("boom")
	})
	if r.Error() == nil || r.IsRollback() {
		t.Fatalf("plain error: expected error stored without rollback, got %+v", r)
	}
	caught := false
	r.CatchException(func(error) { caught = true })
	if !caught {
		t.Fatal("CatchException not called")
	}

	// Unchecked(err) promotes a plain error to RuntimeException and triggers a rollback.
	r = Atomic(ctx, func(ctx context.Context) error {
		return Unchecked(errors.New("boom-checked-promoted"))
	})
	if r.Error() == nil || !r.IsRollback() {
		t.Fatalf("Unchecked error must trigger rollback: %+v", r)
	}

	// A panic is likewise captured as the RuntimeException equivalent (uncheckedErr), triggering a rollback without blowing through.
	r = Atomic(ctx, func(ctx context.Context) error {
		panic("db is down")
	})
	if r.Error() == nil || !r.IsRollback() {
		t.Fatalf("panic rollback: %+v", r)
	}

	// CatchAndReturn
	if v, hit := CatchAndReturn(r, func(e error) string { return "handled:" + e.Error() }); !hit || v == "" {
		t.Fatalf("catchAndReturn: %v %v", v, hit)
	}
}

var errSkipRollback = errors.New("skip rollback sentinel")

func TestAtomicRollbackWhen(t *testing.T) {
	ctx := context.Background()
	base := userCount(t)
	a := Of()
	a.RollbackWhen(func(err error) bool { return !errors.Is(err, errSkipRollback) })
	defer a.RollbackWhen(nil) // Restore the default (the Java RuntimeException equivalent)

	r := Atomic(ctx, func(ctx context.Context) error {
		_, err := Save(ctx, &User{UserName: "keepme", Age: 9})
		if err != nil {
			return err
		}
		return errSkipRollback // Judged as no-rollback -> commit as usual
	})
	if r.Error() == nil || r.IsRollback() {
		t.Fatalf("expected no-rollback path: %+v", r)
	}
	if got := userCount(t); got != base+1 {
		t.Fatalf("count = %d, want committed", got)
	}
}
