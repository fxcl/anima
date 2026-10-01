package anima

import "testing"

// FuzzParseTag pins the parser's robustness contract: any tag text is accepted
// without panicking. Quote characters in a tag are safe because every column
// that reaches SQL goes through Quote (verified separately by
// FuzzQuoteRoundTrip).
func FuzzParseTag(f *testing.F) {
	f.Add("column:full_name,pk")
	f.Add("pk")
	f.Add("column:")
	f.Add(",,,,")
	f.Add("column:a`b")
	f.Add(`column:x"y`)
	f.Add("")
	f.Fuzz(func(t *testing.T, tag string) {
		col, pk := parseTag(tag)
		if pk && col == "" && tag == "" {
			t.Fatal("empty tag must not claim a primary key")
		}
	})
}
