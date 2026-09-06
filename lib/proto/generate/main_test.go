package main

import (
	"context"
	"go/parser"
	"go/token"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// committedProto seeds lib/proto as a regeneration finds it: a hand-written
// file that must survive, a domain the roll no longer has, a type that moved
// to another domain, and a page.go with a field and a type to lose.
var committedProto = map[string]string{
	"a_patch.go": "package proto\n\n// Point from the origin.\ntype Point struct{ X, Y float64 }\n",

	"a_patch_test.go": "package proto_test\n",

	"database.go": strings.Join([]string{
		"package proto",
		"",
		"/*",
		"",
		"Database",
		"",
		"*/",
		"",
		"// DatabaseDatabaseID Unique identifier of Database object.",
		"type DatabaseDatabaseID string",
		"",
	}, "\n"),

	"browser.go": strings.Join([]string{
		"package proto",
		"",
		"/*",
		"",
		"Browser",
		"",
		"The Browser domain defines methods and events for browser managing.",
		"",
		"*/",
		"",
		"// BrowserSessionID Unique identifier of attached debugging session.",
		"type BrowserSessionID string",
		"",
	}, "\n"),

	"page.go": strings.Join([]string{
		"package proto",
		"",
		"/*",
		"",
		"Page",
		"",
		"*/",
		"",
		"// PageCaptureScreenshot Capture page screenshot.",
		"type PageCaptureScreenshot struct {",
		"\t// Quality (optional) Compression quality from range [0..100] (jpeg only).",
		"\tQuality *int `json:\"quality,omitempty\"`",
		"",
		"\t// Clip (optional) Capture the screenshot of a given region only.",
		"\tClip string `json:\"clip,omitempty\"`",
		"}",
		"",
		"// PageSetDownloadBehavior Set the behavior when downloading a file.",
		"type PageSetDownloadBehavior struct {",
		"\t// Behavior Whether to allow all or deny all download requests.",
		"\tBehavior string `json:\"behavior\"`",
		"}",
		"",
		"// PageFrameID Unique frame identifier.",
		"type PageFrameID string",
		"",
	}, "\n"),
}

func seedProto(t *testing.T) (root, dir string) {
	t.Helper()
	root = t.TempDir()
	dir = filepath.Join(root, "lib", "proto")
	for name, content := range committedProto {
		writeFile(t, filepath.Join(dir, name), content)
	}
	return root, dir
}

// squash folds the alignment gofmt adds, so an assertion can quote a line.
func squash(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func listDir(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

func newTestGenerator(t *testing.T, root string) *generator {
	t.Helper()
	src, err := localSource(tagDir(t), 123)
	if err != nil {
		t.Fatal(err)
	}
	return &generator{
		root:   root,
		src:    src,
		roll:   123,
		binary: testBinaryFields,
		log:    log.New(io.Discard, "", 0),
	}
}

func TestGenerate(t *testing.T) {
	g := setup(t)
	root, dir := seedProto(t)

	summary, err := newTestGenerator(t, root).generate(context.Background())
	g.E(err)

	// The hand-written files stay, the removed domain's file is gone, every
	// domain of the roll has a file, and every file parses.
	g.Eq(listDir(t, dir), []string{
		"a_patch.go", "a_patch_test.go",
		"console.go", "debugger.go", "definitions.go", "definitions_test.go",
		"fetch.go", "input.go", "network.go", "page.go", "target.go",
	})
	g.Eq(readFile(t, filepath.Join(dir, "a_patch.go")), committedProto["a_patch.go"])
	for _, name := range listDir(t, dir) {
		_, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, name), nil, parser.ParseComments)
		g.E(err)
	}

	definitions := readFile(t, filepath.Join(dir, "definitions.go"))
	g.Has(definitions, `const Version = "v1.3"`)
	g.Has(definitions, "const ProtocolRoll = 123")
	g.Has(squash(definitions), `"Page.captureScreenshot": reflect.TypeOf(PageCaptureScreenshot{}),`)
	g.Has(squash(definitions), `"Debugger.getWasmBytecode": reflect.TypeOf(DebuggerGetWasmBytecode{}),`)

	// []byte from the marker on a field, on a type, and from the hand list.
	page := readFile(t, filepath.Join(dir, "page.go"))
	g.Has(page, "Data []byte `json:\"data\"`")
	g.Has(page, "PrimaryIcon []byte `json:\"primaryIcon,omitempty\"`")
	g.Has(page, "Chunks [][]byte `json:\"chunks,omitempty\"`")
	fetch := readFile(t, filepath.Join(dir, "fetch.go"))
	g.Has(fetch, "type FetchBlob []byte")
	g.Has(fetch, "Body []byte `json:\"body\"`") // the patch drops optional
	g.Has(readFile(t, filepath.Join(dir, "debugger.go")), "Bytecode []byte `json:\"bytecode\"`")

	// Deprecated: a command, its result, a field, and every entity of a
	// deprecated domain, with a period after the description.
	g.Has(page, "// PageSetDownloadBehavior (deprecated) Set the behavior when downloading a file.\n"+
		"//\n// Deprecated: Page.setDownloadBehavior is deprecated in the Chrome DevTools Protocol.\n"+
		"type PageSetDownloadBehavior struct {")
	g.Has(page, "// Quality (optional) Compression quality from range [0..100] (jpeg only).\n")
	// The enum type of a deprecated command's parameter, and its constants.
	g.Has(page, "// PageSetDownloadBehaviorBehavior (deprecated) enum.\n"+
		"//\n// Deprecated: Page.setDownloadBehavior is deprecated in the Chrome DevTools Protocol.\n"+
		"type PageSetDownloadBehaviorBehavior string")
	g.Has(page, "\t// PageSetDownloadBehaviorBehaviorDeny enum const.\n"+
		"\t//\n\t// Deprecated: Page.setDownloadBehavior is deprecated in the Chrome DevTools Protocol.\n"+
		"\tPageSetDownloadBehaviorBehaviorDeny PageSetDownloadBehaviorBehavior = \"deny\"")
	debugger := readFile(t, filepath.Join(dir, "debugger.go"))
	g.Has(debugger, "// Deprecated: Debugger.getWasmBytecode is deprecated in the Chrome DevTools Protocol.\n"+
		"type DebuggerGetWasmBytecode struct {")
	g.Has(debugger, "// Deprecated: Debugger.getWasmBytecode is deprecated in the Chrome DevTools Protocol.\n"+
		"type DebuggerGetWasmBytecodeResult struct {")
	network := readFile(t, filepath.Join(dir, "network.go"))
	g.Has(network, "\t// SameParty (deprecated) True if cookie is SameParty.\n"+
		"\t//\n\t// Deprecated: Network.Cookie.sameParty is deprecated in the Chrome DevTools Protocol.\n"+
		"\tSameParty bool `json:\"sameParty\"`")
	g.Has(network, "Expires TimeSinceEpoch `json:\"expires\"`") // the patch
	console := readFile(t, filepath.Join(dir, "console.go"))
	g.Has(console, "// Deprecated: the Console domain is deprecated in the Chrome DevTools Protocol.\ntype ConsoleConsoleMessage struct {")
	g.Has(console, "// Deprecated: the Console domain is deprecated in the Chrome DevTools Protocol.\ntype ConsoleEnable struct {")
	g.Has(console, "\t// Text ...\n\tText string `json:\"text\"`") // no paragraph on a field of a deprecated entity

	// The enum the patch adds, and the import only where it is used.
	g.Has(readFile(t, filepath.Join(dir, "target.go")), `TargetTargetInfoTypePage TargetTargetInfoType = "page"`)
	g.False(strings.Contains(page, "lazyjson"))

	// The summary, printed and written.
	g.Eq(summary, readFile(t, filepath.Join(root, "tmp", "proto-summary.md")))
	g.Has(summary, "Protocol roll r123: 3 Go identifiers removed, 1 renamed, 10 newly deprecated,")
	g.Has(summary, "\nRemoved:\n- `DatabaseDatabaseID`\n- `PageCaptureScreenshot.Clip`\n- `PageFrameID`\n")
	g.Has(summary, "\nRenamed:\n- `BrowserSessionID` -> `TargetSessionID`\n")
	g.Has(summary, "\nNewly deprecated:\n- `ConsoleConsoleMessage`\n- `ConsoleEnable`\n- `DebuggerGetWasmBytecode`\n"+
		"- `DebuggerGetWasmBytecodeResult`\n- `NetworkCookie.SameParty`\n- `PageSetDownloadBehavior`\n"+
		"- `PageSetDownloadBehaviorBehavior`\n- `PageSetDownloadBehaviorBehaviorAllow`\n"+
		"- `PageSetDownloadBehaviorBehaviorDefault`\n- `PageSetDownloadBehaviorBehaviorDeny`\n")

	_, err = os.Stat(filepath.Join(root, "tmp", "proto.json"))
	g.E(err)

	// A second run is a no-op with nothing to report.
	summary, err = newTestGenerator(t, root).generate(context.Background())
	g.E(err)
	g.Has(summary, "Protocol roll r123: 0 Go identifiers removed, 0 renamed, 0 newly deprecated, 0 added.")
	g.Has(summary, "\nRemoved: none\n\nRenamed: none\n\nNewly deprecated: none\n")
}

func TestGenerateBinaryMismatch(t *testing.T) {
	g := setup(t)
	root, dir := seedProto(t)
	before := listDir(t, dir)

	gen := newTestGenerator(t, root)
	gen.binary = testBinaryFields[:1] // chunks is not restored

	_, err := gen.generate(context.Background())
	g.Err(err)
	g.Has(err.Error(), "the PDL files use binary 6 times but the generated code would have 5 []byte fields")
	g.Has(err.Error(), "pdl/domains/Page.pdl:13: optional array of binary chunks")
	g.Has(err.Error(), "PageCaptureScreenshotResult.Data")
	g.Has(err.Error(), "FetchBlob")

	// Nothing was written.
	g.Eq(listDir(t, dir), before)
	g.Eq(readFile(t, filepath.Join(dir, "page.go")), committedProto["page.go"])
}

func TestGenerateListedFieldMissing(t *testing.T) {
	g := setup(t)
	root, _ := seedProto(t)

	gen := newTestGenerator(t, root)
	gen.binary = append([]binaryField{{"Page", "commands", "captureScreenshot", "parameters", "clip"}}, testBinaryFields...)

	_, err := gen.generate(context.Background())
	g.Err(err)
	g.Has(err.Error(), "Page.captureScreenshot.clip (commands/parameters) is listed as a marker-less binary field")
	g.Has(err.Error(), "drop it from binaryFields")
}

func TestGeneratePatchTargetMissing(t *testing.T) {
	g := setup(t)
	root, _ := seedProto(t)

	gen := newTestGenerator(t, root)
	schema := strings.Replace(tagFiles["json/browser_protocol.json"], `"id": "TargetInfo"`, `"id": "TargetInfo2"`, 1)
	writeFile(t, filepath.Join(gen.src.base, "json", "browser_protocol.json"), schema)

	_, err := gen.generate(context.Background())
	g.Err(err)
	g.Has(err.Error(), `types: no id "TargetInfo"`)
}

func TestGenerateUnreadable(t *testing.T) {
	g := setup(t)
	root, dir := seedProto(t)

	gen := newTestGenerator(t, root)
	writeFile(t, filepath.Join(dir, "broken.go"), "package proto\n\nfunc {")
	_, err := gen.generate(context.Background())
	g.Err(err)

	g.E(os.RemoveAll(dir))
	_, err = gen.generate(context.Background())
	g.Err(err)
}

func TestParseArgs(t *testing.T) {
	g := setup(t)

	opts, err := parseArgs(nil)
	g.E(err)
	g.Eq(opts, options{})

	opts, err = parseArgs([]string{"-schema", "checkout"})
	g.E(err)
	g.Eq(opts.schema, "checkout")

	_, err = parseArgs([]string{"extra"})
	g.Err(err)
	_, err = parseArgs([]string{"-unknown"})
	g.Err(err)

	var b strings.Builder
	usage(&b)
	g.Has(b.String(), "-schema")
}

func TestRunLocalSourceRefused(t *testing.T) {
	g := setup(t)

	// A checkout of another roll than the pins name is refused before
	// anything is read or written.
	g.Eq(run(options{schema: tagDir(t)}), 1)
}
