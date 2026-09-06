package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ysmood/got"
)

var setup = got.Setup(nil)

// tagFiles is a small devtools-protocol tag, v0.0.123: enough schema for
// every patch to find its target, a binary field of each kind (a marker on
// a field, a marker on a type, a marker-less field, a marker-less array), a
// deprecated domain, a deprecated command and a deprecated field.
var tagFiles = map[string]string{
	"package.json": `{"name": "devtools-protocol", "version": "0.0.123"}`,

	"json/browser_protocol.json": `{
		"version": {"major": "1", "minor": "3"},
		"domains": [
			{"domain": "Target", "types": [
				{"id": "TargetInfo", "type": "object", "properties": [{"name": "type", "type": "string"}]},
				{"id": "SessionID", "type": "string", "description": "Unique identifier of attached debugging session."}
			], "commands": [], "events": []},
			{"domain": "Page", "description": "Actions and events related to the inspected page belong to the page domain.",
			 "types": [],
			 "commands": [
				{"name": "captureScreenshot", "description": "Capture page screenshot.",
				 "parameters": [{"name": "quality", "type": "integer", "optional": true, "description": "Compression quality from range [0..100] (jpeg only)"}],
				 "returns": [{"name": "data", "type": "string", "description": "Base64-encoded image data. (Encoded as a base64 string when passed over JSON)"}]},
				{"name": "setDownloadBehavior", "deprecated": true, "description": "Set the behavior when downloading a file.",
				 "parameters": [{"name": "behavior", "type": "string", "enum": ["deny", "allow", "default"]}, {"name": "downloadPath", "type": "string", "optional": true}]},
				{"name": "getManifestIcons", "returns": [{"name": "primaryIcon", "type": "string", "optional": true}]}
			 ],
			 "events": [
				{"name": "lifecycleEvent", "parameters": [{"name": "name", "type": "string"}]},
				{"name": "screencastFrame", "description": "Compressed image data requested by the startScreencast.",
				 "parameters": [{"name": "chunks", "type": "array", "optional": true, "items": {"type": "string"}}]}
			 ]},
			{"domain": "Input", "types": [{"id": "TimeSinceEpoch", "type": "number"}],
			 "commands": [{"name": "dispatchMouseEvent", "parameters": [
				{"name": "deltaX", "type": "number", "optional": true}, {"name": "deltaY", "type": "number", "optional": true}]}],
			 "events": []},
			{"domain": "Network", "types": [
				{"id": "TimeSinceEpoch", "type": "number"},
				{"id": "MonotonicTime", "type": "number"},
				{"id": "Cookie", "type": "object", "properties": [
					{"name": "name", "type": "string"},
					{"name": "expires", "type": "number"},
					{"name": "sameParty", "type": "boolean", "deprecated": true, "description": "True if cookie is SameParty."}]}
			 ], "commands": [], "events": []},
			{"domain": "Fetch", "types": [{"id": "Blob", "type": "string", "description": "Raw bytes. (Encoded as a base64 string when passed over JSON)"}],
			 "commands": [{"name": "fulfillRequest", "parameters": [
				{"name": "body", "type": "string", "optional": true, "description": "A response body. (Encoded as a base64 string when passed over JSON)"}]}],
			 "events": []},
			{"domain": "Console", "deprecated": true, "description": "This domain is deprecated - use Runtime or Log instead.",
			 "types": [{"id": "ConsoleMessage", "type": "object", "properties": [{"name": "text", "type": "string"}]}],
			 "commands": [{"name": "enable"}], "events": []}
		]
	}`,

	"json/js_protocol.json": `{
		"version": {"major": "1", "minor": "3"},
		"domains": [
			{"domain": "Debugger", "types": [],
			 "commands": [{"name": "getWasmBytecode", "deprecated": true, "description": "This command is deprecated. Use getScriptSource instead.",
				"parameters": [{"name": "scriptId", "type": "string"}],
				"returns": [{"name": "bytecode", "type": "string", "description": "Script source. (Encoded as a base64 string when passed over JSON)"}]}],
			 "events": []}
		]
	}`,

	"pdl/browser_protocol.pdl": "version\n  major 1\n  minor 3\n\ninclude domains/Page.pdl\ninclude domains/Fetch.pdl\n",

	"pdl/domains/Page.pdl": strings.Join([]string{
		"domain Page",
		"  # Capture page screenshot.",
		"  command captureScreenshot",
		"    returns",
		"      # Base64-encoded image data.",
		"      binary data",
		"  command getManifestIcons",
		"    returns",
		"      optional binary primaryIcon",
		"  # Data is binary when the frame is a binary frame.",
		"  event screencastFrame",
		"    parameters",
		"      optional array of binary chunks",
		"",
	}, "\n"),

	"pdl/domains/Fetch.pdl": strings.Join([]string{
		"domain Fetch",
		"  # Raw bytes.",
		"  type Blob extends binary",
		"  command fulfillRequest",
		"    parameters",
		"      optional binary body",
		"",
	}, "\n"),

	"pdl/js_protocol.pdl": "domain Debugger\n  command getWasmBytecode\n    returns\n      binary bytecode\n",
}

// testBinaryFields is the hand-kept list for tagFiles: its two fields
// without a description.
var testBinaryFields = []binaryField{
	{"Page", "commands", "getManifestIcons", "returns", "primaryIcon"},
	{"Page", "events", "screencastFrame", "parameters", "chunks"},
}

// tagServer serves tagFiles the way raw.githubusercontent.com serves a tag:
// /<tag>/<path>, under the fixture's tag only.
func tagServer(t *testing.T) *httptest.Server {
	t.Helper()
	const tag = "v0.0.123"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content, has := tagFiles[strings.TrimPrefix(r.URL.Path, "/"+tag+"/")]
		if !has || !strings.HasPrefix(r.URL.Path, "/"+tag+"/") {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(content))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// tagDir writes tagFiles to a directory, as a checkout of the tag.
func tagDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range tagFiles {
		writeFile(t, filepath.Join(dir, filepath.FromSlash(name)), content)
	}
	return dir
}

func writeFile(t *testing.T, p, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
