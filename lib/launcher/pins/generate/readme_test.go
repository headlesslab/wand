package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestReadmeBlock(t *testing.T) {
	g := setup(t)

	p := samplePins()

	en, err := readmeBlock(p, "README.md")
	g.E(err)

	// prettier leaves the table alone, so its cells need no padding.
	g.True(strings.HasPrefix(en, "<!-- prettier-ignore-start -->\n| Managed browser | Linux x64 | Linux arm64 |"))
	g.True(strings.HasSuffix(en, "<!-- prettier-ignore-end -->\n\n"))
	g.Has(en, "\n| :--- | :---: | :---: | :---: | :---: | :---: | :---: |\n")

	// A mark wherever the pins hold a hash: chrome has linux64, linux-arm64
	// and win64; chrome-headless-shell only linux64; Chromium Linux_x64 and
	// Win, and never linux-arm64, which the trunk build bucket does not serve.
	g.Has(en, "| Chrome 152.0.7977.82 ([Chrome for Testing](https://googlechromelabs.github.io/chrome-for-testing/)) | ✅ | ✅ | ❌ | ❌ | ❌ | ✅ |\n")
	g.Has(en, "| chrome-headless-shell 152.0.7977.82 | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ |\n")
	g.Has(en, "| Chromium 1668000 ([trunk build](https://commondatastorage.googleapis.com/chromium-browser-snapshots/index.html)) | ✅ | ❌ | ❌ | ❌ | ✅ | ❌ |\n")

	g.Has(en, "\nProtocol: [devtools-protocol r1666840](https://github.com/ChromeDevTools/devtools-protocol/tree/v0.0.1666840). "+
		"Support window: Chrome 149 to 152, the Target Chrome and the three stable milestones before it.\n")

	zh, err := readmeBlock(p, "README.zh-CN.md")
	g.E(err)
	g.Has(zh, "| Chrome 152.0.7977.82（[Chrome for Testing](https://googlechromelabs.github.io/chrome-for-testing/)） | ✅ | ✅ | ❌ | ❌ | ❌ | ✅ |\n")
	g.Has(zh, "| Chromium 1668000（[主干构建](https://commondatastorage.googleapis.com/chromium-browser-snapshots/index.html)） | ✅ |")
	g.Has(zh, "\n协议：[devtools-protocol r1666840](https://github.com/ChromeDevTools/devtools-protocol/tree/v0.0.1666840)。"+
		"支持窗口：Chrome 149 至 152，即 Target Chrome 及其之前的三个稳定里程碑。\n")

	// The same pins give the same bytes.
	again, err := readmeBlock(p, "README.md")
	g.E(err)
	g.Eq(again, en)

	_, err = readmeBlock(p, "README.fr.md")
	g.Err(err)

	p.ChromeVersion = "x"
	_, err = readmeBlock(p, "README.md")
	g.Err(err)
}

func TestReplaceBlock(t *testing.T) {
	g := setup(t)

	doc := []byte("# wand\n\nintro\n\n<!-- pins:begin -->\nold\ntable\n<!-- pins:end -->\n\n## Roadmap\n")

	out, err := replaceBlock(doc, "new\n")
	g.E(err)
	g.Eq(string(out), "# wand\n\nintro\n\n<!-- pins:begin -->\nnew\n<!-- pins:end -->\n\n## Roadmap\n")

	// Idempotent.
	again, err := replaceBlock(out, "new\n")
	g.E(err)
	g.Eq(string(again), string(out))

	// An empty block is allowed.
	out, err = replaceBlock([]byte("<!-- pins:begin -->\n<!-- pins:end -->\n"), "x\n")
	g.E(err)
	g.Eq(string(out), "<!-- pins:begin -->\nx\n<!-- pins:end -->\n")

	_, err = replaceBlock([]byte("no markers\n"), "x\n")
	g.Err(err)
	g.Has(err.Error(), "<!-- pins:begin -->")

	_, err = replaceBlock([]byte("<!-- pins:begin -->\nnever closed\n"), "x\n")
	g.Err(err)
	g.Has(err.Error(), "<!-- pins:end -->")

	// The marker must start a line of its own.
	_, err = replaceBlock([]byte("inline <!-- pins:begin --> text <!-- pins:end -->\n"), "x\n")
	g.Err(err)

	g.True(bytes.Equal(doc, []byte("# wand\n\nintro\n\n<!-- pins:begin -->\nold\ntable\n<!-- pins:end -->\n\n## Roadmap\n")))
}
