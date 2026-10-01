package anima

import (
	"regexp"
	"strings"
)

// English singular-to-plural inflection engine
// (based on Damian Conway's "An Algorithmic Approach to English
// Pluralization").

type pluralRule interface {
	// apply returns the inflected word; ok=false means the rule did not match
	apply(word string) (string, bool)
}

type regexRule struct {
	re   *regexp.Regexp
	repl string
}

func (r regexRule) apply(word string) (string, bool) {
	// ExpandString requires a SubmatchIndex containing capture groups,
	// otherwise $1 expands to the empty string.
	loc := r.re.FindStringSubmatchIndex(word)
	if loc == nil {
		return "", false
	}
	// FindStringSubmatchIndex + ExpandString: replace the first match only
	return word[:loc[0]] + string(r.re.ExpandString(nil, r.repl, word, loc)) + word[loc[1]:], true
}

type listRule struct {
	list     []string
	singular string
	plural   string
}

func (r listRule) apply(word string) (string, bool) {
	lower := strings.ToLower(word)
	for _, suffix := range r.list {
		if strings.HasSuffix(lower, suffix) {
			// A list suffix longer than the declared singular suffix means the
			// rule data is inconsistent — the singular form wouldn't actually
			// match what we just keyed on, so the safe behaviour is to skip
			// this rule and let the next one (or the trailing "$" -> "s"
			// fallback) handle the word. A library must never panic on user
			// input or on a misconfigured rule table.
			if !strings.HasSuffix(lower, r.singular) {
				return "", false
			}
			return word[:len(word)-len(r.singular)] + r.plural, true
		}
	}
	return "", false
}

type inflector struct{ rules []pluralRule }

// EnglishMode selects the pluralization rule set (Java English.MODE).
type EnglishMode int

const (
	// EnglishAnglicized uses the everyday plurals: octopuses, brothers, moneys.
	EnglishAnglicized EnglishMode = iota
	// EnglishClassical keeps the Latin and Greek plurals: octopodes, brethren, monies.
	EnglishClassical
)

// English returns an inflector in the given mode (Java new English(MODE)).
// The concrete type is unexported; callers use its exported Plural method.
func English(mode EnglishMode) *inflector {
	return newEnglishInflector(mode == EnglishClassical)
}

// Plural returns the plural form of word under this inflector's mode
// (Java English.plural).
func (f *inflector) Plural(word string) string { return f.getPlural(word) }

// PluralCount returns word itself when count == 1, otherwise the plural form
// (Java English.plural(word, count)).
func (f *inflector) PluralCount(word string, count int) string {
	if count == 1 {
		return word
	}
	return f.Plural(word)
}

func (f *inflector) getPlural(word string) string {
	for _, r := range f.rules {
		if result, ok := r.apply(word); ok {
			return result
		}
	}
	return ""
}

func (f *inflector) uncountable(list []string) {
	f.rules = append(f.rules, listRule{list: list})
}

// irregular only accepts pairs sharing the same first letter; the
// (?i)(first letter) capture group preserves the input word's leading case.
func (f *inflector) irregular(singular, plural string) {
	re := regexp.MustCompile(`(?i)(` + singular[:1] + `)` + singular[1:] + `$`)
	f.rules = append(f.rules, regexRule{re: re, repl: "${1}" + plural[1:]})
}

func (f *inflector) irregularPairs(pairs [][2]string) {
	for _, p := range pairs {
		f.irregular(p[0], p[1])
	}
}

// addRule compiles a case-insensitive regex rule.
func (f *inflector) addRule(singular, plural string) {
	f.rules = append(f.rules, regexRule{re: regexp.MustCompile(`(?i)` + singular), repl: plural})
}

func (f *inflector) addRules(pairs [][2]string) {
	for _, p := range pairs {
		f.addRule(p[0], p[1])
	}
}

func (f *inflector) categoryRule(list []string, singular, plural string) {
	f.rules = append(f.rules, listRule{list: list, singular: singular, plural: plural})
}

