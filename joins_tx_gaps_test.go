package anima

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
)

// Fixture set for the association error matrix: every mount field is tagged
// anima:"-" so it is a recognized mount point, and the shapes are wrong on
// purpose. GhostRow has no backing table (ghost_rows is never created).
type GhostRow struct {
	Id     int64
	TeamId int64
}

type TeamShapes struct {
	Id        int64
	Name      string
	CaptainId int64
	BadList   []int         `anima:"-"`
	BadPtr    *string       `anima:"-"`
	BadScalar string        `anima:"-"`
	Ghosts    []*GhostRow   `anima:"-"`
	ValPlayers []Player     `anima:"-"`
}

func (TeamShapes) TableName() string { return "teams" }

// The default-table derivation for GhostRow must not collide with a real
// table; if someone adds a ghost_rows table this fixture needs renaming.
func TestJoinShapeErrorMatrix(t *testing.T) {
	ctx := t.Context()
	seedTeams(t)

	cases := []struct {
		name string
		spec *JoinSpec
		want string
	}{
		{"on-left unknown", JoinWith[Player]().As("BadList").On("Nope", "TeamId"), `on-left "Nope" not found`},
		{"on-right unknown", JoinWith[Player]().As("BadList").On("Id", "Nope"), `on-right "Nope" not found`},
		{"hasMany wrong elem", JoinWith[Player]().As("BadList").On("Id", "TeamId"), "want []Player"},
		{"hasOne wrong elem", JoinWith[Player]().As("BadPtr").On("CaptainId", "Id"), "want *Player"},
		{"scalar mount", JoinWith[Player]().As("BadScalar").On("CaptainId", "Id"), "must be *Player or []Player"},
	}
	for _, c := range cases {
		_, err := Select[TeamShapes]().Join(c.spec).All(ctx)
		if err == nil || !contains(err.Error(), c.want) {
			t.Fatalf("%s: want %q in error, got %v", c.name, c.want, err)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// JoinWith of an interface type falls back to the (*J)(nil)).Elem() idiom and
// must land in the not-a-struct error; Join must propagate that spec error
// into the query so the terminal call fails.
func TestJoinWithInterfaceAndSpecErrorPropagation(t *testing.T) {
	ctx := t.Context()
	seedTeams(t)

	spec := JoinWith[any]()
	if spec.err == nil {
		t.Fatal("JoinWith[any] must carry a not-a-struct error")
	}
	if _, err := Select[TeamShapes]().Join(spec).All(ctx); err == nil {
		t.Fatal("Join must propagate the spec error to the terminal call")
	}
}

// The joined entity's table is derived from its type: GhostRow maps to
// ghost_rows, which does not exist, so the association query fails even
// though the main query succeeded.
func TestJoinMissingJoinedTableSurfacesQueryError(t *testing.T) {
	ctx := t.Context()
	seedTeams(t)

	_, err := Select[TeamShapes]().Join(JoinWith[GhostRow]().As("Ghosts").On("Id", "TeamId")).All(ctx)
	if err == nil {
		t.Fatal("association query against a missing table must fail")
	}
}

// hasMany also backfills value-element slices ([]Player, not just []*Player).
func TestJoinHasManyValueElements(t *testing.T) {
	ctx := t.Context()
	seedTeams(t)

	list, err := Select[TeamShapes]().
		Join(JoinWith[Player]().As("ValPlayers").On("Id", "TeamId").Order("id asc")).
		All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || len(list[0].ValPlayers) != 2 ||
		list[0].ValPlayers[0].Name != "zion" || list[0].ValPlayers[1].Name != "nero" {
		t.Fatalf("value-element hasMany: %+v", list)
	}
}

// Atomic's not-init and BeginTx-failure paths both return the error in the
// AtomicResult without a rollback flag (no transaction was ever open).
func TestAtomicNotInitAndBeginFailure(t *testing.T) {
	ctx := t.Context()
	prev := Of()

	SetDefault(nil)
	r := Atomic(ctx, func(ctx context.Context) error { return nil })
	if r.Error() == nil || !isNotInit(r.Error()) {
		t.Fatalf("Atomic without init must carry errNotInit, got %v", r.Error())
	}
	if r.IsRollback() {
		t.Fatal("not-init failure must not claim a rollback")
	}

	db, err := sql.Open("mysql", "root@tcp(127.0.0.1:1)/nope")
	if err != nil {
		t.Fatal(err)
	}
	a := OpenDB(db)
	SetDefault(a)
	db.Close()
	r2 := Atomic(ctx, func(ctx context.Context) error { return nil })
	SetDefault(prev)

	if r2.Error() == nil {
		t.Fatal("Atomic on a closed db must fail at BeginTx")
	}
	if !contains(r2.Error().Error(), "database is closed") {
		t.Fatalf("want the driver's closed-db error, got %v", r2.Error())
	}
	if r2.IsRollback() {
		t.Fatal("BeginTx failure must not claim a rollback")
	}
}

// CatchAndReturn's documented contract: (zero, false) on success, the
// callback's value and true on failure.
func TestCatchAndReturnBothPaths(t *testing.T) {
	v, had := CatchAndReturn[int](&AtomicResult{}, func(err error) int {
		t.Fatal("callback must not run on success")
		return -1
	})
	if v != 0 || had {
		t.Fatalf("success must be (zero, false), got (%d, %v)", v, had)
	}

	boom := errors.New("boom")
	v2, had2 := CatchAndReturn[int](&AtomicResult{err: boom}, func(err error) int {
		if !errors.Is(err, boom) {
			t.Fatalf("callback must receive the stored error, got %v", err)
		}
		return 7
	})
	if v2 != 7 || !had2 {
		t.Fatalf("failure must be (callback value, true), got (%d, %v)", v2, had2)
	}
}

var _ = fmt.Sprintf // keep fmt available for fixture debugging
