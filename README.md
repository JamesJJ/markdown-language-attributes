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
interior digits, spaces, and CJK punctuation. A run of pure Han is classified
`zh`; a run containing any kana (Hiragana or Katakana) is classified `ja`.

The output form is chosen by the required `-mode` flag:

| Mode | `zh` run | `ja` run |
|------|----------|----------|
| `markers` | `{{zh}}台灣{{/zh}}` | `{{ja}}こんにちは{{/ja}}` |
| `passthrough` | `<!--lang:zh-->台灣<!--/lang-->` | `<!--lang:ja-->こんにちは<!--/lang-->` |
| `spans` | `<span lang="zh-Hant-TW">台灣</span>` | `<span lang="ja">こんにちは</span>` |

- **`markers`** — compact custom delimiters, rendered by a downstream template.
- **`passthrough`** — a valid HTML comment pair. It degrades to invisible
  comments if the renderer has no handler, and carries the language code inside
  the delimited text so a single Hugo `render-passthrough` hook can route every
  language and strip the marker. Works whether or not the renderer allows raw
  HTML.
- **`spans`** — final `<span lang>` HTML directly; needs the consuming renderer
  to permit raw HTML (e.g. Hugo `markup.goldmark.renderer.unsafe = true`).

Run detection (mode-independent):

| Input | `zh`/`ja` |
|-------|-----------|
| `from 桃園 today` | `桃園` → zh |
| `五香粉，黑胡椒` | one zh run across the comma |
| `在2013年8月時` | one zh run, digits kept |
| `8 月` | zh, digit kept |
| `こんにちは` | ja |
| `日本語のテスト` | ja |

The following are left unchanged in every mode:

- Leading and trailing punctuation is excluded from a run (`去台灣。` → the run
  is `去台灣`, the `。` stays outside).
- CJK brackets around non-CJK content (`「english」`).
- Inline code (`` `台灣` ``) and fenced code blocks.
- A link whose text is a substring of its destination.
- URL- and IDN-like text: a run adjacent to `://`, preceded by `www.`, or
  forming a short dotted host such as `例子.測試` or `台灣.com`.
- A run glued to ASCII letters (`wifi台灣net`), unless
  `-tag-ascii-adjacent` is set.

The tool is idempotent per mode: a run already wrapped in the active mode's
delimiters is not re-wrapped, so it is safe to run repeatedly.

> Classification is script-based. A kanji-only run with no kana is classified
> `zh` even if it is Japanese, because it is indistinguishable from Chinese by
> script alone.

## Install

```
go install github.com/jamesjj/markdown-language-attributes@latest
```

## Usage

```
mdlang -mode=<markers|passthrough|spans> [flags] [file ...]
```

`-mode` is required. With no file arguments, `mdlang` reads standard input and
writes standard output. With one or more files, it rewrites each in place unless
`-stdout` is given.

| Flag | Default | Description |
|------|---------|-------------|
| `-mode` | *(required)* | Output form: `markers`, `passthrough`, or `spans`. |
| `-zh` | `true` | Tag pure-Han (Chinese) runs. |
| `-ja` | `true` | Tag kana-containing (Japanese) runs. |
| `-tag-ascii-adjacent` | `false` | Also tag runs glued to ASCII letters. |
| `-stdout` | `false` | Write to standard output instead of rewriting files. |
| `-check` | `false` | Make no changes; exit 1 if any input would change. |
| `-version` | | Print the version and exit. |

### Examples

```sh
# Preview changes for one file (passthrough mode)
mdlang -mode=passthrough -stdout content/post.md

# Rewrite files in place as HTML comment markers
mdlang -mode=passthrough content/post.md content/other.md

# Emit final spans directly (renderer must allow raw HTML)
mdlang -mode=spans content/post.md

# Fail if any file is missing markers (CI gate)
mdlang -mode=passthrough -check content/*.md

# Chinese only
mdlang -mode=markers -ja=false content/post.md
```

## Library

```go
import "github.com/jamesjj/markdown-language-attributes/tagger"

cfg := tagger.DefaultConfig()
cfg.Mode = tagger.ModePassthrough // required
out := tagger.Tag(src, cfg)
```

`tagger.Config` exposes `Mode` (required — `ModeMarkers`, `ModePassthrough`, or
`ModeSpans`), the `EnableZh` / `EnableJa` toggles, the ascii-adjacent option,
and the IDN label-length threshold.

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
