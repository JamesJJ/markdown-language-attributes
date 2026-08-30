// Command mdlang wraps runs of Chinese and Japanese script in Markdown files
// with language markers, using the Markdown AST so that code, links, and raw
// HTML are never modified.
//
// The output form is selected by the required -mode flag:
//
//	markers      {{zh}}…{{/zh}}                {{ja}}…{{/ja}}
//	passthrough  <!--lang:zh-->…<!--/lang-->    <!--lang:ja-->…<!--/lang-->
//	spans        <span lang="zh-Hant-TW">…</span> <span lang="ja">…</span>
//
// Usage:
//
//	mdlang -mode=<markers|passthrough|spans> [flags] [file ...]
//
// With no file arguments it reads stdin and writes stdout. With files it
// rewrites each in place unless -stdout is given. With -check it makes no
// changes and exits 1 if any input would change (for CI).
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
		mode        = flag.String("mode", "", "REQUIRED output mode: markers | passthrough | spans")
		enableZh    = flag.Bool("zh", true, "tag pure-Han (Chinese) runs")
		enableJa    = flag.Bool("ja", true, "tag kana-containing (Japanese) runs")
		ascii       = flag.Bool("tag-ascii-adjacent", false, "also tag runs glued to ASCII letters (e.g. wifi\u53f0\u7063net)")
		toStdout    = flag.Bool("stdout", false, "write result to stdout instead of rewriting files in place")
		check       = flag.Bool("check", false, "do not write; exit 1 if any input would change (for CI)")
	)
	flag.Parse()

	if *showVersion {
		fmt.Println("mdlang", version)
		return
	}

	m, err := tagger.ParseMode(*mode)
	if err != nil {
		fmt.Fprintln(os.Stderr, "mdlang:", err)
		fmt.Fprintln(os.Stderr, "the -mode flag is required")
		os.Exit(2)
	}

	cfg := tagger.DefaultConfig()
	cfg.Mode = m
	cfg.EnableZh, cfg.EnableJa = *enableZh, *enableJa
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
