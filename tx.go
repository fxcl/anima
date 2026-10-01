package anima

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"sync/atomic"
)

// AtomicResult is the transaction outcome. It is anima's signature design: a
// transaction error is never thrown but stored in an observable object, left
// for the caller to handle via CatchException / Error.
type AtomicResult struct {
	err        error
	isRollback bool
}

// Error returns the transaction error, or nil on success.
func (r *AtomicResult) Error() error { return r.err }

// AtomicOK returns a success outcome (Java Atomic.ok()).
func AtomicOK() *AtomicResult { return &AtomicResult{} }

// AtomicError returns a failed outcome carrying err (Java Atomic.error(e)).
func AtomicError(err error) *AtomicResult { return &AtomicResult{err: err} }

// Rollback sets the rollback flag chain-style (Java Atomic.rollback(boolean)).
func (r *AtomicResult) Rollback(v bool) *AtomicResult { r.isRollback = v; return r }

// IsRollback reports whether a rollback happened.
func (r *AtomicResult) IsRollback() bool { return r.isRollback }

// CatchException installs the exception callback.
func (r *AtomicResult) CatchException(f func(error)) *AtomicResult {
	if r.err != nil {
		f(r.err)
	}
	return r
}

// CatchAndReturn invokes the callback on error and exchanges the value.
// Go has no null, so on success it returns (zero value, false).
func CatchAndReturn[R any](r *AtomicResult, f func(error) R) (R, bool) {
	var zero R
	if r.err != nil {
		return f(r.err), true
	}
	return zero, false
}

// savepointSeq feeds nested SAVEPOINT names (sp1/sp2/...).
// Monotonically increasing process-wide, so that same-named savepoints never
// coexist inside one parent transaction.
var savepointSeq uint64

// nextSavepoint generates a unique SAVEPOINT name.
func nextSavepoint() string {
	n := atomic.AddUint64(&savepointSeq, 1)
	return "sp" + strconv.FormatUint(n, 10)
}

// Atomic runs fn inside a transaction.
// Inside fn, any anima API (Save/Select/Execute, etc.) can be used through
// the ctx; they reuse the same transaction connection via context.
// An error or panic returned by fn counts as an exception: RollbackWhen
// decides whether to roll back, and the exception is swallowed into the
// returned AtomicResult.
//
// The default RollbackWhen rolls back only on errors implementing
// RuntimeException or runtime.Error. A panic is wrapped as uncheckedErr after
// recovery, which satisfies the RuntimeException condition automatically.
//
// Nesting semantics: when ctx already carries an outer transaction, this
// implementation uses SAVEPOINT/RELEASE SAVEPOINT/ROLLBACK TO SAVEPOINT — the
// inner unit is part of the outer one, a nested rollback goes through
// SAVEPOINT, and an outer rollback takes the inner one with it.
// MySQL/PostgreSQL/SQLite/Oracle/SqlServer2012 support this natively; a
// database/sql Tx cannot be reused after rollback, so this path uses
// SAVEPOINT instead of a Tx wrapper — here inner success becomes
// RELEASE SAVEPOINT (releasing the checkpoint only; persistence stays with
// the outer transaction).
func Atomic(ctx context.Context, fn func(ctx context.Context) error) *AtomicResult {
	a := Of()
	if a == nil {
		return &AtomicResult{err: errNotInit()}
	}

	// Nested branch: ctx already carries a transaction -> SAVEPOINT nesting
	if parentTx, ok := ctx.Value(txKey{}).(*sql.Tx); ok {
		return nestedSavepoint(ctx, a, parentTx, fn)
	}

	var (
		tx      *sql.Tx
		failure error
	)
	// BeginTx and fn share one recover guard: a nil-pointer panic from
	// BeginTx (e.g. OpenDB(nil)) is also converted into AtomicResult{err}
	// instead of crashing the process.
	func() {
		defer func() {
			if p := recover(); p != nil {
				failure = uncheckedErr{err: fmt.Errorf("anima: panic in atomic: %v", p)}
			}
		}()
		t, err := a.db.BeginTx(ctx, nil)
		if err != nil {
			failure = err
			return
		}
		tx = t
		failure = fn(context.WithValue(ctx, txKey{}, t))
	}()

	// Transaction already failed to open (including a nil-db panic): nothing
	// to roll back, return the error directly.
	if tx == nil {
		return &AtomicResult{err: failure}
	}
	if failure == nil {
		if cerr := tx.Commit(); cerr != nil {
			return &AtomicResult{err: fmt.Errorf("commit failed: %w", cerr), isRollback: true}
		}
		return &AtomicResult{}
	}

	if a.rollbackWhen(failure) {
		if rerr := tx.Rollback(); rerr != nil {
			return &AtomicResult{err: fmt.Errorf("rollback failed: %w (original: %v)", rerr, failure), isRollback: true}
		}
		return &AtomicResult{err: failure, isRollback: true}
	}
	// Rollback predicate not satisfied: commit as usual
	if cerr := tx.Commit(); cerr != nil {
		return &AtomicResult{err: fmt.Errorf("commit failed: %w (original: %v)", cerr, failure)}
	}
	return &AtomicResult{err: failure, isRollback: false}
}

