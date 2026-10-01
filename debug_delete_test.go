package anima

import "testing"

func TestDebugDeleteSQL(t *testing.T) {
	Of().EnableSQLStatistic(true)
	defer Of().EnableSQLStatistic(false)
	ctx := t.Context()
	seedUsers(t)
	n, err := Delete(ctx, &User{Age: 10})
	t.Logf("DELETE affected=%d err=%v", n, err)
}
