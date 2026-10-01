package anima

import (
	"testing"
)

func TestOrderColRejectsNonDirectionalDir(t *testing.T) {
	ctx := t.Context()
	seedUsers(t)

	if _, err := Select[User]().OrderCol("Age", "ASC, 1=1").All(ctx); err == nil {
		t.Fatal("OrderCol must reject a dir that is not a pure direction")
	}
	if _, err := Select[User]().OrderCol("Age", "SORT").All(ctx); err == nil {
		t.Fatal("OrderCol must reject unknown direction keywords")
	}
	us, err := Select[User]().OrderCol("Age", "desc").All(ctx)
	if err != nil {
		t.Fatalf("case-insensitive direction must be accepted: %v", err)
	}
	if len(us) < 2 || us[0].Age < us[len(us)-1].Age {
		t.Fatal("DESC ordering not observed")
	}
}

func TestJoinSpecOrderRejectsNonIdentifierExpression(t *testing.T) {
	if _, err := Select[Team]().Join(JoinWith[Player]().Order("name; DROP TABLE players")).All(t.Context()); err == nil {
		t.Fatal("JoinSpec.Order must reject non identifier expressions")
	}
	if _, err := Select[Team]().Join(JoinWith[Player]().Order("name DESC)")).All(t.Context()); err == nil {
		t.Fatal("JoinSpec.Order must reject stray SQL punctuation")
	}
	if _, err := Select[Team]().Join(JoinWith[Player]().Order("score desc")).All(t.Context()); err != nil {
		t.Fatalf("column + direction must stay legal: %v", err)
	}
}
