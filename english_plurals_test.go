package anima

import "testing"

// Full-rule-matrix pin for the English pluralization engine. Every
// expectation below was PROBED from the actual implementation (not derived
// from Conway's rule tables): the port's category lists and rule order
// produce several non-dictionary quirks that are pinned as-is —
//   - status: plain "statuses" but classical "status" (categoryUsUs empty
//     plural wins before the us->es rules in classical mode)
//   - index: plain "indexes" / classical "indices", while codex -> "codices"
//     in BOTH modes (different category memberships)
//   - millennium -> "millenniums" (not in the -a/-ata categories), while
//     datum/bacterium/curriculum assimilate
//   - the `ulum` rule is UNANCHORED: any word containing "ulum" pluralizes
//     as if it ended there
//   - person -> "persons" even in classical mode (no person->people pair)
// The classical variants are only reachable via newEnglishInflector(true);
// the production plural() entry uses the non-classical default.
func TestEnglishPluralRuleMatrix(t *testing.T) {
	cases := []struct{ word, plain, classical string }{
		{"cat", "cats", "cats"},
		{"address", "addresses", "addresses"},
		{"box", "boxes", "boxes"},
		{"quiz", "quizzes", "quizzes"},
		{"category", "categories", "categories"},
		{"status", "statuses", "status"},
		{"virus", "viruses", "viruses"},
		{"genus", "genera", "genera"},
		{"cactus", "cactuses", "cactuses"},
		{"focus", "focuses", "focuses"},
		{"radius", "radiuses", "radiuses"},
		{"syllabus", "syllabuses", "syllabuses"},
		{"alumnus", "alumni", "alumni"},
		{"nucleus", "nuclei", "nuclei"},
		{"child", "children", "children"},
		{"mongoose", "mongoose", "mongoose"},
		{"soliloquy", "soliloquies", "soliloquies"},
		{"trilby", "trilbys", "trilbys"},
		{"tooth", "teeth", "teeth"},
		{"goose", "geese", "geese"},
		{"foot", "feet", "feet"},
		{"toot", "toots", "toots"},
		{"woman", "women", "women"},
		{"axis", "axes", "axes"},
		{"codex", "codices", "codices"},
		{"index", "indexes", "indices"},
		{"matrix", "matrixes", "matrices"},
		{"wolf", "wolves", "wolves"},
		{"calf", "calves", "calves"},
		{"leaf", "leaves", "leaves"},
		{"knife", "knives", "knives"},
		{"life", "lives", "lives"},
		{"wife", "wives", "wives"},
		{"day", "days", "days"},
		{"city", "cities", "cities"},
		{"holey", "holeys", "holeys"},
		{"die", "dies", "dies"},
		{"tie", "ties", "ties"},
		{"hero", "heroes", "heroes"},
		{"photo", "photos", "photos"},
		{"ratio", "ratios", "ratios"},
		{"apollo", "apolloes", "apolloes"},
		{"banjo", "banjoes", "banjoes"},
		{"ulum", "ula", "ula"},
		{"person", "persons", "persons"},
		{"deer", "deer", "deer"},
		{"fish", "fish", "fish"},
		{"sheep", "sheep", "sheep"},
		{"money", "moneys", "monies"},
		{"cow", "cows", "cows"},
		{"octopus", "octopuses", "octopodes"},
		{"opus", "opuses", "opera"},
		{"brother", "brothers", "brethren"},
		{"genie", "genies", "genii"},
		{"beef", "beefs", "beeves"},
		{"criterion", "criteria", "criteria"},
		{"datum", "data", "data"},
		{"medium", "mediums", "mediums"},
		{"phenomenon", "phenomena", "phenomena"},
		{"bacterium", "bacteria", "bacteria"},
		{"curriculum", "curricula", "curricula"},
		{"millennium", "millenniums", "millenniums"},
		{"memorandum", "memorandums", "memorandums"},
		{"seraph", "seraphs", "seraphim"},
		{"tabella", "tabellas", "tabellas"},
	}
	plain := newEnglishInflector(false)
	classical := newEnglishInflector(true)
	for _, c := range cases {
		if got := plain.getPlural(c.word); got != c.plain {
			t.Errorf("plain(%q) = %q, want %q", c.word, got, c.plain)
		}
		if got := classical.getPlural(c.word); got != c.classical {
			t.Errorf("classical(%q) = %q, want %q", c.word, got, c.classical)
		}
	}
	if got := plural("cat"); got != "cats" {
		t.Errorf("plural entry uses the non-classical default: %q", got)
	}
}
