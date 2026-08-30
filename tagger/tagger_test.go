package tagger

import "testing"

func tagMode(s string, m Mode) string {
	cfg := DefaultConfig()
	cfg.Mode = m
	return string(Tag([]byte(s), cfg))
}

// markersCases is the canonical positive/negative table (markers mode). Outputs
// were verified against the tagger; "(unchanged)" cases assert the input is
// returned verbatim.
var markersCases = []struct {
	name string
	in   string
	out  string
}{
	// positive — should tag
	{"plain english", "Plain English only.", "Plain English only."},
	{"single han run", "Rescued me from 桃園 at midnight.", "Rescued me from {{zh}}桃園{{/zh}} at midnight."},
	{"run at start", "台灣 is an island.", "{{zh}}台灣{{/zh}} is an island."},
	{"run at end", "An island: 台灣", "An island: {{zh}}台灣{{/zh}}"},
	{"whole string han", "台灣", "{{zh}}台灣{{/zh}}"},
	{"two separate runs", "from 台北 to 高雄 today", "from {{zh}}台北{{/zh}} to {{zh}}高雄{{/zh}} today"},
	{"join fullwidth comma", "spices 五香粉，黑胡椒 today", "spices {{zh}}五香粉，黑胡椒{{/zh}} today"},
	{"interior digits", "在2013年8月時", "{{zh}}在2013年8月時{{/zh}}"},
	{"leading digit no space", "Date 8月8日 here.", "Date {{zh}}8月8日{{/zh}} here."},
	{"leading digit space", "Date 8 月 here.", "Date {{zh}}8 月{{/zh}} here."},
	{"digit-space-han", "有 3 個蘋果", "{{zh}}有 3 個蘋果{{/zh}}"},
	{"trailing period excluded", "去台灣。Next", "{{zh}}去台灣{{/zh}}。Next"},
	{"brackets excluded (han)", "「保生大帝」here", "「{{zh}}保生大帝{{/zh}}」here"},
	{"hiragana ja", "こんにちは world", "{{ja}}こんにちは{{/ja}} world"},
	{"katakana ja", "カタカナ test", "{{ja}}カタカナ{{/ja}} test"},
	{"han+kana ja", "日本語のテスト done", "{{ja}}日本語のテスト{{/ja}} done"},
	{"link text (url intact)", "See [台灣](/path/somewhere/).", "See [{{zh}}台灣{{/zh}}](/path/somewhere/)."},
	{"emphasis around", "*台灣* rocks", "*{{zh}}台灣{{/zh}}* rocks"},
	{"heading", "# 台灣 heading", "# {{zh}}台灣{{/zh}} heading"},
	{"multi-paragraph", "First 台北\n\nSecond 高雄", "First {{zh}}台北{{/zh}}\n\nSecond {{zh}}高雄{{/zh}}"},
	// ascii before han (left-only) is prose under option (a): TAGGED.
	{"ascii before han (left only)", "x台灣", "x{{zh}}台灣{{/zh}}"},

	// negative — should be left unchanged
	{"punct-only", "。。。 alone", "。。。 alone"},
	{"cjk brackets + english", "「english」word", "「english」word"},
	{"inline code", "Run `台灣` verbatim.", "Run `台灣` verbatim."},
	{"ascii-han-ascii (both sides)", "wifi台灣net", "wifi台灣net"},
	{"scheme url", "see http://台灣.example for more", "see http://台灣.example for more"},
	{"www host", "visit www.台灣today", "visit www.台灣today"},
	{"idn two-label", "go to 例子.測試 now", "go to 例子.測試 now"},
	{"idn cjk.com", "at 台灣.com here", "at 台灣.com here"},
	{"link mirrors url", "[台灣](https://台灣.example/台灣)", "[台灣](https://台灣.example/台灣)"},
	{"idempotent zh", "from {{zh}}桃園{{/zh}} today", "from {{zh}}桃園{{/zh}} today"},
	{"idempotent ja", "say {{ja}}こんにちは{{/ja}} now", "say {{ja}}こんにちは{{/ja}} now"},
}

