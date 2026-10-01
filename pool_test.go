package anima

import "testing"

func TestSetPoolConfiguresAndChains(t *testing.T) {
	mu.RLock()
	a := defaultAnima
	mu.RUnlock()

	got := a.SetPool(17, 9)
	if got != a {
		t.Fatal("SetPool must return the receiver for chaining")
	}
	if st := a.db.Stats(); st.MaxOpenConnections != 17 {
		t.Fatalf("MaxOpenConnections = %d, want 17", st.MaxOpenConnections)
	}
}
