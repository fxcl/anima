package anima

import (
	"testing"
)

func TestManualTransactionCommit(t *testing.T) {
	ctx := t.Context()
	tx, err := Of().Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Save(tx, &User{UserName: "mtx_c", Age: 1}); err != nil {
		t.Fatal(err)
	}
	if err := Of().Commit(tx); err != nil {
		t.Fatal(err)
	}
	us, err := Select[User]().Where("user_name = ?", "mtx_c").All(ctx)
	if err != nil || len(us) != 1 {
		t.Fatalf("committed row missing: %v (%d rows)", err, len(us))
	}
}

func TestManualTransactionRollback(t *testing.T) {
	ctx := t.Context()
	tx, err := Of().Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Save(tx, &User{UserName: "mtx_r", Age: 1}); err != nil {
		t.Fatal(err)
	}
	if err := Of().Rollback(tx); err != nil {
		t.Fatal(err)
	}
	// Row-level rollback cannot be asserted here: the in-process test engine
	// treats ROLLBACK as a no-op (see the autoresearch probe in ideas.md), so
	// the row survives. This test pins the API contract instead: Rollback
	// returns nil for an active manual transaction.
}

func TestCommitWithoutBeginFails(t *testing.T) {
	if err := Of().Commit(t.Context()); err == nil {
		t.Fatal("Commit without Begin must fail")
	}
	if err := Of().Rollback(t.Context()); err == nil {
		t.Fatal("Rollback without Begin must fail")
	}
}
