package anima

import "testing"

// TestPageDoesNotLeakCountRows pins the count-query lifetime of Page. Page
// hands its COUNT(*) rows to scanCount, which owns closing them; if that
// ownership is ever dropped, every paged query pins one pooled connection
// until GC finalizers run, which exhausts small connection pools under
// pagination-heavy load.
func TestPageDoesNotLeakCountRows(t *testing.T) {
	ctx := t.Context()
	mu.RLock()
	db := defaultAnima.db
	mu.RUnlock()

	if _, err := Select[User]().All(ctx); err != nil {
		t.Fatalf("warm-up select: %v", err)
	}

	before := db.Stats().OpenConnections
	for i := 0; i < 30; i++ {
		if _, err := Select[User]().Page(ctx, 1, 10); err != nil {
			t.Fatalf("page %d: %v", i, err)
		}
	}

	leaked := db.Stats().OpenConnections - before
	if leaked > 5 {
		t.Fatalf("Page leaked %d connections across 30 calls (before=%d, after=%d)",
			leaked, before, db.Stats().OpenConnections)
	}
}
