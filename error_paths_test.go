package anima

import (
	"errors"
	"testing"
)

// NameOnlyRow deliberately has no primary key column: it exists to exercise
// the requirePK failure path shared by the keyed read/write APIs.
type NameOnlyRow struct {
	Name string
}

func TestErrorPathsRequirePKAndResolve(t *testing.T) {
	ctx := t.Context()

	if _, err := Save[User](ctx, nil); err == nil {
		t.Fatal("Save(nil) must fail via resolveEntity")
	}

	if _, err := Select[NameOnlyRow]().ByIds(ctx, 1); err == nil {
		t.Fatal("ByIds on a PK-less entity must fail")
	}

	if _, err := Select[NameOnlyRow]().WhereCol("name").Eq("x").UpdateById(ctx, 1); err == nil {
		t.Fatal("UpdateById on a PK-less entity must fail")
	}

	if _, err := DeleteById[NameOnlyRow](ctx, 1); err == nil {
		t.Fatal("DeleteById on a PK-less entity must fail")
	}

	var nullPtr *User
	if _, err := Save(ctx, nullPtr); err == nil {
		t.Fatal("Save of a nil *User must fail")
	}
}

func TestDeleteBatchNoOpAndMissingPK(t *testing.T) {
	ctx := t.Context()
	if n, err := DeleteBatch[User](ctx); err != nil || n != 0 {
		t.Fatalf("DeleteBatch with no ids must be a no-op, got (%d, %v)", n, err)
	}
	if _, err := DeleteBatch[NameOnlyRow](ctx, 1); err == nil {
		t.Fatal("DeleteBatch on a PK-less entity must fail")
	}
}

func TestResolveEntityRejectsNonStruct(t *testing.T) {
	ctx := t.Context()
	n := 5
	if _, err := Save(ctx, &n); err == nil {
		t.Fatal("Save of a non-struct must fail")
	}
}

func TestTypeNameCoversNullLiteral(t *testing.T) {
	if typeName(nil) != "NULL" {
		t.Fatal("typeName(nil) must render the NULL literal")
	}
}

// SaveBatch wraps Save in an Atomic block, so its edge semantics are
// transactional: empty input is a no-op, and one bad element must fail the
// whole batch as a rollback-triggering error.
//
// Row-level rollback is NOT asserted here: go-mysql-server's ROLLBACK — both
// tx ROLLBACK and ROLLBACK TO SAVEPOINT — is a no-op in the embedded test
// server (probe-verified 2026-09-29: rows persisted after both paths). The
// observable contract is therefore the RuntimeException wrap, which is what
// defaultRollbackWhen consults to trigger the rollback on a real server
// (same environment limitation as the comment in atomic_test.go).
func TestSaveBatchEdgeSemantics(t *testing.T) {
	ctx := t.Context()
	seedUsers(t)

	before := userCount(t)

	if err := SaveBatch[User](ctx, nil); err != nil {
		t.Fatalf("SaveBatch with a nil slice must be a no-op, got %v", err)
	}
	if err := SaveBatch[User](ctx, []*User{}); err != nil {
		t.Fatalf("SaveBatch with an empty slice must be a no-op, got %v", err)
	}
	if got := userCount(t); got != before {
		t.Fatalf("empty batch changed row count: %d -> %d", before, got)
	}

	err := SaveBatch[User](ctx, []*User{{UserName: "batch-ok", Age: 1}, nil})
	if err == nil {
		t.Fatal("SaveBatch containing a nil element must fail")
	}
	var re RuntimeException
	if !errors.As(err, &re) {
		t.Fatalf("SaveBatch failure must be a RuntimeException (rollback trigger under defaultRollbackWhen), got %T", err)
	}
	var aErr *Error
	if !errors.As(err, &aErr) || aErr.Code != ErrorCodeEmptyFromClause {
		t.Fatalf("SaveBatch failure must still unwrap to the original *Error, got %v", err)
	}
}

// PtrPKUser never touches the database: both of its conventions tests exit
// before any SQL is issued. It exists so the nil-PK guard in Update, which
// only fires for pointer-kind PK fields, is reachable and pinned.
type PtrPKUser struct {
	Id   *int64
	Name *string
}

// Model Update conventions: zero-value non-PK fields are written as-is (Go
// zero != SQL NULL; only the auto-increment PK zero is skipped), an
// all-nil pointer entity is a silent (0, nil) no-op, and a non-nil column
// paired with a nil PK fails with ErrorCodeNoPrimaryKey.
func TestUpdateConventionPins(t *testing.T) {
	ctx := t.Context()
	ids := seedUsers(t)

	if n, err := Update(ctx, &User{Id: ids[0]}); err != nil || n != 1 {
		t.Fatalf("update with zero-value fields must write them, got (%d, %v)", n, err)
	}
	back, err := Select[User]().ById(ctx, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if back.UserName != "" || back.Age != 0 {
		t.Fatalf("zero-value update must zero the row, got %+v", back)
	}

	if n, err := Update(ctx, &PtrPKUser{}); err != nil || n != 0 {
		t.Fatalf("all-nil entity must be a silent no-op, got (%d, %v)", n, err)
	}

	name := "ptr"
	if _, err := Update(ctx, &PtrPKUser{Name: &name}); err == nil {
		t.Fatal("Update with a nil PK and a non-nil column must fail")
	}
}
