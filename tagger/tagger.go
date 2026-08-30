// Package tagger wraps runs of Chinese and Japanese script in a Markdown
// document with language markers, operating on the Markdown AST so that code
// spans, link destinations, autolinks and raw HTML are never touched.
//
// A run of pure Han is classified "zh"; a run containing any kana
// (Hiragana/Katakana) is classified "ja". The output form is chosen by Mode:
//
//	markers      {{zh}}台灣{{/zh}}                 {{ja}}こんにちは{{/ja}}
//	passthrough  <!--lang:zh-->台灣<!--/lang-->     <!--lang:ja-->こんにちは<!--/lang-->
//	spans        <span lang="zh-Hant-TW">台灣</span> <span lang="ja">こんにちは</span>
//
// The passthrough form is a valid HTML comment pair: it degrades to invisible
// comments if the consuming renderer has no handler, and it carries the
// language code inside the delimited text so a single Hugo render-passthrough
// hook can route every language.
package tagger

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// Mode selects the output wrapping form.
type Mode string

const (
	ModeMarkers     Mode = "markers"
	ModePassthrough Mode = "passthrough"
	ModeSpans       Mode = "spans"
)

// ValidModes lists the accepted -mode values.
var ValidModes = []Mode{ModeMarkers, ModePassthrough, ModeSpans}

// ParseMode validates a mode string.
func ParseMode(s string) (Mode, error) {
	for _, m := range ValidModes {
		if string(m) == s {
			return m, nil
		}
	}
	return "", fmt.Errorf("invalid mode %q (want one of: markers, passthrough, spans)", s)
}

// langCode is the internal classification of a run.
type langCode string

const (
	zh langCode = "zh"
	ja langCode = "ja"
)

// htmlLang maps an internal code to the BCP-47 tag used in spans mode.
var htmlLang = map[langCode]string{zh: "zh-Hant-TW", ja: "ja"}

// Config controls detection and output.
type Config struct {
	// Mode is the output form. Required — there is no default.
	Mode Mode

	// EnableZh / EnableJa select which run classes are tagged. Both default
	// to true via DefaultConfig.
	EnableZh, EnableJa bool

	// TagAsciiAdjacent, when true, tags a run glued to ASCII letters with no
	// separating space (e.g. "wifi台灣net"). Default false.
	TagAsciiAdjacent bool

	// IDNSecondLabelMax is the max glyph length of the other dotted label for
	// a "<cjk>.<cjk>" sequence to be treated as an IDN host and left untagged.
	IDNSecondLabelMax int
}

// DefaultConfig enables both languages, leaves ascii-adjacent runs as-is, and
// treats a "<cjk>.<cjk≤3>" sequence as an IDN host. Mode must be set by the
// caller.
func DefaultConfig() Config {
	return Config{
		EnableZh:          true,
		EnableJa:          true,
		TagAsciiAdjacent:  false,
		IDNSecondLabelMax: 3,
	}
}

// open/close returns the wrapping strings for a code under the config's mode.
func (c Config) wrap(code langCode) (open, close string) {
	switch c.Mode {
	case ModeMarkers:
		return fmt.Sprintf("{{%s}}", code), fmt.Sprintf("{{/%s}}", code)
	case ModePassthrough:
		return fmt.Sprintf("<!--lang:%s-->", code), "<!--/lang-->"
	case ModeSpans:
		return fmt.Sprintf(`<span lang="%s">`, htmlLang[code]), "</span>"
	}
	return "", ""
}

// --- script tables -----------------------------------------------------------

var scripts = []*unicode.RangeTable{unicode.Han, unicode.Hiragana, unicode.Katakana}
var kana = []*unicode.RangeTable{unicode.Hiragana, unicode.Katakana}

var interiorPunct = &unicode.RangeTable{
	R16: []unicode.Range16{
		{Lo: 0x3001, Hi: 0x3003, Stride: 1}, // 、 。 〃
		{Lo: 0x3008, Hi: 0x3011, Stride: 1}, // 〈〉《》「」『』【】
		{Lo: 0x30fb, Hi: 0x30fb, Stride: 1}, // ・
		{Lo: 0xff01, Hi: 0xff0f, Stride: 1}, // fullwidth ! " # $ % & ' ( ) * + , - . /
		{Lo: 0xff1a, Hi: 0xff1b, Stride: 1}, // ： ；
		{Lo: 0xff1f, Hi: 0xff1f, Stride: 1}, // ？
	},
}

var digits = &unicode.RangeTable{
	R16: []unicode.Range16{
		{Lo: 0x0030, Hi: 0x0039, Stride: 1},
		{Lo: 0xff10, Hi: 0xff19, Stride: 1},
	},
}

func isScript(r rune) bool { return inAny(r, scripts) }
func isKana(r rune) bool   { return inAny(r, kana) }
func isDigit(r rune) bool  { return unicode.Is(digits, r) }

func isConnector(r rune) bool {
	return isDigit(r) || r == ' ' || unicode.Is(interiorPunct, r)
}

func inAny(r rune, tables []*unicode.RangeTable) bool {
	for _, t := range tables {
		if unicode.Is(t, r) {
			return true
		}
	}
	return false
}

// --- edits -------------------------------------------------------------------

type edit struct {
	at  int
	str string
}

