package main

import (
	"path/filepath"
	"testing"
)

func symbolsOf(t *testing.T, files map[string]string) *symbolSet {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		writeFile(t, filepath.Join(dir, name), content)
	}
	set, err := readSymbols(dir)
	if err != nil {
		t.Fatal(err)
	}
	return set
}

func TestReadSymbols(t *testing.T) {
	g := setup(t)

	set := symbolsOf(t, map[string]string{
		"a_hand.go":    "package proto\n\ntype Hand struct{ Written bool }\n",
		"page_test.go": "package proto_test\n\ntype Test struct{}\n",
		"page.go": "package proto\n\n/*\n\nPage\n\n*/\n\n" +
			"// PageFrame (deprecated) old style.\ntype PageFrame struct {\n" +
			"\t// ID of the frame.\n\tID string\n" +
			"\t// Name (deprecated) of the frame.\n\tName string\n" +
			"\t// Loader ...\n\t//\n\t// Deprecated: gone.\n\tLoaderID string\n" +
			"\tunexported int\n}\n\n" +
			"// PageFrameID ...\ntype PageFrameID string\n\n" +
			"const (\n\t// PageFrameIDA enum const.\n\tPageFrameIDA PageFrameID = \"a\"\n)\n" +
			"type pagePrivate int\n",
		"definitions.go": "package proto\n\n// Version ...\nconst Version = \"v1.3\"\n",
	})

	g.Eq(set.domain, map[string]string{
		"PageFrame":    "Page",
		"PageFrameID":  "Page",
		"PageFrameIDA": "Page",
		"Version":      "",
	})
	g.Eq(set.fields, map[string]bool{
		"PageFrame.ID":       true,
		"PageFrame.Name":     true,
		"PageFrame.LoaderID": true,
	})
	g.Eq(set.deprecated, map[string]bool{
		"PageFrame":          true,
		"PageFrame.Name":     true,
		"PageFrame.LoaderID": true,
	})
}

func TestReadSymbolsErrors(t *testing.T) {
	g := setup(t)

	_, err := readSymbols(filepath.Join(t.TempDir(), "missing"))
	g.Err(err)

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "broken.go"), "package proto\n\ntype {")
	_, err = readSymbols(dir)
	g.Err(err)
}

func TestSummarizeRenames(t *testing.T) {
	g := setup(t)

	before := symbolsOf(t, map[string]string{
		"css.go": "package proto\n\n/*\n\nCSS\n\n*/\n\ntype CSSStyleSheetID string\n\ntype CSSRule struct{ Text string }\n",
		"page.go": "package proto\n\n/*\n\nPage\n\n*/\n\n" +
			"type PageFrame struct{ ID string; URL string }\n\ntype PageOld string\n\ntype PageAmbiguous string\n",
	})
	after := symbolsOf(t, map[string]string{
		"dom.go": "package proto\n\n/*\n\nDOM\n\n*/\n\ntype DOMStyleSheetID string\n\ntype DOMAmbiguous string\n",
		"css.go": "package proto\n\n/*\n\nCSS\n\n*/\n\n" +
			"// CSSRule ...\n//\n// Deprecated: gone.\ntype CSSRule struct{ Text string }\n",
		"page.go": "package proto\n\n/*\n\nPage\n\n*/\n\n" +
			"type PageFrame struct{ ID string }\n\ntype PageNew string\n",
		"net.go": "package proto\n\n/*\n\nNetwork\n\n*/\n\ntype NetworkAmbiguous string\n",
	})

	s := summarize(before, after)

	// StyleSheetID moved from CSS to DOM: a rename. Ambiguous has two
	// candidates, so it is a removal. Old has none. Frame lost URL.
	g.Eq(s.renamed, [][2]string{{"CSSStyleSheetID", "DOMStyleSheetID"}})
	g.Eq(s.removed, []string{"PageAmbiguous", "PageFrame.URL", "PageOld"})
	g.Eq(s.deprecated, []string{"CSSRule"})
	g.Eq(s.added, 4)

	g.Eq(s.markdown(7), "Protocol roll r7: 3 Go identifiers removed, 1 renamed, 1 newly deprecated, 4 added.\n"+
		"\nRemoved:\n- `PageAmbiguous`\n- `PageFrame.URL`\n- `PageOld`\n"+
		"\nRenamed:\n- `CSSStyleSheetID` -> `DOMStyleSheetID`\n"+
		"\nNewly deprecated:\n- `CSSRule`\n")
}

func TestSummarizeFieldsFollowRenames(t *testing.T) {
	g := setup(t)

	before := symbolsOf(t, map[string]string{
		"css.go": "package proto\n\n/*\n\nCSS\n\n*/\n\n" +
			"type CSSRule struct{ Text string; Origin string }\n",
	})
	after := symbolsOf(t, map[string]string{
		"dom.go": "package proto\n\n/*\n\nDOM\n\n*/\n\n" +
			"type DOMRule struct {\n\tText string\n\t// Origin ...\n\t//\n\t// Deprecated: gone.\n\tOrigin string\n}\n",
	})

	s := summarize(before, after)
	g.Eq(s.renamed, [][2]string{{"CSSRule", "DOMRule"}})
	g.Eq(len(s.removed), 0)
	g.Eq(s.deprecated, []string{"DOMRule.Origin"})

	// Already deprecated before the rename: nothing new.
	before = symbolsOf(t, map[string]string{
		"css.go": "package proto\n\n/*\n\nCSS\n\n*/\n\n" +
			"type CSSRule struct {\n\tText string\n\t// Origin (deprecated) ...\n\tOrigin string\n}\n",
	})
	s = summarize(before, after)
	g.Eq(len(s.deprecated), 0)
}

func TestCommentHelpers(t *testing.T) {
	g := setup(t)

	g.Eq(withPeriod("Capture page screenshot"), "Capture page screenshot.")
	g.Eq(withPeriod("Done.  "), "Done.")
	g.Eq(withPeriod("Really?"), "Really?")
	g.Eq(withPeriod("Go!"), "Go!")
	g.Eq(commentLines("A\n\nB"), "// A\n//\n// B")
}
