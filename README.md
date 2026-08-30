# Markdown Language Attributes

When a web page mixes languages — for example English prose with Chinese or
Japanese words in it — a screen reader will mispronounce the foreign words
unless each one is marked with its language. Marking every such word by hand is
tedious and easy to forget.

`mdlang` does it automatically. It finds runs of Chinese and Japanese text in
your Markdown and wraps each one in a small marker, which your site generator
then turns into a language-tagged HTML span (such as
`<span lang="zh-Hant-TW">…</span>`). The result reads correctly to assistive
technology, with no manual tagging.

## How it works

It parses the Markdown to an abstract syntax tree using
[goldmark](https://github.com/yuin/goldmark) and edits only text nodes, so
inline code, fenced code, link destinations, autolinks, and raw HTML are not
modified. `mdlang` only edits Markdown source; how the markers render is
configured in the consuming project.

## Behaviour

A *run* is a maximal sequence of target-script characters, joined across
interior digits, spaces, and CJK punctuation. A run of pure Han is tagged with
the `zh` delimiters; a run containing any kana (Hiragana or Katakana) is tagged
with the `jp` delimiters.

| Input | Output |
|-------|--------|
| `from 桃園 today` | `from {{zh}}桃園{{/zh}} today` |
| `五香粉，黑胡椒` | `{{zh}}五香粉，黑胡椒{{/zh}}` |
| `在2013年8月時` | `{{zh}}在2013年8月時{{/zh}}` |
| `8 月` | `{{zh}}8 月{{/zh}}` |
| `こんにちは` | `{{jp}}こんにちは{{/jp}}` |
| `日本語のテスト` | `{{jp}}日本語のテスト{{/jp}}` |

The following are left unchanged:

- Leading and trailing punctuation is excluded from a run (`去台灣。` →
  `{{zh}}去台灣{{/zh}}。`).
- CJK brackets around non-CJK content (`「english」`).
- Inline code (`` `台灣` ``) and fenced code blocks.
- A link whose text is a substring of its destination.
- URL- and IDN-like text: a run adjacent to `://`, preceded by `www.`, or
  forming a short dotted host such as `例子.測試` or `台灣.com`.
- A run glued to ASCII letters (`wifi台灣net`), unless
  `-tag-ascii-adjacent` is set.

The tool is idempotent: a run already enclosed by its delimiters is not
re-wrapped, so it is safe to run repeatedly.

## Install

```
go install github.com/jamesjj/markdown-language-attributes@latest
```

## Usage

```
mdlang [flags] [file ...]
```

With no file arguments, `mdlang` reads standard input and writes standard
output. With one or more files, it rewrites each in place unless `-stdout` is
given.

| Flag | Default | Description |
|------|---------|-------------|
| `-zh` | `true` | Tag pure-Han (Chinese) runs. |
| `-jp` | `true` | Tag kana-containing (Japanese) runs. |
| `-zh-open`, `-zh-close` | `{{zh}}`, `{{/zh}}` | Delimiters for a Chinese run. |
| `-jp-open`, `-jp-close` | `{{jp}}`, `{{/jp}}` | Delimiters for a Japanese run. |
| `-tag-ascii-adjacent` | `false` | Also tag runs glued to ASCII letters. |
| `-stdout` | `false` | Write to standard output instead of rewriting files. |
| `-check` | `false` | Make no changes; exit 1 if any input would change. |
| `-version` | | Print the version and exit. |

### Examples

```sh
# Preview changes for one file
mdlang -stdout content/post.md

# Rewrite files in place
mdlang content/post.md content/other.md

# Fail if any file is missing markers (CI gate)
mdlang -check content/*.md

# Chinese only
mdlang -jp=false content/post.md

# Custom delimiters
mdlang -zh-open '[[zh]]' -zh-close '[[/zh]]' content/post.md
```

## Library

```go
import "github.com/jamesjj/markdown-language-attributes/tagger"

out := tagger.Tag(src, tagger.DefaultConfig())
```

`tagger.Config` exposes the `zh`/`jp` delimiter pairs, the `EnableZh` /
`EnableJp` toggles, the ascii-adjacent option, and the IDN label-length
threshold.

## Limitations

- Only Markdown text nodes are processed. Text inside a raw HTML block (for
  example a hand-written `<figcaption>`) is opaque to the Markdown parser and is
  not marked.
- Detection is script-based, not language-based. A run of pure Han is tagged
  `zh` by default; it cannot distinguish Chinese Kanji from Japanese Kanji
  without kana present.
- The URL/IDN detection is heuristic and may not cover every case.

## License

[MIT](LICENSE)
