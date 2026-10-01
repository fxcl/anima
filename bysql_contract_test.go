package anima

import (
	"errors"
	"testing"
)

// The bysql.go doc comments promise pointer scalars for nullable columns:
// ScalarSQL[*string] must map NULL to a nil pointer and a present value to a
// non-nil one. scanScalarRow's pointer branch had no coverage before this.
func TestScalarSQLPointerNullAndValue(t *testing.T) {
	ctx := t.Context()
	ids := seedUsers(t)

	email, err := ScalarSQL[*string](ctx, "select email from users where id = ?", ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if email != nil {
		t.Fatalf("seeded email is NULL, must map to a nil *string, got %v", *email)
	}

	if _, err := Execute(ctx, "update users set email = ? where id = ?", "x@y.z", ids[0]); err != nil {
		t.Fatal(err)
	}
	email, err = ScalarSQL[*string](ctx, "select email from users where id = ?", ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if email == nil || *email != "x@y.z" {
		t.Fatalf("non-NULL email must map to a non-nil pointer, got %v", email)
	}
}

// ScalarSQL's documented no-result contract: zero value of V, nil error.
func TestScalarSQLZeroOnEmptyResult(t *testing.T) {
	ctx := t.Context()
	seedUsers(t)

	v, err := ScalarSQL[int](ctx, "select id from users where id < 0")
	if err != nil || v != 0 {
		t.Fatalf("empty scalar must be (zero, nil), got (%d, %v)", v, err)
	}
}

// QuerySQL returns the out := []V{} empty slice, i.e. a non-nil slice on no rows.
func TestQuerySQLReturnsNonNilEmptySlice(t *testing.T) {
	ctx := t.Context()
	seedUsers(t)

	names, err := QuerySQL[string](ctx, "select user_name from users where id < 0")
	if err != nil {
		t.Fatal(err)
	}
	if names == nil || len(names) != 0 {
		t.Fatalf("empty result must be a non-nil empty slice, got %#v", names)
	}
}

func TestBysqlErrorPropagation(t *testing.T) {
	ctx := t.Context()
	seedUsers(t)

	if _, err := ScalarSQL[int](ctx, "select * from no_such_table"); err == nil {
		t.Fatal("bad SQL must fail")
	}
	if _, err := QuerySQL[string](ctx, "select * from no_such_table"); err == nil {
		t.Fatal("bad SQL must fail")
	}

	// A string column scanned into an int destination must surface the
	// database/sql Scan error, not a silent zero.
	if _, err := QuerySQL[int](ctx, "select user_name from users"); err == nil {
		t.Fatal("string column into int destination must fail")
	}
}

// SetDefault(nil) makes Of() return nil: every entry point must answer with
// the errNotInit sentinel instead of panicking.
func TestBysqlAndSaveNotInit(t *testing.T) {
	prev := Of()
	SetDefault(nil)
	defer SetDefault(prev)
	ctx := t.Context()

	if _, err := Execute(ctx, "select 1"); !isNotInit(err) {
		t.Fatalf("Execute must return errNotInit, got %v", err)
	}
	if _, err := QuerySQL[int](ctx, "select 1"); !isNotInit(err) {
		t.Fatalf("QuerySQL must return errNotInit, got %v", err)
	}
	if _, err := ScalarSQL[int](ctx, "select 1"); !isNotInit(err) {
		t.Fatalf("ScalarSQL must return errNotInit, got %v", err)
	}
	if _, err := Save(ctx, &User{UserName: "x"}); !isNotInit(err) {
		t.Fatalf("Save must return errNotInit, got %v", err)
	}
	if _, err := Update(ctx, &User{UserName: "x"}); !isNotInit(err) {
		t.Fatalf("Update must return errNotInit, got %v", err)
	}
	if _, err := Delete(ctx, &User{UserName: "x"}); !isNotInit(err) {
		t.Fatalf("Delete must return errNotInit, got %v", err)
	}
}

func isNotInit(err error) bool {
	var aErr *Error
	return errors.As(err, &aErr) && aErr.Code == ErrorCodeSql2oNotInit
}

// Update's requirePK path fires on a plain Update too, not just UpdateById;
// Update(nil) must fail in resolveEntity before any reflection on fields.
func TestUpdateRequirePKAndNilEntity(t *testing.T) {
	ctx := t.Context()
	seedUsers(t)

	if _, err := Update(ctx, &NameOnlyRow{Name: "x"}); err == nil {
		t.Fatal("Update on a PK-less entity must fail")
	}
	if _, err := Update[User](ctx, nil); err == nil {
		t.Fatal("Update(nil) must fail via resolveEntity")
	}
}

// Delete with every field nil has no WHERE-worthy column: unlike Update's
// silent no-op it is an error (ErrorCodeEmptyFromClause), because a WHERE-less
// DELETE would truncate the table. The fixture never issues SQL.
func TestDeleteAllNilFieldsIsError(t *testing.T) {
	ctx := t.Context()

	n, err := Delete(ctx, &PtrPKUser{})
	if err == nil || n != 0 {
		t.Fatalf("Delete of an all-nil entity must fail, got (%d, %v)", n, err)
	}
	var aErr *Error
	if !errors.As(err, &aErr) || aErr.Code != ErrorCodeEmptyFromClause {
		t.Fatalf("all-nil Delete must be ErrorCodeEmptyFromClause, got %v", err)
	}
}

// Save with an explicit non-zero PK: the returned key must round-trip the
// explicit id whether the driver reports it via LastInsertId directly or the
// key==0 fallback reads it from the entity.
func TestSaveExplicitPKKeyRoundTrip(t *testing.T) {
	ctx := t.Context()
	seedUsers(t)

	key, err := Save(ctx, &User{Id: 901, UserName: "explicit", Age: 1})
	if err != nil {
		t.Fatal(err)
	}
	if key.AsLong() != 901 {
		t.Fatalf("explicit PK must round-trip through the result key, got %d", key.AsLong())
	}
}