func TestMarkersMode(t *testing.T) {
	for _, c := range markersCases {
		t.Run(c.name, func(t *testing.T) {
			if got := tagMode(c.in, ModeMarkers); got != c.out {
				t.Errorf("\n in:   %q\n got:  %q\n want: %q", c.in, got, c.out)
			}
		})
	}
}

func TestPassthroughMode(t *testing.T) {
	got := tagMode("CN 台灣 JP こんにちは", ModePassthrough)
	want := "CN <!--lang:zh-->台灣<!--/lang--> JP <!--lang:ja-->こんにちは<!--/lang-->"
	if got != want {
		t.Fatalf("\n got:  %q\n want: %q", got, want)
	}
}

func TestPassthroughKanjiOnlyJaStaysZhByScript(t *testing.T) {
	// A kanji-only run has no kana, so it is classified zh. This is expected:
	// classification is script-based. (The Hugo hook cannot recover ja here
	// either — it is the same information.)
	got := tagMode("x 日本 y", ModePassthrough)
	want := "x <!--lang:zh-->日本<!--/lang--> y"
	if got != want {
		t.Fatalf("\n got:  %q\n want: %q", got, want)
	}
}

func TestSpansMode(t *testing.T) {
	got := tagMode("CN 台灣 JP こんにちは", ModeSpans)
	want := `CN <span lang="zh-Hant-TW">台灣</span> JP <span lang="ja">こんにちは</span>`
	if got != want {
		t.Fatalf("\n got:  %q\n want: %q", got, want)
	}
}

func TestIdempotentEachMode(t *testing.T) {
	for _, m := range ValidModes {
		in := "CN 台灣 and JP こんにちは here"
		once := tagMode(in, m)
		twice := tagMode(once, m)
		if once != twice {
			t.Errorf("mode %s not idempotent:\n once=%q\n twice=%q", m, once, twice)
		}
	}
}

func TestParseMode(t *testing.T) {
	for _, m := range ValidModes {
		if got, err := ParseMode(string(m)); err != nil || got != m {
			t.Errorf("ParseMode(%q) = %q, %v", m, got, err)
		}
	}
	if _, err := ParseMode(""); err == nil {
		t.Error("empty mode should error")
	}
	if _, err := ParseMode("jp"); err == nil {
		t.Error("invalid mode should error")
	}
}

