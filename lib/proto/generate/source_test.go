package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/headlesslab/lazyjson"
)

func domainNames(s lazyjson.JSON) []string {
	names := []string{}
	for _, d := range s.Get("domains").Arr() {
		names = append(names, d.Get("domain").Str())
	}
	return names
}

var fixtureDomains = []string{"Target", "Page", "Input", "Network", "Fetch", "Console", "Debugger"}

func TestRemoteSchemaMergesBothFiles(t *testing.T) {
	g := setup(t)
	srv := tagServer(t)

	src := remoteSource(srv.Client(), srv.URL, 123)
	g.Eq(src.String(), "tag v0.0.123 of ChromeDevTools/devtools-protocol at "+srv.URL)

	s, err := src.schema(context.Background())
	g.E(err)
	g.Eq(domainNames(s), fixtureDomains)
	g.Eq(s.Get("version.major").Str(), "1")
	g.Eq(s.Get("version.minor").Str(), "3")
}

func TestRemoteSchemaMissingTag(t *testing.T) {
	g := setup(t)
	srv := tagServer(t)

	_, err := remoteSource(srv.Client(), srv.URL, 124).schema(context.Background())
	g.Err(err)
	g.Has(err.Error(), "/v0.0.124/json/browser_protocol.json")
	g.Has(err.Error(), "404")
}

func TestRemoteSchemaUnreachable(t *testing.T) {
	g := setup(t)
	srv := tagServer(t)
	srv.Close()

	_, err := remoteSource(srv.Client(), srv.URL, 123).schema(context.Background())
	g.Err(err)
}

func TestLocalSchema(t *testing.T) {
	g := setup(t)
	dir := tagDir(t)

	src, err := localSource(dir, 123)
	g.E(err)
	g.Eq(src.String(), dir)

	s, err := src.schema(context.Background())
	g.E(err)
	g.Eq(domainNames(s), fixtureDomains)
}

func TestLocalSchemaOtherRoll(t *testing.T) {
	g := setup(t)
	dir := tagDir(t)

	_, err := localSource(dir, 124)
	g.Err(err)
	g.Has(err.Error(), "0.0.123")
	g.Has(err.Error(), "v0.0.124")

	_, err = localSource(filepath.Join(dir, "missing"), 123)
	g.Err(err)

	writeFile(t, filepath.Join(dir, "package.json"), "not json")
	_, err = localSource(dir, 123)
	g.Err(err)
}

func TestLocalSchemaMissingFile(t *testing.T) {
	g := setup(t)
	dir := tagDir(t)
	src, err := localSource(dir, 123)
	g.E(err)

	g.E(os.Remove(filepath.Join(dir, "json", "js_protocol.json")))
	_, err = src.schema(context.Background())
	g.Err(err)

	g.E(os.Remove(filepath.Join(dir, "json", "browser_protocol.json")))
	_, err = src.schema(context.Background())
	g.Err(err)
}

func TestSchemaWithoutDomains(t *testing.T) {
	g := setup(t)
	dir := tagDir(t)
	src, err := localSource(dir, 123)
	g.E(err)

	writeFile(t, filepath.Join(dir, "json", "js_protocol.json"), `{"version": {}}`)
	_, err = src.schema(context.Background())
	g.Err(err)
	g.Has(err.Error(), "js_protocol.json")

	writeFile(t, filepath.Join(dir, "json", "browser_protocol.json"), `{"version": {}}`)
	_, err = src.schema(context.Background())
	g.Err(err)
	g.Has(err.Error(), "browser_protocol.json")
}

func TestPDLFollowsIncludes(t *testing.T) {
	g := setup(t)
	srv := tagServer(t)

	files, err := remoteSource(srv.Client(), srv.URL, 123).pdl(context.Background())
	g.E(err)

	paths := []string{}
	for _, f := range files {
		paths = append(paths, f.path)
	}
	g.Eq(paths, []string{
		"pdl/browser_protocol.pdl",
		"pdl/domains/Page.pdl",
		"pdl/domains/Fetch.pdl",
		"pdl/js_protocol.pdl",
	})
	g.Eq(files[1].text, tagFiles["pdl/domains/Page.pdl"])
}

func TestPDLMissing(t *testing.T) {
	g := setup(t)
	dir := tagDir(t)
	src, err := localSource(dir, 123)
	g.E(err)

	g.E(os.Remove(filepath.Join(dir, "pdl", "domains", "Fetch.pdl")))
	_, err = src.pdl(context.Background())
	g.Err(err)
	g.Has(err.Error(), "Fetch.pdl")

	g.E(os.Remove(filepath.Join(dir, "pdl", "browser_protocol.pdl")))
	_, err = src.pdl(context.Background())
	g.Err(err)
	g.Has(err.Error(), "browser_protocol.pdl")
}

func TestBinaryOccurrences(t *testing.T) {
	g := setup(t)
	dir := tagDir(t)

	src, err := localSource(dir, 123)
	g.E(err)
	files, err := src.pdl(context.Background())
	g.E(err)

	// The comment lines mentioning binary do not count; the fields, the
	// array item, the type alias and the JS field do.
	g.Eq(binaryOccurrences(files), []string{
		"pdl/domains/Page.pdl:6: binary data",
		"pdl/domains/Page.pdl:9: optional binary primaryIcon",
		"pdl/domains/Page.pdl:13: optional array of binary chunks",
		"pdl/domains/Fetch.pdl:3: type Blob extends binary",
		"pdl/domains/Fetch.pdl:6: optional binary body",
		"pdl/js_protocol.pdl:4: binary bytecode",
	})
}