// Tag returns src with every qualifying run wrapped per cfg.Mode. Idempotent
// for the active mode: a run already wrapped in that mode's delimiters is left
// untouched.
func Tag(src []byte, cfg Config) []byte {
	if cfg.IDNSecondLabelMax == 0 {
		cfg.IDNSecondLabelMax = 3
	}
	md := goldmark.New()
	doc := md.Parser().Parse(text.NewReader(src))

	var edits []edit
	ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch t := n.(type) {
		case *ast.CodeSpan, *ast.RawHTML, *ast.AutoLink:
			return ast.WalkSkipChildren, nil
		case *ast.Link:
			if linkTextMirrorsURL(src, t) {
				return ast.WalkSkipChildren, nil
			}
		}
		tn, ok := n.(*ast.Text)
		if !ok {
			return ast.WalkContinue, nil
		}
		seg := tn.Segment
		collectRuns(src, seg.Start, seg.Stop, cfg, &edits)
		return ast.WalkContinue, nil
	})

	if len(edits) == 0 {
		return src
	}
	return applyEdits(src, edits)
}

func collectRuns(src []byte, start, stop int, cfg Config, edits *[]edit) {
	i := start
	for i < stop {
		r, size := utf8.DecodeRune(src[i:stop])
		if size == 0 {
			break
		}
		startsRun := isScript(r) || (isDigit(r) && digitLeadsToScript(src, i, stop))
		if !startsRun {
			i += size
			continue
		}
		runStart := i
		lastScriptEnd := -1
		hasKana := false
		j := i
		for j < stop {
			rr, sz := utf8.DecodeRune(src[j:stop])
			if sz == 0 {
				break
			}
			if isScript(rr) {
				if isKana(rr) {
					hasKana = true
				}
				j += sz
				lastScriptEnd = j
				continue
			}
			if isConnector(rr) {
				j += sz
				continue
			}
			break
		}
		if lastScriptEnd < 0 {
			i += size
			continue
		}
		runEnd := lastScriptEnd

		if shouldSkip(src, runStart, runEnd, cfg) {
			i = runEnd
			continue
		}
		code := zh
		if hasKana {
			code = ja
		}
		if (code == zh && !cfg.EnableZh) || (code == ja && !cfg.EnableJa) {
			i = runEnd
			continue
		}
		open, close := cfg.wrap(code)
		if !alreadyWrapped(src, runStart, runEnd, open, close) {
			*edits = append(*edits, edit{at: runStart, str: open})
			*edits = append(*edits, edit{at: runEnd, str: close})
		}
		i = runEnd
	}
}

func digitLeadsToScript(src []byte, i, stop int) bool {
	j := i
	for j < stop {
		r, sz := utf8.DecodeRune(src[j:stop])
		if sz == 0 {
			return false
		}
		if isDigit(r) || r == ' ' {
			j += sz
			continue
		}
		return isScript(r)
	}
	return false
}

func shouldSkip(src []byte, start, end int, cfg Config) bool {
	if start >= 3 && string(src[start-3:start]) == "://" {
		return true
	}
	if start >= 4 && strings.EqualFold(string(src[start-4:start]), "www.") {
		return true
	}
	if !cfg.TagAsciiAdjacent && (asciiLetterBefore(src, start) || asciiLetterAfter(src, end)) {
		return true
	}
	if idnForward(src, start, end, cfg.IDNSecondLabelMax) || idnBackward(src, start, cfg.IDNSecondLabelMax) {
		return true
	}
	return false
}

func asciiLetterBefore(src []byte, start int) bool {
	if start == 0 {
		return false
	}
	r, _ := utf8.DecodeLastRune(src[:start])
	return isASCIILetter(r)
}

func asciiLetterAfter(src []byte, end int) bool {
	if end >= len(src) {
		return false
	}
	r, _ := utf8.DecodeRune(src[end:])
	return isASCIILetter(r)
}

func isASCIILetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func idnForward(src []byte, start, end, max int) bool {
	if end >= len(src) || src[end] != '.' {
		return false
	}
	j := end + 1
	glyphs := 0
	for j < len(src) {
		r, sz := utf8.DecodeRune(src[j:])
		if sz == 0 {
			break
		}
		if isScript(r) || isASCIILetter(r) || isDigit(r) {
			glyphs++
			j += sz
			continue
		}
		break
	}
	return glyphs >= 1 && glyphs <= max
}

func idnBackward(src []byte, start, max int) bool {
	if start == 0 || src[start-1] != '.' {
		return false
	}
	glyphs := 0
	k := start - 1
	for k > 0 {
		r, sz := utf8.DecodeLastRune(src[:k])
		if sz == 0 {
			break
		}
		if isScript(r) || isASCIILetter(r) || isDigit(r) {
			glyphs++
			k -= sz
			continue
		}
		break
	}
	return glyphs >= 1 && glyphs <= max
}

func linkTextMirrorsURL(src []byte, l *ast.Link) bool {
	var txt strings.Builder
	for c := l.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			txt.Write(src[t.Segment.Start:t.Segment.Stop])
		}
	}
	t := strings.TrimSpace(txt.String())
	if t == "" {
		return false
	}
	return strings.Contains(string(l.Destination), t)
}

func alreadyWrapped(src []byte, start, end int, open, close string) bool {
	pre := start - len(open)
	if pre >= 0 && string(src[pre:start]) == open {
		if end+len(close) <= len(src) && string(src[end:end+len(close)]) == close {
			return true
		}
	}
	return false
}

func applyEdits(src []byte, edits []edit) []byte {
	sort.SliceStable(edits, func(a, b int) bool { return edits[a].at < edits[b].at })
	var b strings.Builder
	b.Grow(len(src) + len(edits)*12)
	prev := 0
	for _, e := range edits {
		b.Write(src[prev:e.at])
		b.WriteString(e.str)
		prev = e.at
	}
	b.Write(src[prev:])
	return []byte(b.String())
}
