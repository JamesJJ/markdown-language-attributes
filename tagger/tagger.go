// Package tagger wraps runs of CJK/Japanese script in a Markdown document with
// language delimiters, operating on the Markdown AST so that code spans, link
// destinations, autolinks and raw HTML are never touched.
//
// A run of pure Han is tagged with the "zh" delimiters; a run containing any
// kana (Hiragana/Katakana) is tagged with the "jp" delimiters. Delimiters,
// scripts and several heuristics are configurable.
package tagger

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// Config controls detection and wrapping.
type Config struct {
	// ZhOpen/ZhClose wrap a run of pure Han (Chinese).
	ZhOpen, ZhClose string
	// JpOpen/JpClose wrap a run that contains any kana (Japanese).
	JpOpen, JpClose string

	// EnableZh / EnableJp select which run classes are tagged. Both default
	// to true. Disabling one leaves those runs untouched (e.g. EnableJp=false
	// tags only pure-Han runs; kana-containing runs are left as-is).
	EnableZh, EnableJp bool

	// TagAsciiAdjacent, when true, tags a CJK run even when it is glued to
	// ASCII letters with no separating space (e.g. "wifi台灣net"). Default
	// false: such runs are left as-is (likely identifiers/slugs).
	TagAsciiAdjacent bool

	// IDNSecondLabelMax is the maximum glyph length of the SECOND dotted
	// label for a "<cjk>.<cjk>" sequence to be treated as an IDN host and
	// left untagged (e.g. 例子.測試). Default 3.
	IDNSecondLabelMax int
}

// DefaultConfig tags Han→{{zh}} and kana→{{jp}}, leaves ascii-adjacent runs
// as-is, and treats a "<cjk>.<cjk≤3>" sequence as an IDN host.
func DefaultConfig() Config {
	return Config{
		ZhOpen: "{{zh}}", ZhClose: "{{/zh}}",
		JpOpen: "{{jp}}", JpClose: "{{/jp}}",
		EnableZh:          true,
		EnableJp:          true,
		TagAsciiAdjacent:  false,
		IDNSecondLabelMax: 3,
	}
}

// --- script tables -----------------------------------------------------------

var scripts = []*unicode.RangeTable{unicode.Han, unicode.Hiragana, unicode.Katakana}
var kana = []*unicode.RangeTable{unicode.Hiragana, unicode.Katakana}

// interiorPunct is punctuation permitted BETWEEN two script runes without
// breaking a run. R16 ranges must be sorted ascending by Lo.
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
		{Lo: 0x0030, Hi: 0x0039, Stride: 1}, // 0-9
		{Lo: 0xff10, Hi: 0xff19, Stride: 1}, // fullwidth 0-9
	},
}

func isScript(r rune) bool { return inAny(r, scripts) }
func isKana(r rune) bool   { return inAny(r, kana) }
func isDigit(r rune) bool  { return unicode.Is(digits, r) }
func isInteriorPunct(r rune) bool {
	return unicode.Is(interiorPunct, r)
}

// isConnector: a rune allowed INTERIOR to a run (between script runes): a
// digit, a space, or interior CJK punctuation.
func isConnector(r rune) bool {
	return isDigit(r) || r == ' ' || isInteriorPunct(r)
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

// Tag returns src with every qualifying script run wrapped in the configured
// language delimiters. Idempotent: a run already enclosed by its delimiters is
// left untouched.
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
			// If the link text is a verbatim substring of the destination,
			// treat the text as URL-mirroring and skip the whole subtree.
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

// collectRuns scans src[start:stop] for qualifying runs.
func collectRuns(src []byte, start, stop int, cfg Config, edits *[]edit) {
	i := start
	for i < stop {
		r, size := utf8.DecodeRune(src[i:stop])
		if size == 0 {
			break
		}
		// A run may begin on a script rune, or on a digit that is immediately
		// (across spaces) followed by a script rune within the run.
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
		if lastScriptEnd < 0 { // no script rune actually seen (shouldn't happen)
			i += size
			continue
		}
		runEnd := lastScriptEnd // trim any trailing connectors

		if shouldSkip(src, runStart, runEnd, cfg) {
			i = runEnd
			continue
		}
		open, close_ := cfg.ZhOpen, cfg.ZhClose
		if hasKana {
			if !cfg.EnableJp {
				i = runEnd
				continue
			}
			open, close_ = cfg.JpOpen, cfg.JpClose
		} else if !cfg.EnableZh {
			i = runEnd
			continue
		}
		if !alreadyWrapped(src, runStart, runEnd, open, close_) {
			*edits = append(*edits, edit{at: runStart, str: open})
			*edits = append(*edits, edit{at: runEnd, str: close_})
		}
		i = runEnd
	}
}

// digitLeadsToScript reports whether the digits/spaces starting at i reach a
// script rune before any other character. Lets "8月" / "8 月" start a run.
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

// shouldSkip applies the URL/IDN/ascii-adjacent heuristics.
func shouldSkip(src []byte, start, end int, cfg Config) bool {
	// URL scheme: run touches "://" just before it.
	if start >= 3 && string(src[start-3:start]) == "://" {
		return true
	}
	// "www." immediately before the run.
	if start >= 4 && strings.EqualFold(string(src[start-4:start]), "www.") {
		return true
	}
	// ascii-han-ascii: an ASCII letter immediately adjacent on either side,
	// unless explicitly enabled.
	if !cfg.TagAsciiAdjacent && (asciiLetterBefore(src, start) || asciiLetterAfter(src, end)) {
		return true
	}
	// IDN host: skip if the run is the label on EITHER side of a dot in a
	// "<label>.<label>" host, where the OTHER label is short (≤max glyphs).
	//   forward:  <run>.<shortlabel>   e.g. 例子.測試 , 台灣.com
	//   backward: <shortlabel>.<run>   e.g. 例子.測試 (the 測試 side)
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

// idnForward reports whether the run at [start:end] is followed by ".<label>"
// where label is a short glyph sequence (≤max glyphs) — a likely IDN host with
// this run as the FIRST label.
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

// idnBackward reports whether the run at start is immediately preceded by
// "<label>." where label is a short glyph sequence (≤max glyphs) — a likely IDN
// host with this run as a later label.
func idnBackward(src []byte, start, max int) bool {
	if start == 0 || src[start-1] != '.' {
		return false
	}
	// walk back over the label before the dot
	j := start - 1 // at the dot
	glyphs := 0
	k := j
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

// linkTextMirrorsURL reports whether the link's rendered text is a verbatim
// substring of its destination URL.
func linkTextMirrorsURL(src []byte, l *ast.Link) bool {
	var txt strings.Builder
	for c := l.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			txt.Write(src[t.Segment.Start:t.Segment.Stop])
		}
	}
	text := strings.TrimSpace(txt.String())
	if text == "" {
		return false
	}
	return strings.Contains(string(l.Destination), text)
}

func alreadyWrapped(src []byte, start, end int, open, close_ string) bool {
	pre := start - len(open)
	if pre >= 0 && string(src[pre:start]) == open {
		if end+len(close_) <= len(src) && string(src[end:end+len(close_)]) == close_ {
			return true
		}
	}
	return false
}

func applyEdits(src []byte, edits []edit) []byte {
	sort.SliceStable(edits, func(a, b int) bool { return edits[a].at < edits[b].at })
	var b strings.Builder
	b.Grow(len(src) + len(edits)*8)
	prev := 0
	for _, e := range edits {
		b.Write(src[prev:e.at])
		b.WriteString(e.str)
		prev = e.at
	}
	b.Write(src[prev:])
	return []byte(b.String())
}
