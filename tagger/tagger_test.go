package tagger

import "testing"

func tagMode(s string, m Mode) string {
	cfg := DefaultConfig()
	cfg.Mode = m
	return string(Tag([]byte(s), cfg))
}

// markerCase gives the expected output in each mode for one input. The zh/ja
// wrappers differ per mode; cases express the input and the run boundaries via
// the markers mode, then the other modes are derived by re-wrapping.
var markersCases = []struct {
	name string
	in   string
	out  string // markers mode
}{
	{"plain english", "Plain English only.", "Plain English only."},
	{"single han run", "from 桃園 today", "from {{zh}}桃園{{/zh}} today"},
	{"run at start", "台灣 is home", "{{zh}}台灣{{/zh}} is home"},
	{"join fullwidth comma", "spices 五香粉，黑胡椒 today", "spices {{zh}}五香粉，黑胡椒{{/zh}} today"},
	{"interior digits", "在2013年8月時", "{{zh}}在2013年8月時{{/zh}}"},
	{"leading digit joins", "Date 8月8日 here", "Date {{zh}}8月8日{{/zh}} here"},
	{"leading digit space", "Date 8 月 here", "Date {{zh}}8 月{{/zh}} here"},
	{"trailing period excluded", "去台灣。Next", "{{zh}}去台灣{{/zh}}。Next"},
	{"brackets excluded", "「保生大帝」x", "「{{zh}}保生大帝{{/zh}}」x"},
	{"cjk brackets around english", "「english」x", "「english」x"},
	{"hiragana → ja", "こんにちは world", "{{ja}}こんにちは{{/ja}} world"},
	{"katakana → ja", "カタカナ test", "{{ja}}カタカナ{{/ja}} test"},
	{"han+kana → ja", "日本語のテスト done", "{{ja}}日本語のテスト{{/ja}} done"},
	{"inline code untouched", "Run `台灣` x", "Run `台灣` x"},
	{"link text tagged url not", "See [台灣](/p/q/).", "See [{{zh}}台灣{{/zh}}](/p/q/)."},
	{"ascii-adjacent as-is", "wifi台灣net", "wifi台灣net"},
	{"scheme url as-is", "see http://台灣.example here", "see http://台灣.example here"},
	{"www host as-is", "visit www.台灣today", "visit www.台灣today"},
	{"idn two-label as-is", "go 例子.測試 now", "go 例子.測試 now"},
	{"idn cjk dot com as-is", "at 台灣.com here", "at 台灣.com here"},
	{"link mirrors url as-is", "[台灣](https://台灣.example/台灣)", "[台灣](https://台灣.example/台灣)"},
	{"multiline", "First 台北\n\nSecond 高雄", "First {{zh}}台北{{/zh}}\n\nSecond {{zh}}高雄{{/zh}}"},
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
