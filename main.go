// Command mdlang wraps runs of a target Unicode script in Markdown files with
// configurable delimiters, using the Markdown AST so that code, links, and raw
// HTML are never modified.
//
// Typical use is a CI step that inserts inline markers a static-site generator
// then renders as language spans. The tool itself is generator-agnostic.
//
// Usage:
//
//	mdlang [flags] [file ...]
//
// With no file arguments it reads stdin and writes stdout. With files it
// rewrites each in place unless -stdout is given. Exit status 0 on success;
// with -check it exits 1 if any file would change (no writes), for CI gating.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/jamesjj/markdown-language-attributes/tagger"
)

// version is set at build time via -ldflags "-X main.version=…".
var version = "dev"

func main() {
	var (
		showVersion = flag.Bool("version", false, "print version and exit")
		zhOpen      = flag.String("zh-open", "{{zh}}", "opening delimiter for a pure-Han (Chinese) run")
		zhClose     = flag.String("zh-close", "{{/zh}}", "closing delimiter for a pure-Han (Chinese) run")
		jpOpen      = flag.String("jp-open", "{{jp}}", "opening delimiter for a kana-containing (Japanese) run")
		jpClose     = flag.String("jp-close", "{{/jp}}", "closing delimiter for a kana-containing (Japanese) run")
		enableZh    = flag.Bool("zh", true, "tag pure-Han (Chinese) runs")
		enableJp    = flag.Bool("jp", true, "tag kana-containing (Japanese) runs")
		ascii       = flag.Bool("tag-ascii-adjacent", false, "also tag runs glued to ASCII letters (e.g. wifi\u53f0\u7063net)")
		toStdout    = flag.Bool("stdout", false, "write result to stdout instead of rewriting files in place")
		check       = flag.Bool("check", false, "do not write; exit 1 if any input would change (for CI)")
	)
	flag.Parse()

	if *showVersion {
		fmt.Println("mdlang", version)
		return
	}

	cfg := tagger.DefaultConfig()
	cfg.ZhOpen, cfg.ZhClose = *zhOpen, *zhClose
	cfg.JpOpen, cfg.JpClose = *jpOpen, *jpClose
	cfg.EnableZh, cfg.EnableJp = *enableZh, *enableJp
	cfg.TagAsciiAdjacent = *ascii

	files := flag.Args()

	if len(files) == 0 {
		in, err := io.ReadAll(os.Stdin)
		if err != nil {
			fatal(err)
		}
		out := tagger.Tag(in, cfg)
		if *check {
			os.Exit(changedExit(in, out))
		}
		os.Stdout.Write(out)
		return
	}

	anyChanged := false
	for _, f := range files {
		in, err := os.ReadFile(f)
		if err != nil {
			fatal(err)
		}
		out := tagger.Tag(in, cfg)
		changed := string(in) != string(out)
		anyChanged = anyChanged || changed

		switch {
		case *check:
			if changed {
				fmt.Fprintf(os.Stderr, "would change: %s\n", f)
			}
		case *toStdout:
			os.Stdout.Write(out)
		case changed:
			if err := os.WriteFile(f, out, 0o644); err != nil {
				fatal(err)
			}
		}
	}
	if *check && anyChanged {
		os.Exit(1)
	}
}

func changedExit(in, out []byte) int {
	if string(in) != string(out) {
		return 1
	}
	return 0
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "mdlang:", err)
	os.Exit(2)
}
