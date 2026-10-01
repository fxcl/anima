package anima

import (
	"errors"
	"testing"
)

func TestErrorChainUnwrap(t *testing.T) {
	sentinel := errors.New("root cause")

	e := &Error{Code: 42, Msg: "boom", Err: sentinel}
	if !errors.Is(e, sentinel) {
		t.Fatal("errors.Is must reach the wrapped cause through Error.Unwrap")
	}
	if got, want := e.Error(), "anima: boom (code 42): root cause"; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}

	bare := &Error{Code: 42, Msg: "boom"}
	if got, want := bare.Error(), "anima: boom (code 42)"; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

func TestErrNotInitShape(t *testing.T) {
	err := errNotInit()
	var animaErr *Error
	if !errors.As(err, &animaErr) {
		t.Fatal("errNotInit must produce an *Error")
	}
	if animaErr.Code != ErrorCodeSql2oNotInit {
		t.Fatalf("Code = %d, want %d", animaErr.Code, ErrorCodeSql2oNotInit)
	}
}

func TestUncheckedErrChainAndMarker(t *testing.T) {
	sentinel := errors.New("tx blew up")
	u := uncheckedErr{err: sentinel}
	if !errors.Is(u, sentinel) {
		t.Fatal("errors.Is must reach the wrapped cause through uncheckedErr.Unwrap")
	}
	var rt RuntimeException = u
	if rt == nil {
		t.Fatal("uncheckedErr must satisfy RuntimeException")
	}
}

func TestRuntimeExceptionMarkerMethodCallable(t *testing.T) {
	u := uncheckedErr{err: errors.New("x")}
	u.RuntimeException()
}
