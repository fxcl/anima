package anima

import (
	"context"
	"errors"
	"testing"
)

// savepointActuallyRollsBack probes whether the driver in use actually rolls rows back on
// ROLLBACK TO SAVEPOINT. Known: real MySQL/PostgreSQL/Oracle/SqlServer2012 support it;
// go-mysql-server accepts the syntax but has no real side effect (a parse-level stub).
// On side-effect-free engines the "row-level rollback" assertions should be skipped.
func savepointActuallyRollsBack(t *testing.T) bool {
	t.Helper()
	a := Of()
	ctx := context.Background()
	if _, err := a.db.ExecContext(ctx, "DELETE FROM users"); err != nil {
		t.Fatal(err)
	}
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO users (user_name, age) VALUES ('sprobe_a', 1)"); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, "SAVEPOINT sps"); err != nil {
		t.Fatalf("SAVEPOINT: %v", err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO users (user_name, age) VALUES ('sprobe_b', 2)"); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT sps"); err != nil {
		t.Fatalf("ROLLBACK TO SAVEPOINT: %v", err)
	}
	if _, err := tx.ExecContext(ctx, "RELEASE SAVEPOINT sps"); err != nil {
		t.Fatalf("RELEASE SAVEPOINT: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := a.db.QueryRowContext(ctx, "select count(*) from users where user_name='sprobe_b'").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n == 0
}

// TestAtomicNestedCommit nested commit: both inner and outer succeed, so all rows are visible.
// It does not depend on real SAVEPOINT rollback behavior, so it passes on any engine.
func TestAtomicNestedCommit(t *testing.T) {
	ctx := context.Background()
	seedUsers(t)
	base := userCount(t)

	r := Atomic(ctx, func(ctx context.Context) error {
		if _, err := Save(ctx, &User{UserName: "outer-c", Age: 100}); err != nil {
			return err
		}
		return Atomic(ctx, func(ctx context.Context) error {
			_, err := Save(ctx, &User{UserName: "inner-c", Age: 101})
			return err
		}).Error()
	})
	if r.Error() != nil || r.IsRollback() {
		t.Fatalf("outer commit: %+v", r)
	}
	if got := userCount(t); got != base+2 {
		t.Fatalf("nested commit count = %d, want %d", got, base+2)
	}
}

// TestAtomicNestedSavepoint verifies the rollback semantics of nested Atomic via SAVEPOINT (outer/inner).
// Same shape as Java "beginTransaction is a no-op when a connection already exists", but via SAVEPOINT
// instead of Tx reuse: a *sql.Tx cannot be reused after Rollback, and a Tx wrapper would pollute the outer scope.
// Known: go-mysql-server accepts SAVEPOINT/ROLLBACK TO syntax but does not actually roll rows back (a parse-level stub),
// so "row-level rollback" is only verified when the engine genuinely supports it; the IsRollback flag is always trustworthy.
func TestAtomicNestedSavepoint(t *testing.T) {
	ctx := context.Background()
	if !savepointActuallyRollsBack(t) {
		t.Skip("engine in use does not actually undo rows on ROLLBACK TO SAVEPOINT; behavior still correct on real MySQL/PG/Oracle/SqlServer2012")
	}
	seedUsers(t)
	base := userCount(t)

	// Scenario 2: the inner rolls back so only the outer row is visible
	r := Atomic(ctx, func(ctx context.Context) error {
		_, err := Save(ctx, &User{UserName: "outer2", Age: 200})
		if err != nil {
			return err
		}
		inner := Atomic(ctx, func(ctx context.Context) error {
			_, err := Save(ctx, &User{UserName: "inner-rolled", Age: 201})
			if err != nil {
				return err
			}
			return Unchecked(errors.New("inner fail"))
		})
		if !inner.IsRollback() {
			t.Fatalf("inner expected rollback, got %+v", inner)
		}
		// The inner rollback must not affect the outer: keep committing normally
		return nil
	})
	if r.Error() != nil || r.IsRollback() {
		t.Fatalf("outer after inner-rollback: %+v", r)
	}
	if got := userCount(t); got != base+1 {
		t.Fatalf("after inner-rollback count = %d, want %d", got, base+1)
	}

	// Scenario 3: the outer rolls back so the inner row also disappears
	r = Atomic(ctx, func(ctx context.Context) error {
		_, err := Save(ctx, &User{UserName: "outer3", Age: 300})
		if err != nil {
			return err
		}
		return Atomic(ctx, func(ctx context.Context) error {
			_, err := Save(ctx, &User{UserName: "inner-vanished", Age: 301})
			return err
		}).Error()
	})
	if !r.IsRollback() {
		t.Fatalf("outer rollback expected: %+v", r)
	}
	if got := userCount(t); got != base+1 {
		t.Fatalf("after outer-rollback count = %d, want %d", got, base+1)
	}
}

// TestAtomicNestedPlainErrorNoRollback in a nested scope the inner returns a plain error:
// the default RollbackWhen triggers a rollback only on RuntimeException, so a plain error is stored in the
// inner result without releasing the SAVEPOINT, and the outer continues. Verifies: the outer sees its earlier
// writes, the inner has IsRollback=false but a non-nil Error.
func TestAtomicNestedPlainErrorNoRollback(t *testing.T) {
	ctx := context.Background()
	seedUsers(t)
	base := userCount(t)

	r := Atomic(ctx, func(ctx context.Context) error {
		if _, err := Save(ctx, &User{UserName: "outer-ne", Age: 500}); err != nil {
			return err
		}
		inner := Atomic(ctx, func(ctx context.Context) error {
			// a plain error does not trigger a rollback; the inner fn also raises no panic
			return errors.New("inner plain fail")
		})
		if inner.Error() == nil {
			t.Fatal("inner should carry the error")
		}
		if inner.IsRollback() {
			t.Fatalf("plain error must not roll back: %+v", inner)
		}
		// The inner is treated as a commit (no-op) and the outer continues
		return nil
	})
	if r.Error() != nil || r.IsRollback() {
		t.Fatalf("outer: %+v", r)
	}
	if got := userCount(t); got != base+1 {
		t.Fatalf("outer row count = %d, want %d", got, base+1)
	}
}

// TestAtomicNestedPanicInInner a panic in the inner nested scope is wrapped as uncheckedErr and triggers a rollback.
// Verifies: the inner panic is swallowed without leaking, IsRollback=true, and the outer is left unpolluted so it can continue.
func TestAtomicNestedPanicInInner(t *testing.T) {
	ctx := context.Background()
	seedUsers(t)
	base := userCount(t)

	r := Atomic(ctx, func(ctx context.Context) error {
		if _, err := Save(ctx, &User{UserName: "outer-panic", Age: 600}); err != nil {
			return err
		}
		inner := Atomic(ctx, func(ctx context.Context) error {
			panic("inner db down")
		})
		if !inner.IsRollback() {
			t.Fatalf("panic in inner must roll back: %+v", inner)
		}
		// The inner rollback must not affect the outer: keep committing normally
		return nil
	})
	if r.Error() != nil || r.IsRollback() {
		t.Fatalf("outer: %+v", r)
	}
	if got := userCount(t); got != base+1 {
		t.Fatalf("outer row count = %d, want %d", got, base+1)
	}
}

// TestAtomicNestedThreeLevels three-level nesting (outer -> middle -> inner):
// verifies the savepoint name is unique each time without collision, and all rows are visible after each layer commits.
func TestAtomicNestedThreeLevels(t *testing.T) {
	ctx := context.Background()
	seedUsers(t)
	base := userCount(t)

	r := Atomic(ctx, func(ctx context.Context) error {
		if _, err := Save(ctx, &User{UserName: "lv1", Age: 700}); err != nil {
			return err
		}
		return Atomic(ctx, func(ctx context.Context) error {
			if _, err := Save(ctx, &User{UserName: "lv2", Age: 701}); err != nil {
				return err
			}
			return Atomic(ctx, func(ctx context.Context) error {
				_, err := Save(ctx, &User{UserName: "lv3", Age: 702})
				return err
			}).Error()
		}).Error()
	})
	if r.Error() != nil || r.IsRollback() {
		t.Fatalf("three-level commit: %+v", r)
	}
	if got := userCount(t); got != base+3 {
		t.Fatalf("three-level count = %d, want %d", got, base+3)
	}
}
