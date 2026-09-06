package main

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// The READMEs carry the pins as a table the way Playwright's does, so a user
// sees at a glance which browser versions wand is aligned to and where a
// Managed browser can be downloaded. The Roll rewrites everything between
// these two markers; the prose around them is hand-written.
const (
	blockBegin = "<!-- pins:begin -->"
	blockEnd   = "<!-- pins:end -->"

	chromeForTestingSite = "https://googlechromelabs.github.io/chrome-for-testing/"
	chromiumBucketSite   = "https://commondatastorage.googleapis.com/chromium-browser-snapshots/index.html"
	protocolTagSite      = "https://github.com/ChromeDevTools/devtools-protocol/tree/v0.0."
)

// readmeColumns are the platforms the table shows, each with the Chrome for
// Testing platform and the Chromium bucket prefix that serve it. A missing
// prefix means the Chromium trunk build bucket has no build for it.
var readmeColumns = []struct {
	name     string
	chrome   string
	chromium string
}{
	{"Linux x64", "linux64", "Linux_x64"},
	{"Linux arm64", "linux-arm64", ""},
	{"macOS x64", "mac-x64", "Mac"},
	{"macOS arm64", "mac-arm64", "Mac_Arm"},
	{"Windows x86", "win32", "Win"},
	{"Windows x64", "win64", "Win_x64"},
}

// readmeWords are the strings that differ between the two READMEs.
type readmeWords struct {
	browser        string // the table's first header cell
	chromeForTest  string // the Chrome for Testing link text
	trunkBuild     string // the Chromium trunk build link text
	open, close    string // the parentheses around a link
	protocol       string // "Protocol: " and "Support window: " sentences
	supportWindow  string
	windowBetween  string // "%d to %d"
	windowMeaning  string // the clause after the range
	sentenceEnd    string
	sentenceJoiner string
}

var readmeLanguages = map[string]readmeWords{
	"README.md": {
		browser:        "Managed browser",
		chromeForTest:  "Chrome for Testing",
		trunkBuild:     "trunk build",
		open:           " (",
		close:          ")",
		protocol:       "Protocol: ",
		supportWindow:  "Support window: Chrome ",
		windowBetween:  "%d to %d",
		windowMeaning:  ", the Target Chrome and the three stable milestones before it",
		sentenceEnd:    ".",
		sentenceJoiner: " ",
	},
	"README.zh-CN.md": {
		browser:        "Managed browser",
		chromeForTest:  "Chrome for Testing",
		trunkBuild:     "主干构建",
		open:           "（",
		close:          "）",
		protocol:       "协议：",
		supportWindow:  "支持窗口：Chrome ",
		windowBetween:  "%d 至 %d",
		windowMeaning:  "，即 Target Chrome 及其之前的三个稳定里程碑",
		sentenceEnd:    "。",
		sentenceJoiner: "",
	},
}

// readmeBlock is the generated part of a README: the table of Managed
// browsers by platform, with a mark wherever the pins hold an archive hash,
// then the Protocol roll and the Support window (ADR-0008: the Target Chrome
// and the three stable milestones before it). prettier is told to leave the
// table alone, so the cells need no padding; the blank line after its end
// marker is the one prettier puts between two HTML blocks.
func readmeBlock(p Pins, readme string) (string, error) {
	words, known := readmeLanguages[readme]
	if !known {
		return "", fmt.Errorf("no README words for %s", readme)
	}
	major, err := strconv.Atoi(strings.SplitN(p.ChromeVersion, ".", 2)[0])
	if err != nil {
		return "", fmt.Errorf("bad Chrome version %q", p.ChromeVersion)
	}

	b := &strings.Builder{}
	b.WriteString("<!-- prettier-ignore-start -->\n")

	fmt.Fprintf(b, "| %s |", words.browser)
	for _, column := range readmeColumns {
		fmt.Fprintf(b, " %s |", column.name)
	}
	b.WriteString("\n| :--- |")
	for range readmeColumns {
		b.WriteString(" :---: |")
	}
	b.WriteString("\n")

	row := func(label string, has func(column int) bool) {
		fmt.Fprintf(b, "| %s |", label)
		for i := range readmeColumns {
			mark := "❌"
			if has(i) {
				mark = "✅"
			}
			fmt.Fprintf(b, " %s |", mark)
		}
		b.WriteString("\n")
	}
	chrome := func(binary string) func(int) bool {
		return func(i int) bool {
			_, has := p.ChromeSHA256[binary][readmeColumns[i].chrome]
			return has
		}
	}
	row(fmt.Sprintf("Chrome %s%s[%s](%s)%s",
		p.ChromeVersion, words.open, words.chromeForTest, chromeForTestingSite, words.close), chrome("chrome"))
	row("chrome-headless-shell "+p.ChromeVersion, chrome("chrome-headless-shell"))
	row(fmt.Sprintf("Chromium %d%s[%s](%s)%s",
		p.ChromiumPosition, words.open, words.trunkBuild, chromiumBucketSite, words.close),
		func(i int) bool {
			prefix := readmeColumns[i].chromium
			_, has := p.ChromiumSHA256[prefix]
			return prefix != "" && has
		})

	fmt.Fprintf(b, "\n%s[devtools-protocol r%d](%s%d)%s%s%s",
		words.protocol, p.ProtocolRoll, protocolTagSite, p.ProtocolRoll,
		words.sentenceEnd, words.sentenceJoiner, words.supportWindow)
	fmt.Fprintf(b, words.windowBetween, major-3, major)
	fmt.Fprintf(b, "%s%s\n", words.windowMeaning, words.sentenceEnd)

	b.WriteString("<!-- prettier-ignore-end -->\n\n")
	return b.String(), nil
}

// replaceBlock puts block between the markers of doc, keeping everything
// else, so a hand-written README and a generated table live in one file.
func replaceBlock(doc []byte, block string) ([]byte, error) {
	begin := bytes.Index(doc, []byte(blockBegin+"\n"))
	if begin < 0 {
		return nil, fmt.Errorf("no %s marker", blockBegin)
	}
	begin += len(blockBegin) + 1
	end := bytes.Index(doc[begin:], []byte(blockEnd))
	if end < 0 {
		return nil, fmt.Errorf("no %s marker after %s", blockEnd, blockBegin)
	}
	end += begin

	out := make([]byte, 0, len(doc)+len(block))
	out = append(out, doc[:begin]...)
	out = append(out, block...)
	out = append(out, doc[end:]...)
	return out, nil
}