// nestedSavepoint opens a SAVEPOINT on the outer transaction's connection,
// rolling back with ROLLBACK TO SAVEPOINT and succeeding with RELEASE
// SAVEPOINT. It shares one connection with the parent transaction.
func nestedSavepoint(ctx context.Context, a *Anima, parentTx *sql.Tx, fn func(ctx context.Context) error) *AtomicResult {
	sp := nextSavepoint()
	var (
		failure error
		opened  bool
	)

	func() {
		defer func() {
			if p := recover(); p != nil {
				failure = uncheckedErr{err: fmt.Errorf("anima: panic in atomic: %v", p)}
			}
		}()
		if _, err := a.execCtx(ctx, parentTx, "SAVEPOINT "+sp, nil); err != nil {
			failure = err
			return
		}
		opened = true
		failure = fn(ctx) // fn's ctx already carries the parent transaction
	}()

	// SAVEPOINT itself failed (unsupported driver, etc.): fn never ran, so
	// this is an open-transaction failure without the rollback flag.
	if !opened {
		return &AtomicResult{err: failure}
	}
	if failure == nil {
		// Inner success: RELEASE SAVEPOINT only releases the checkpoint and
		// does not persist; commit is the outer transaction's job.
		if _, rerr := a.execCtx(ctx, parentTx, "RELEASE SAVEPOINT "+sp, nil); rerr != nil {
			return &AtomicResult{err: fmt.Errorf("release savepoint failed: %w", rerr), isRollback: true}
		}
		return &AtomicResult{}
	}

	if a.rollbackWhen(failure) {
		if _, rerr := a.execCtx(ctx, parentTx, "ROLLBACK TO SAVEPOINT "+sp, nil); rerr != nil {
			return &AtomicResult{err: fmt.Errorf("rollback to savepoint failed: %w (original: %v)", rerr, failure), isRollback: true}
		}
		// The savepoint still exists after ROLLBACK TO SAVEPOINT; release it
		// explicitly so the outer transaction is unaffected. If RELEASE fails,
		// the outer rollback is the safety net.
		_, _ = a.execCtx(ctx, parentTx, "RELEASE SAVEPOINT "+sp, nil)
		return &AtomicResult{err: failure, isRollback: true}
	}
	// Rollback predicate not satisfied: same as an inner commit; store the
	// exception and release the checkpoint.
	if _, rerr := a.execCtx(ctx, parentTx, "RELEASE SAVEPOINT "+sp, nil); rerr != nil {
		return &AtomicResult{err: fmt.Errorf("release savepoint failed: %w (original: %v)", rerr, failure)}
	}
	return &AtomicResult{err: failure, isRollback: false}
}
