package tagger

import "testing"

func tag(s string) string { return string(Tag([]byte(s), DefaultConfig())) }

// cases is the canonical input→output table for the default config.
var cases = []struct {
	name string
	in   string
	out  string
}{
	// basics
	{"plain english", "Plain English only.", "Plain English only."},
	{"single han run", "Rescued me from 桃園 at midnight.", "Rescued me from {{zh}}桃園{{/zh}} at midnight."},
	{"run at start", "台灣 is an island.", "{{zh}}台灣{{/zh}} is an island."},
	{"run at end", "An island: 台灣", "An island: {{zh}}台灣{{/zh}}"},
	{"whole string han", "台灣", "{{zh}}台灣{{/zh}}"},
	{"two separate runs", "from 台北 to 高雄 today", "from {{zh}}台北{{/zh}} to {{zh}}高雄{{/zh}} today"},

	// joining across interior punctuation / digits / spaces
	{"join fullwidth comma", "spices 五香粉，黑胡椒 today", "spices {{zh}}五香粉，黑胡椒{{/zh}} today"},
	{"interior digits", "在2013年8月時", "{{zh}}在2013年8月時{{/zh}}"},
	{"leading digit joins (no space)", "Date 8月8日 here.", "Date {{zh}}8月8日{{/zh}} here."},
	{"leading digit joins (space)", "Date 8 月 here.", "Date {{zh}}8 月{{/zh}} here."},
	{"digit-space-han run", "有 3 個蘋果", "{{zh}}有 3 個蘋果{{/zh}}"},

	// boundary trimming: leading/trailing punctuation excluded
	{"trailing period excluded", "去台灣。Next", "{{zh}}去台灣{{/zh}}。Next"},
	{"brackets excluded around han", "「保生大帝」here", "「{{zh}}保生大帝{{/zh}}」here"},
	{"punct-only no run", "。。。 alone", "。。。 alone"},

	// CJK brackets wrapping non-CJK content → nothing to tag
	{"cjk brackets around english", "「english」word", "「english」word"},

	// script → tag selection (zh vs jp)
	{"hiragana → jp", "こんにちは world", "{{jp}}こんにちは{{/jp}} world"},
	{"katakana → jp", "カタカナ test", "{{jp}}カタカナ{{/jp}} test"},
	{"han+kana → jp", "日本語のテスト done", "{{jp}}日本語のテスト{{/jp}} done"},

	// markdown structure exclusions
	{"inline code untouched", "Run `台灣` verbatim.", "Run `台灣` verbatim."},
	{"fenced code untouched", "```\n台灣\n```\n", "```\n台灣\n```\n"},
	{"link text tagged, url not", "See [台灣](/path/somewhere/).", "See [{{zh}}台灣{{/zh}}](/path/somewhere/)."},
	{"emphasis around run", "*台灣* rocks", "*{{zh}}台灣{{/zh}}* rocks"},
	{"heading", "# 台灣 heading", "# {{zh}}台灣{{/zh}} heading"},

	// ascii-adjacent left as-is by default
	{"ascii-han-ascii as-is", "wifi台灣net", "wifi台灣net"},
	{"ascii before han as-is", "x台灣", "x台灣"},

	// URL / IDN heuristics → as-is
	{"scheme url as-is", "see http://台灣.example for more", "see http://台灣.example for more"},
	{"www host as-is", "visit www.台灣today", "visit www.台灣today"},
	{"idn two-label host as-is", "go to 例子.測試 now", "go to 例子.測試 now"},
	{"idn cjk dot com as-is", "at 台灣.com here", "at 台灣.com here"},
	{"link text mirrors url as-is", "[台灣](https://台灣.example/台灣)", "[台灣](https://台灣.example/台灣)"},

	// idempotency sample
	{"already wrapped zh", "from {{zh}}桃園{{/zh}} today", "from {{zh}}桃園{{/zh}} today"},
	{"already wrapped jp", "say {{jp}}こんにちは{{/jp}} now", "say {{jp}}こんにちは{{/jp}} now"},

	// multiline
	{"multiple paragraphs", "First 台北\n\nSecond 高雄", "First {{zh}}台北{{/zh}}\n\nSecond {{zh}}高雄{{/zh}}"},
}

func TestTable(t *testing.T) {
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := tag(c.in); got != c.out {
				t.Errorf("\n in:   %q\n got:  %q\n want: %q", c.in, got, c.out)
			}
		})
	}
}

func TestIdempotentOverTable(t *testing.T) {
	for _, c := range cases {
		once := tag(c.in)
		if twice := tag(once); once != twice {
			t.Errorf("%s not idempotent:\n once=%q\n twice=%q", c.name, once, twice)
		}
	}
}

func TestAsciiAdjacentKnob(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TagAsciiAdjacent = true
	got := string(Tag([]byte("wifi台灣net"), cfg))
	want := "wifi{{zh}}台灣{{/zh}}net"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestDisableJp(t *testing.T) {
	cfg := DefaultConfig()
	cfg.EnableJp = false
	// Chinese still tagged; Japanese left as-is.
	got := string(Tag([]byte("台灣 and こんにちは"), cfg))
	want := "{{zh}}台灣{{/zh}} and こんにちは"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestDisableZh(t *testing.T) {
	cfg := DefaultConfig()
	cfg.EnableZh = false
	// Japanese still tagged; pure-Han left as-is.
	got := string(Tag([]byte("台灣 and こんにちは"), cfg))
	want := "台灣 and {{jp}}こんにちは{{/jp}}"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestCustomDelimiters(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ZhOpen, cfg.ZhClose = "[[zh]]", "[[/zh]]"
	got := string(Tag([]byte("from 台灣 here"), cfg))
	want := "from [[zh]]台灣[[/zh]] here"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