var (
	categoryExIces  = []string{"codex", "murex", "silex"}
	categoryIxIces  = []string{"radix", "helix"}
	categoryUmA     = []string{"bacterium", "agendum", "desideratum", "erratum", "stratum", "datum", "ovum", "extremum", "candelabrum"}
	categoryUsI     = []string{"alumnus", "alveolus", "bacillus", "bronchus", "locus", "nucleus", "stimulus", "meniscus", "thesaurus"}
	categoryOnA     = []string{"criterion", "perihelion", "aphelion", "phenomenon", "prolegomenon", "noumenon", "organon", "asyndeton", "hyperbaton"}
	categoryAAe     = []string{"alumna", "alga", "vertebra", "persona"}
	categoryOOs     = []string{"albino", "archipelago", "armadillo", "commando", "crescendo", "fiasco", "ditto", "dynamo", "embryo", "ghetto", "guano", "inferno", "jumbo", "lumbago", "magneto", "manifesto", "medico", "octavo", "photo", "pro", "quarto", "canto", "lingo", "generalissimo", "stylo", "rhino", "casino", "auto", "macro", "zero", "todo"}
	categoryOI      = []string{"solo", "soprano", "basso", "alto", "contralto", "tempo", "piano", "virtuoso"}
	categoryEnIna   = []string{"stamen", "foramen", "lumen"}
	categoryAAta    = []string{"anathema", "enema", "oedema", "bema", "enigma", "sarcoma", "carcinoma", "gumma", "schema", "charisma", "lemma", "soma", "diploma", "lymphoma", "stigma", "dogma", "magma", "stoma", "drama", "melisma", "trauma", "edema", "miasma"}
	categoryIsIdes  = []string{"iris", "clitoris"}
	categoryUsUs    = []string{"apparatus", "impetus", "prospectus", "cantus", "nexus", "sinus", "coitus", "plexus", "status", "hiatus"}
	categoryNoneI   = []string{"afreet", "afrit", "efreet"}
	categoryNoneIm  = []string{"cherub", "goy", "seraph"}
	categoryExExes  = []string{"apex", "latex", "vertex", "cortex", "pontifex", "vortex", "index", "simplex"}
	categoryIxIxes  = []string{"appendix"}
	categorySEs     = []string{"acropolis", "chaos", "lens", "aegis", "cosmos", "mantis", "alias", "dais", "marquis", "asbestos", "digitalis", "metropolis", "atlas", "epidermis", "pathos", "bathos", "ethos", "pelvis", "bias", "gas", "polis", "caddis", "glottis", "rhinoceros", "cannabis", "glottis", "sassafras", "canvas", "ibis", "trellis"}
	categoryManMans = []string{"human", "Alabaman", "Bahaman", "Burman", "German", "Hiroshiman", "Liman", "Nakayaman", "Oklahoman", "Panaman", "Selman", "Sonaman", "Tacoman", "Yakiman", "Yokohaman", "Yuman"}

	uncountableList = []string{
		"fish", "ois", "sheep", "deer", "pox", "itis",
		"bison", "flounder", "pliers", "bream",
		"gallows", "proceedings", "breeches", "graffiti", "rabies",
		"britches", "headquarters", "salmon", "carp", "herpes",
		"scissors", "chassis", "high-jinks", "sea-bass", "clippers",
		"homework", "series", "cod", "innings", "shears",
		"contretemps", "jackanapes", "species", "corps", "mackerel",
		"swine", "debris", "measles", "trout", "diabetes", "mews",
		"tuna", "djinn", "mumps", "whiting", "eland", "news",
		"wildebeest", "elk", "pincers", "sugar",
	}
)

