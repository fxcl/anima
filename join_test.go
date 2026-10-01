package anima

import (
	"bytes"
	"context"
	"log"
	"reflect"
	"strings"
	"testing"
)

// ---- Association backfill (alignment guard for Java JoinTest: @Ignore field mounting + N+1 backfill) ----

type Player struct {
	Id     int64
	Name   string
	TeamId int64
}

type Team struct {
	Id        int64
	Name      string
	CaptainId int64
}

// TeamFull.Players is the hasMany mount point and Captain is hasOne (alignment guard for @Ignore private List<OrderInfo> orders)
type TeamFull struct {
	Id        int64
	Name      string
	CaptainId int64
	Players   []*Player `anima:"-"`
	Captain   *Player   `anima:"-"`
}

// TableName overrides the default plural rule (alignment guard for Java @Table(name = "teams"))
func (TeamFull) TableName() string { return "teams" }

func seedTeams(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	for _, tbl := range []string{"teams", "players"} {
		if _, err := Execute(ctx, "DELETE FROM "+tbl); err != nil {
			t.Fatal(err)
		}
	}
	k, err := Save(ctx, &Team{Name: "eagles", CaptainId: 11})
	if err != nil {
		t.Fatal(err)
	}
	teamID := k.AsLong()
	for _, p := range []struct {
		id  int64
		n   string
		tid int64
	}{{11, "zion", teamID}, {12, "nero", teamID}} {
		if _, err := Save(ctx, &Player{Id: p.id, Name: p.n, TeamId: p.tid}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestJoinBackfill(t *testing.T) {
	ctx := context.Background()
	seedTeams(t)

	// Alignment guard for select().from(TeamFull.class).join(Joins.with(Player.class).as(TeamFull::getPlayers).on(TeamFull::getId, Player::getTeamId))
	list, err := Select[TeamFull]().
		Join(
			JoinWith[Player]().As("Players").On("Id", "TeamId").Order("id asc"),
			JoinWith[Player]().As("Captain").On("CaptainId", "Id"),
		).
		All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("%d teams", len(list))
	}
	tf := list[0]
	if len(tf.Players) != 2 || tf.Players[0].Name != "zion" || tf.Players[1].Name != "nero" {
		t.Fatalf("hasMany: %+v", tf.Players)
	}
	if tf.Captain == nil || tf.Captain.Name != "zion" {
		t.Fatalf("hasOne: %+v", tf.Captain)
	}

	// With no matching rows hasOne stays nil and hasMany is an empty slice
	seedTeams(t)
	if _, err := Execute(ctx, "DELETE FROM players"); err != nil {
		t.Fatal(err)
	}
	one, err := Select[TeamFull]().
		Join(
			JoinWith[Player]().As("Players").On("Id", "TeamId"),
			JoinWith[Player]().As("Captain").On("CaptainId", "Id"),
		).
		One(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(one.Players) != 0 || one.Captain != nil {
		t.Fatalf("empty joins: %+v", one)
	}
}

func TestJoinUnknownField(t *testing.T) {
	ctx := context.Background()
	_, err := Select[TeamFull]().Join(JoinWith[Player]().As("Nope").On("Id", "TeamId")).All(ctx)
	if err == nil || !strings.Contains(err.Error(), "Nope") {
		t.Fatalf("want as-field error, got %v", err)
	}
}

// ---- Batch ----

func TestBatch(t *testing.T) {
	ctx := context.Background()
	seedUsers(t) // Clears users and inserts 3 rows

	before := userCount(t)
	if err := SaveBatch(ctx, []*User{{UserName: "b1", Age: 1}, {UserName: "b2", Age: 2}}); err != nil {
		t.Fatal(err)
	}
	if got := userCount(t); got != before+2 {
		t.Fatalf("after batch = %d", got)
	}

	list, err := Select[User]().WhereCol("Age").Le(2).All(ctx)
	if err != nil || len(list) != 2 {
		t.Fatalf("batch rows: %v %+v", err, list)
	}
	ids := []any{list[0].Id, list[1].Id}
	n, err := DeleteBatch[User](ctx, ids...)
	if err != nil || n != 2 {
		t.Fatalf("deleteBatch: %v %d", err, n)
	}
}

// ---- tablePrefix ----

type PrefixedUser struct {
	Id       int64
	UserName string
	Age      int
}

func TestTablePrefix(t *testing.T) {
	a := Of()
	a.TablePrefix("t")
	defer a.TablePrefix("")

	// Alignment guard for Java: plural(prefix + "_" + toUnderline(className)) => t_prefixed_users
	m := a.metaOf(reflect.TypeOf(PrefixedUser{}))
	if m.table != "t_prefixed_users" {
		t.Fatalf("table = %q", m.table)
	}
}

// ---- SQL statistic toggle (on by default, off during tests) ----

func TestSQLStatisticLog(t *testing.T) {
	a := Of()
	var buf bytes.Buffer
	oldOut := log.Writer()
	oldFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(oldOut)
		log.SetFlags(oldFlags)
		a.EnableSQLStatistic(false)
	}()

	a.EnableSQLStatistic(true)
	if _, err := Select[User]().Count(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "SELECT COUNT(*) FROM `users`") {
		t.Fatalf("statistic log missing: %q", buf.String())
	}

	buf.Reset()
	a.EnableSQLStatistic(false)
	if _, err := Select[User]().Count(context.Background()); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != 0 {
		t.Fatalf("statistic should be silent: %q", buf.String())
	}
}
