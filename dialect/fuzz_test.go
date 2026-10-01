package dialect

import "testing"

// FuzzQuoteRoundTrip pins the escaping property that makes Quote injection-safe:
// whatever identifier goes in, the result must be backtick-delimited with every
// interior backtick doubled, so no input can break out of the quoted token.
func FuzzQuoteRoundTrip(f *testing.F) {
	f.Add("users")
	f.Add("we`ird")
	f.Add("a`b`c")
	f.Add("'; DROP TABLE x")
	f.Add("")
	f.Add("日本語テーブル")
	f.Fuzz(func(t *testing.T, name string) {
		q := Base{}.Quote(name)
		if len(q) < 2 || q[0] != '`' || q[len(q)-1] != '`' {
			t.Fatalf("Quote(%q) = %q is not backtick-delimited", name, q)
		}
		inner := q[1 : len(q)-1]
		for i := 0; i < len(inner); i++ {
			if inner[i] != '`' {
				continue
			}
			if i+1 >= len(inner) || inner[i+1] != '`' {
				t.Fatalf("Quote(%q) = %q has an unescaped backtick at offset %d", name, q, i)
			}
			i++
		}
	})
}