func newEnglishInflector(classical bool) *inflector {
	f := &inflector{}
	f.uncountable(uncountableList)
	f.irregularPairs([][2]string{
		{"child", "children"},
		{"ephemeris", "ephemerides"},
		{"mongoose", "mongoose"},
		{"mythos", "mythoi"},
		{"soliloquy", "soliloquies"},
		{"trilby", "trilbys"},
		{"genus", "genera"},
		{"quiz", "quizzes"},
	})
	if classical {
		f.irregularPairs([][2]string{
			{"beef", "beeves"}, {"brother", "brethren"}, {"cos", "kine"},
			{"genie", "genii"}, {"money", "monies"}, {"octopus", "octopodes"}, {"opus", "opera"},
		})
	} else {
		f.irregularPairs([][2]string{
			{"beef", "beefs"}, {"brother", "brothers"}, {"cow", "cows"},
			{"genie", "genies"}, {"money", "moneys"}, {"octopus", "octopuses"}, {"opus", "opuses"},
		})
	}

	f.categoryRule(categoryManMans, "", "s")

	// 5. Irregular inflections for common suffixes
	f.addRules([][2]string{
		{`man$`, "men"},
		{`([lm])ouse$`, "${1}ice"},
		{`tooth$`, "teeth"},
		{`goose$`, "geese"},
		{`foot$`, "feet"},
		{`zoon$`, "zoa"},
		{`([csx])is$`, "${1}es"},
	})

	// 6. Fully assimilated classical inflections
	f.categoryRule(categoryExIces, "ex", "ices")
	f.categoryRule(categoryIxIces, "ix", "ices")
	f.categoryRule(categoryUmA, "um", "a")
	f.categoryRule(categoryOnA, "on", "a")
	f.categoryRule(categoryAAe, "a", "ae")

	// 7. Classical variants (classical mode only)
	if classical {
		f.addRules([][2]string{
			{`trix$`, "trices"},
			{`eau$`, "eaux"},
			{`ieu$`, "ieux"},
			{`(..[iay])nx$`, "${1}nges"},
		})
		f.categoryRule(categoryEnIna, "en", "ina")
		f.categoryRule(categoryAAta, "a", "ata")
		f.categoryRule(categoryIsIdes, "is", "ides")
		f.categoryRule(categoryUsUs, "", "")
		f.categoryRule(categoryOI, "o", "i")
		f.categoryRule(categoryNoneI, "", "i")
		f.categoryRule(categoryNoneIm, "", "im")
		f.categoryRule(categoryExExes, "ex", "ices")
		f.categoryRule(categoryIxIxes, "ix", "ices")
	}

	f.categoryRule(categoryUsI, "us", "i")

	f.addRule(`([cs]h|[zx])$`, "${1}es")
	f.categoryRule(categorySEs, "", "es")
	f.categoryRule(categoryIsIdes, "", "es")
	f.categoryRule(categoryUsUs, "", "es")
	f.addRule(`(us)$`, "${1}es")
	f.categoryRule(categoryAAta, "", "s")

	f.addRules([][2]string{
		{`([cs])h$`, "${1}hes"},
		{`ss$`, "sses"},
	})

	f.addRules([][2]string{
		{`([aeo]l)f$`, "${1}ves"},
		{`([^d]ea)f$`, "${1}ves"},
		{`(ar)f$`, "${1}ves"},
		{`([nlw]i)fe$`, "${1}ves"},
	})

	f.addRules([][2]string{
		{`([aeiou])y$`, "${1}ys"},
		{`y$`, "ies"},
	})

	f.categoryRule(categoryOI, "o", "os")
	f.categoryRule(categoryOOs, "o", "os")
	f.addRule(`([aeiou])o$`, "${1}os")
	f.addRule(`o$`, "oes")

	f.addRule(`ulum`, "ula")

	f.categoryRule(categoryAAta, "", "es")

	f.addRule(`s$`, "ses")
	f.addRule(`$`, "s")
	return f
}

var englishInflector = newEnglishInflector(false)

// plural returns the plural form of a word.
func plural(word string) string {
	return englishInflector.getPlural(word)
}
