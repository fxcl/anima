package anima

import (
	"context"
	"errors"
	"testing"

	"github.com/fxcl/anima/dialect"
)

// Connection-lifecycle contract pins: setter semantics, failure isolation of
// Open, and the nil-default vs nil-db distinction. All environment-agnostic.

// Fluent setters mutate the instance IN PLACE and return it — the Java-port
// idiom Of().TablePrefix("t") only works because the returned pointer IS the
// global instance. Pins that no setter drifts into copy-on-write semantics.
func TestSettersMutateInPlace(t *testing.T) {
	a := Of()
	if a == nil {
		t.Fatal("TestMain must wire the default")
	}
	if got := a.TablePrefix("x"); got != a {
		t.Fatal("TablePrefix must return the same instance")
	}
	a.TablePrefix("")
	if got := a.Dialect(dialect.MySQL{}); got != a {
		t.Fatal("Dialect must return the same instance")
	}
	if got := a.UseSQLLimit(true); got != a {
		t.Fatal("UseSQLLimit must return the same instance")
	}
	if got := a.RollbackWhen(nil); got != a {
		t.Fatal("RollbackWhen(nil) must return the same instance")
	}
	if got := a.EnableSQLStatistic(true); got != a {
		t.Fatal("EnableSQLStatistic must return the same instance")
	}
}

// A failed Open must not clobber the default instance: the swap happens only
// after open AND ping succeed.
func TestFailedOpenLeavesDefaultUntouched(t *testing.T) {
	prev := Of()
	defer SetDefault(prev)

	if _, err := OpenWith("mysql", "root@tcp(127.0.0.1:1)/none"); err == nil {
		t.Fatal("unreachable DSN must fail (OpenWith pings eagerly)")
	}
	if Of() != prev {
		t.Fatal("failed OpenWith (bad ping) must not replace the default")
	}
	if _, err := OpenWith("no-such-driver", "x"); err == nil {
		t.Fatal("unknown driver must fail")
	}
	if Of() != prev {
		t.Fatal("failed OpenWith (unknown driver) must not replace the default")
	}
}

// SetDefault(nil) and OpenDB(nil) are distinct broken states: the former
// makes Of() itself nil (everything answers not-init before any lookup),
// the latter yields a live default whose nil *sql.DB still surfaces the same
// not-init error at the execution exit.
func TestNilDefaultVsNilDB(t *testing.T) {
	prev := Of()
	defer SetDefault(prev)

	SetDefault(nil)
	if Of() != nil {
		t.Fatal("SetDefault(nil) must make Of() return nil")
	}

	SetDefault(OpenDB(nil))
	if Of() == nil {
		t.Fatal("OpenDB(nil) must still install a non-nil default")
	}
	if _, err := Save[User](context.Background(), &User{UserName: "lifecycle"}); err == nil {
		t.Fatal("queries on a nil-db default must fail")
	} else {
		var aErr *Error
		if !errors.As(err, &aErr) || aErr.Code != ErrorCodeSql2oNotInit {
			t.Fatalf("nil-db failure must surface not-init, got %v", err)
		}
	}
}

// SetPool is a thin database/sql delegation; pin the boundary semantics the
// one existing pool test does not: 0 means unlimited (pass-through), a
// negative idle is accepted WITHOUT panic (modern database/sql clamps
// instead of panicking), and positive values reflect in Stats.
func TestSetPoolEdgeSemantics(t *testing.T) {
	a := Of()
	if a == nil {
		t.Fatal("TestMain must wire the default")
	}
	defer a.SetPool(0, 0)

	a.SetPool(0, 0)
	if st := a.db.Stats(); st.MaxOpenConnections != 0 {
		t.Fatalf("SetPool(0, 0) must pass through 0-as-unlimited, got %d", st.MaxOpenConnections)
	}
	a.SetPool(2, -1)
	if st := a.db.Stats(); st.MaxOpenConnections != 2 {
		t.Fatalf("SetPool(2, -1) must accept negative idle without panic, got %d", st.MaxOpenConnections)
	}
	a.SetPool(3, 2)
	if st := a.db.Stats(); st.MaxOpenConnections != 3 {
		t.Fatalf("SetPool(3, 2) must reflect in Stats, got %d", st.MaxOpenConnections)
	}
}
