package anima

import (
	"errors"
	"runtime"
)

// RuntimeException marks error types that trigger rollback under the default
// RollbackWhen. An error implementing this interface rolls the transaction
// back; any other error commits as usual (the error is still stored in
// AtomicResult, with IsRollback=false).
type RuntimeException interface {
	error
	RuntimeException()
}

// Unchecked wraps a plain error in a type implementing RuntimeException, so
// the default RollbackWhen triggers a rollback. Use it when Save/Select fails
// inside a transaction fn and any-error-rolls-back semantics are wanted.
func Unchecked(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := err.(RuntimeException); ok {
		return err
	}
	return uncheckedErr{err}
}

type uncheckedErr struct{ err error }

func (u uncheckedErr) Error() string     { return u.err.Error() }
func (u uncheckedErr) Unwrap() error     { return u.err }
func (u uncheckedErr) RuntimeException() {}

// defaultRollbackWhen is the default rollback predicate: only RuntimeException
// implementations or runtime.Error trigger a rollback.
func defaultRollbackWhen(err error) bool {
	if err == nil {
		return false
	}
	if _, ok := err.(RuntimeException); ok {
		return true
	}
	var re runtime.Error
	return errors.As(err, &re)
}