func TestDisableJa(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Mode = ModeMarkers
	cfg.EnableJa = false
	got := string(Tag([]byte("台灣 and こんにちは"), cfg))
	want := "{{zh}}台灣{{/zh}} and こんにちは"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestDisableZh(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Mode = ModeMarkers
	cfg.EnableZh = false
	got := string(Tag([]byte("台灣 and こんにちは"), cfg))
	want := "台灣 and {{ja}}こんにちは{{/ja}}"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestAsciiAdjacentKnob(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Mode = ModeMarkers
	cfg.TagAsciiAdjacent = true
	got := string(Tag([]byte("wifi台灣net"), cfg))
	want := "wifi{{zh}}台灣{{/zh}}net"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func passthroughCfg() Config {
	c := DefaultConfig()
	c.Mode = ModePassthrough
	return c
}

func TestSkipsYAMLFrontMatter(t *testing.T) {
	in := "---\ntitle: 水餃\ntags:\n- 好吃 Food\n---\n\nBody with 台灣 here.\n"
	want := "---\ntitle: 水餃\ntags:\n- 好吃 Food\n---\n\nBody with <!--lang:zh-->台灣<!--/lang--> here.\n"
	if got := string(Tag([]byte(in), passthroughCfg())); got != want {
		t.Fatalf("\n got:  %q\n want: %q", got, want)
	}
}

func TestSkipsTOMLFrontMatter(t *testing.T) {
	in := "+++\ntitle = \"水餃\"\n+++\n\n台灣 body\n"
	want := "+++\ntitle = \"水餃\"\n+++\n\n<!--lang:zh-->台灣<!--/lang--> body\n"
	if got := string(Tag([]byte(in), passthroughCfg())); got != want {
		t.Fatalf("\n got:  %q\n want: %q", got, want)
	}
}

func TestNoFrontMatterUnaffected(t *testing.T) {
	in := "Just 台灣 body, no front matter.\n"
	want := "Just <!--lang:zh-->台灣<!--/lang--> body, no front matter.\n"
	if got := string(Tag([]byte(in), passthroughCfg())); got != want {
		t.Fatalf("\n got:  %q\n want: %q", got, want)
	}
}

// --- CJK prose abutting Latin words (option a) -------------------------------

func TestProseCJKAbuttingLatinIsTagged(t *testing.T) {
	// A Latin proper noun merely abutting CJK is prose, not an identifier:
	// tag the CJK runs, leave the Latin word bare.
	got := tagMode("台灣到Frankfurt沒問題", ModeMarkers)
	want := "{{zh}}台灣到{{/zh}}Frankfurt{{zh}}沒問題{{/zh}}"
	if got != want {
		t.Fatalf("\n got:  %q\n want: %q", got, want)
	}
}

func TestIdentifierGluedBothSidesSkipped(t *testing.T) {
	// CJK glued to ASCII letters on BOTH sides is one identifier-like token.
	if got := tagMode("wifi台灣net", ModeMarkers); got != "wifi台灣net" {
		t.Fatalf("identifier was modified: %q", got)
	}
}

func TestStructuralTokenSkipped(t *testing.T) {
	// A token containing a URL/identifier structural char (. / : @) is skipped.
	for _, in := range []string{"台灣.com", "host/台灣", "user@台灣"} {
		if got := tagMode(in, ModeMarkers); got != in {
			t.Errorf("structural token modified: in=%q got=%q", in, got)
		}
	}
}

func TestLinkTextNotCaughtByURLSlashes(t *testing.T) {
	// The structural-token scan must stop at markdown []() so a link's text is
	// still tagged even though its destination has slashes.
	got := tagMode("See [台灣](/p/q/).", ModeMarkers)
	want := "See [{{zh}}台灣{{/zh}}](/p/q/)."
	if got != want {
		t.Fatalf("\n got:  %q\n want: %q", got, want)
	}
}

func TestMixedProseParagraph(t *testing.T) {
	// The mr-baozi paragraph: CJK tagged, Latin place-names left bare. NOTE a
	// known edge of the "glued both sides" rule: CJK particles sandwiched
	// between two Latin words with no spaces (Frankfurt到Heathrow也OK) are
	// treated as identifier-internal and left untagged.
	in := "台灣到Frankfurt沒問題， Frankfurt到Heathrow也OK，但是在Heathrow有一個大問題！"
	got := tagMode(in, ModePassthrough)
	want := "<!--lang:zh-->台灣到<!--/lang-->Frankfurt<!--lang:zh-->沒問題<!--/lang-->， Frankfurt到Heathrow也OK，<!--lang:zh-->但是在<!--/lang-->Heathrow<!--lang:zh-->有一個大問題<!--/lang-->！"
	if got != want {
		t.Fatalf("\n got:  %q\n want: %q", got, want)
	}
}

func TestAlreadyMarkedContentIsNoOp(t *testing.T) {
	// Already-wrapped content is an HTML comment block (RawHTML to goldmark);
	// the walker skips it, so a second pass changes nothing.
	in := "<!--lang:zh-->在台灣認識朋友。台灣到Frankfurt沒問題！<!--/lang-->"
	if got := tagMode(in, ModePassthrough); got != in {
		t.Fatalf("already-marked content was modified:\n in:  %q\n got: %q", in, got)
	}
}

func TestMinimalAlreadyMarkedIsNoOp(t *testing.T) {
	// A single already-wrapped character round-trips unchanged (RawHTML block).
	in := "<!--lang:zh-->愛<!--/lang-->"
	for _, m := range ValidModes {
		if got := tagMode(in, m); got != in {
			t.Errorf("mode %s modified marked content:\n in:  %q\n got: %q", m, in, got)
		}
	}
}

func TestFencedCodeUntouched(t *testing.T) {
	in := "```\n台灣\n```\n"
	if got := tagMode(in, ModeMarkers); got != in {
		t.Fatalf("fenced code was modified: %q", got)
	}
}
