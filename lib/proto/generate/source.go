package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/headlesslab/lazyjson"
)

// protocolRaw serves any file of any tag of ChromeDevTools/devtools-protocol
// as <protocolRaw>/<tag>/<path>.
const protocolRaw = "https://raw.githubusercontent.com/ChromeDevTools/devtools-protocol"

// source reads the files of one devtools-protocol tag, from GitHub or from a
// local checkout of that tag.
type source struct {
	tag    string       // v0.0.<roll>
	base   string       // the raw URL prefix, or the checkout directory
	client *http.Client // nil for a checkout
}

// remoteSource reads tag v0.0.<roll> from raw, GitHub's raw file host in
// production and a test server otherwise.
func remoteSource(client *http.Client, raw string, roll int) source {
	return source{tag: fmt.Sprintf("v0.0.%d", roll), base: raw, client: client}
}

// localSource reads a checkout of devtools-protocol, for offline use. The
// checkout must be at tag v0.0.<roll>: its package.json carries the roll as
// the package version, and a checkout of another roll is refused, so the
// generated code never claims a roll it was not generated from.
func localSource(dir string, roll int) (source, error) {
	src := source{tag: fmt.Sprintf("v0.0.%d", roll), base: dir}

	data, err := src.read(context.Background(), "package.json")
	if err != nil {
		return source{}, err
	}
	var pkg struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return source{}, fmt.Errorf("%s: package.json: %w", dir, err)
	}
	if pkg.Version != fmt.Sprintf("0.0.%d", roll) {
		return source{}, fmt.Errorf("%s is devtools-protocol %s, not a checkout of tag %s", dir, pkg.Version, src.tag)
	}

	return src, nil
}

func (s source) String() string {
	if s.client == nil {
		return s.base
	}
	return fmt.Sprintf("tag %s of ChromeDevTools/devtools-protocol at %s", s.tag, s.base)
}

// read returns one file of the tag by its path in the repository, such as
// json/browser_protocol.json.
func (s source) read(ctx context.Context, p string) ([]byte, error) {
	if s.client == nil {
		return os.ReadFile(filepath.Join(s.base, filepath.FromSlash(p)))
	}

	u := s.base + "/" + s.tag + "/" + p
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	res, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", u, res.Status)
	}

	return io.ReadAll(res.Body)
}

// schema is the browser and JS JSON of the tag merged into one document, the
// same content a browser serves at /json/protocol: the JS domains follow the
// browser domains.
func (s source) schema(ctx context.Context) (lazyjson.JSON, error) {
	browser, err := s.read(ctx, "json/browser_protocol.json")
	if err != nil {
		return lazyjson.JSON{}, err
	}
	js, err := s.read(ctx, "json/js_protocol.json")
	if err != nil {
		return lazyjson.JSON{}, err
	}

	merged := lazyjson.New(browser)
	domains, ok := merged.Get("domains").Val().([]interface{})
	if !ok {
		return lazyjson.JSON{}, errors.New("json/browser_protocol.json has no domains array")
	}
	jsDomains, ok := lazyjson.New(js).Get("domains").Val().([]interface{})
	if !ok {
		return lazyjson.JSON{}, errors.New("json/js_protocol.json has no domains array")
	}
	merged.Set("domains", append(domains, jsDomains...))

	return merged, nil
}

// pdlFile is one PDL file of the tag.
type pdlFile struct {
	path string // in the repository, such as pdl/domains/Page.pdl
	text string
}

// pdl reads the browser and JS PDL of the tag and every file the browser
// PDL includes, in include order.
func (s source) pdl(ctx context.Context) ([]pdlFile, error) {
	files := []pdlFile{}
	for _, root := range []string{"pdl/browser_protocol.pdl", "pdl/js_protocol.pdl"} {
		text, err := s.read(ctx, root)
		if err != nil {
			return nil, err
		}
		files = append(files, pdlFile{root, string(text)})

		for _, inc := range includes(string(text)) {
			p := path.Join(path.Dir(root), inc)
			text, err := s.read(ctx, p)
			if err != nil {
				return nil, err
			}
			files = append(files, pdlFile{p, string(text)})
		}
	}
	return files, nil
}

// includes lists the files a PDL file includes, as written after the
// include keyword, relative to the including file.
func includes(text string) []string {
	list := []string{}
	for _, line := range strings.Split(text, "\n") {
		if inc, found := strings.CutPrefix(strings.TrimSpace(line), "include "); found {
			list = append(list, strings.TrimSpace(inc))
		}
	}
	return list
}

// binaryOccurrences lists every use of the binary type in the PDL files as
// "<path>:<line>: <text>". PDL comment lines start with #; on every other
// line a binary token is a type: of a parameter, a property, a return value,
// an array item or a type alias. The generator must produce exactly one
// []byte for each (ADR-0004).
func binaryOccurrences(files []pdlFile) []string {
	list := []string{}
	for _, f := range files {
		for i, line := range strings.Split(f.text, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "#") {
				continue
			}
			for _, token := range strings.Fields(trimmed) {
				if token == "binary" {
					list = append(list, fmt.Sprintf("%s:%d: %s", f.path, i+1, trimmed))
				}
			}
		}
	}
	return list
}
