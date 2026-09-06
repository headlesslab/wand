package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/format"
	"go/parser"
	"go/token"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/ysmood/got"
)

var setup = got.Setup(nil)

// bucket fakes the three Google endpoints the Roll reads: Chrome for
// Testing's version JSON, its archive bucket and the Chromium snapshots
// bucket with its listing API. Every archive's content is its own URL path,
// so the expected hash of an archive is the hash of its path.
type bucket struct {
	stable    string
	versions  map[string]int   // Chrome for Testing version -> branch position
	positions map[string][]int // Chromium bucket prefix -> positions listed
	missing   map[string]bool  // URL path -> absent (404)
	pageSize  int              // listing page size, to exercise pagination

	mu       sync.Mutex
	requests []string // method + path of every request, in order
}

func (b *bucket) handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/cft/last-known-good-versions.json", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]map[string]map[string]string{
			"channels": {
				"Stable": {
					"channel":  "Stable",
					"version":  b.stable,
					"revision": strconv.Itoa(b.versions[b.stable]),
				},
			},
		})
	})

	mux.HandleFunc("/cft/known-good-versions.json", func(w http.ResponseWriter, _ *http.Request) {
		list := []map[string]string{}
		for version, position := range b.versions {
			list = append(list, map[string]string{"version": version, "revision": strconv.Itoa(position)})
		}
		_ = json.NewEncoder(w).Encode(map[string][]map[string]string{"versions": list})
	})

	mux.HandleFunc("/api/o", b.list)

	archive := func(w http.ResponseWriter, r *http.Request) {
		if b.missing[r.URL.Path] {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(r.URL.Path)))
		_, _ = io.WriteString(w, r.URL.Path)
	}
	mux.HandleFunc("/cft-bucket/", archive)
	mux.HandleFunc("/snapshots/", archive)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b.mu.Lock()
		b.requests = append(b.requests, r.Method+" "+r.URL.Path)
		b.mu.Unlock()
		mux.ServeHTTP(w, r)
	})
}

// list answers the JSON API's prefix listing with the lexicographic window
// semantics of the real one: names at or after startOffset and before
// endOffset, paged by pageToken.
func (b *bucket) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	prefix := strings.TrimSuffix(q.Get("prefix"), "/")
	names := []string{}
	for _, position := range b.positions[prefix] {
		name := fmt.Sprintf("%s/%d/", prefix, position)
		if name >= q.Get("startOffset") && name < q.Get("endOffset") {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	from := 0
	if token := q.Get("pageToken"); token != "" {
		from, _ = strconv.Atoi(token)
	}
	to := from + b.pageSize
	var res struct {
		Prefixes      []string `json:"prefixes"`
		NextPageToken string   `json:"nextPageToken,omitempty"`
	}
	if to < len(names) {
		res.NextPageToken = strconv.Itoa(to)
	} else {
		to = len(names)
	}
	res.Prefixes = names[from:to]
	if err := json.NewEncoder(w).Encode(res); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (b *bucket) count(method, path string) int {
	b.mu.Lock()
	defer b.mu.Unlock()

	n := 0
	for _, req := range b.requests {
		if req == method+" "+path {
			n++
		}
	}
	return n
}

func newBucket() *bucket {
	return &bucket{
		stable: "152.0.7977.82",
		versions: map[string]int{
			"152.0.7977.82": 1669021,
			"153.0.8010.27": 1681091,
		},
		positions: map[string][]int{
			// 166850 is lexicographically inside the window but numerically
			// far below it; 1669100 is above the branch position.
			"Linux_x64": {166850, 1660000, 1668000, 1668500, 1669021, 1669100},
			"Mac":       {1660000, 1668000, 1668500},
			"Mac_Arm":   {1660000, 1668000, 1668500, 1669021},
			"Win":       {1660000, 1668000, 1668500},
			"Win_x64":   {1660000, 1668000, 1668500},
		},
		// 1668500 is listed for every prefix, but its Win archive is gone, so
		// the Companion Chromium must fall back to 1668000.
		missing:  map[string]bool{"/snapshots/Win/1668500/chrome-win.zip": true},
		pageSize: 2,
	}
}

func testRoller(t *testing.T, b *bucket, rolls []int) *roller {
	t.Helper()

	srv := httptest.NewServer(b.handler())
	t.Cleanup(srv.Close)

	return &roller{
		client:          srv.Client(),
		cft:             srv.URL + "/cft",
		chromeBucket:    srv.URL + "/cft-bucket",
		chromiumBucket:  srv.URL + "/snapshots",
		chromiumListing: srv.URL + "/api/o",
		rolls:           func(context.Context) ([]int, error) { return rolls, nil },
		parallel:        3,
		log:             log.New(io.Discard, "", 0),
	}
}

func hashOf(path string) string {
	sum := sha256.Sum256([]byte(path))
	return hex.EncodeToString(sum[:])
}

func TestRoll(t *testing.T) {
	g := setup(t)

	b := newBucket()
	s := testRoller(t, b, []int{1669207, 1666840, 1663043})

	p, missing, err := s.roll(context.Background(), "")
	g.E(err)
	g.Len(missing, 0)

	g.Eq(p.ChromeVersion, "152.0.7977.82")
	g.Eq(p.ChromePosition, 1669021)
	g.Eq(p.ProtocolRoll, 1666840)
	g.Eq(p.ChromiumPosition, 1668000)

	g.Len(p.ChromeSHA256, 2)
	for _, binary := range chromeBinaries {
		g.Len(p.ChromeSHA256[binary], len(chromePlatforms))
		for _, platform := range chromePlatforms {
			path := fmt.Sprintf("/cft-bucket/152.0.7977.82/%s/%s-%s.zip", platform, binary, platform)
			g.Desc(path).Eq(p.ChromeSHA256[binary][platform], hashOf(path))
			g.Desc(path).Eq(b.count(http.MethodGet, path), 1)
		}
	}

	g.Eq(p.ChromiumSHA256, map[string]string{
		"Linux_x64": hashOf("/snapshots/Linux_x64/1668000/chrome-linux.zip"),
		"Mac":       hashOf("/snapshots/Mac/1668000/chrome-mac.zip"),
		"Mac_Arm":   hashOf("/snapshots/Mac_Arm/1668000/chrome-mac.zip"),
		"Win":       hashOf("/snapshots/Win/1668000/chrome-win.zip"),
		"Win_x64":   hashOf("/snapshots/Win_x64/1668000/chrome-win.zip"),
	})

	// The rejected candidate was probed, never downloaded.
	g.Eq(b.count(http.MethodHead, "/snapshots/Win/1668500/chrome-win.zip"), 1)
	g.Eq(b.count(http.MethodGet, "/snapshots/Win/1668500/chrome-win.zip"), 0)
	g.Eq(b.count(http.MethodGet, "/snapshots/Linux_x64/1669021/chrome-linux.zip"), 0)
}

func TestRollMissingArchive(t *testing.T) {
	g := setup(t)

	b := newBucket()
	gone := "/cft-bucket/152.0.7977.82/linux-arm64/chrome-headless-shell-linux-arm64.zip"
	b.missing[gone] = true
	s := testRoller(t, b, []int{1666840})

	p, missing, err := s.roll(context.Background(), "")
	g.E(err)
	g.Len(missing, 1)
	g.Has(missing[0], gone)

	// Everything that exists is still recorded.
	_, has := p.ChromeSHA256["chrome-headless-shell"]["linux-arm64"]
	g.False(has)
	g.Len(p.ChromeSHA256["chrome-headless-shell"], 5)
	g.Len(p.ChromeSHA256["chrome"], 6)
	g.Len(p.ChromiumSHA256, 5)
}

func TestRollVersionArgument(t *testing.T) {
	g := setup(t)

	b := newBucket()
	b.positions["Linux_x64"] = append(b.positions["Linux_x64"], 1681000)
	for _, prefix := range []string{"Mac", "Mac_Arm", "Win", "Win_x64"} {
		b.positions[prefix] = append(b.positions[prefix], 1681000)
	}
	s := testRoller(t, b, []int{1681094, 1681000, 1666840})

	p, missing, err := s.roll(context.Background(), "153.0.8010.27")
	g.E(err)
	g.Len(missing, 0)
	g.Eq(p.ChromeVersion, "153.0.8010.27")
	g.Eq(p.ChromePosition, 1681091)
	g.Eq(p.ProtocolRoll, 1681000)
	g.Eq(p.ChromiumPosition, 1681000)
	g.Eq(p.ChromeSHA256["chrome"]["win64"], hashOf("/cft-bucket/153.0.8010.27/win64/chrome-win64.zip"))
	g.Eq(b.count(http.MethodGet, "/cft/last-known-good-versions.json"), 0)

	_, _, err = s.roll(context.Background(), "1.2.3.4")
	g.Err(err)
	g.Has(err.Error(), "1.2.3.4")
	g.Has(err.Error(), "Chrome for Testing")
}

func TestRollErrors(t *testing.T) {
	g := setup(t)

	// No roll at or below the branch position.
	s := testRoller(t, newBucket(), []int{1669207})
	_, _, err := s.roll(context.Background(), "")
	g.Err(err)
	g.Has(err.Error(), "1669021")

	// No Chromium trunk build common to the five prefixes anywhere below the
	// branch position: the search widens to position zero, then fails.
	b := newBucket()
	b.positions["Win"] = []int{1700000}
	s = testRoller(t, b, []int{1666840})
	_, _, err = s.roll(context.Background(), "")
	g.Err(err)
	g.Has(err.Error(), "Win")
	g.Has(err.Error(), "1669021")
	g.Gt(b.count(http.MethodGet, "/api/o"), 5)

	// The tag source failing.
	s = testRoller(t, newBucket(), nil)
	s.rolls = func(context.Context) ([]int, error) { return nil, fmt.Errorf("git: boom") }
	_, _, err = s.roll(context.Background(), "")
	g.Err(err)
	g.Has(err.Error(), "boom")

	// Every endpoint unreachable.
	s = testRoller(t, newBucket(), []int{1666840})
	s.cft = "http://127.0.0.1:1/cft"
	_, _, err = s.roll(context.Background(), "")
	g.Err(err)
}

func TestProtocolRoll(t *testing.T) {
	g := setup(t)

	rolls := []int{10, 1, 5}

	roll, err := protocolRoll(rolls, 7)
	g.E(err)
	g.Eq(roll, 5)

	roll, err = protocolRoll(rolls, 10)
	g.E(err)
	g.Eq(roll, 10)

	roll, err = protocolRoll(rolls, 100)
	g.E(err)
	g.Eq(roll, 10)

	_, err = protocolRoll(rolls, 0)
	g.Err(err)

	_, err = protocolRoll(nil, 7)
	g.Err(err)
}

func TestParseRolls(t *testing.T) {
	g := setup(t)

	out := strings.Join([]string{
		"a3a137c5184da59ffaac03e426651e0a9ab91470\trefs/tags/v0.1",
		"6fe72ec7cb3d1c3b1fc4e0b2b0a5d9b3b3f2f1a0\trefs/tags/v0.0.1669207",
		"0539a3c0cb3d1c3b1fc4e0b2b0a5d9b3b3f2f1a0\trefs/tags/v0.0.1681094",
		"da569ab18bb3e66ccd01b314b3db07e768cb6422\trefs/tags/v1.0",
		"deadbeef\trefs/tags/v0.0.abc",
		"",
	}, "\n")

	g.Eq(parseRolls(out), []int{1669207, 1681094})
	g.Eq(parseRolls(""), []int{})
}

func TestCommonPositions(t *testing.T) {
	g := setup(t)

	g.Eq(commonPositions([][]int{{1, 2, 3, 5}, {5, 2, 9}, {2, 5, 3}}), []int{5, 2})
	g.Eq(commonPositions([][]int{{1, 2}, {3}}), []int{})
	g.Eq(commonPositions([][]int{{4, 4, 2}}), []int{4, 2})
	g.Eq(commonPositions(nil), []int{})
}

func samplePins() Pins {
	return Pins{
		ChromeVersion:    "152.0.7977.82",
		ChromePosition:   1669021,
		ProtocolRoll:     1666840,
		ChromiumPosition: 1668000,
		ChromeSHA256: map[string]map[string]string{
			"chrome": {
				"linux64":     hashOf("a"),
				"linux-arm64": hashOf("b"),
				"win64":       hashOf("c"),
			},
			"chrome-headless-shell": {
				"linux64": hashOf("d"),
			},
		},
		ChromiumSHA256: map[string]string{
			"Win":       hashOf("e"),
			"Linux_x64": hashOf("f"),
		},
	}
}

func TestRender(t *testing.T) {
	g := setup(t)

	out, err := render(samplePins())
	g.E(err)

	again, err := render(samplePins())
	g.E(err)
	g.Eq(string(again), string(out))

	// Well-formed, gofmt-stable Go with the standard generated-file marker.
	_, err = parser.ParseFile(token.NewFileSet(), "pins.go", out, parser.AllErrors)
	g.E(err)
	formatted, err := format.Source(out)
	g.E(err)
	g.Eq(string(formatted), string(out))
	g.True(bytes.HasPrefix(out, []byte("// Code generated by lib/launcher/pins/generate; DO NOT EDIT.")))
	g.Has(string(out), "\npackage pins\n")

	g.Has(string(out), `const ChromeVersion = "152.0.7977.82"`)
	g.Has(string(out), "const ChromePosition = 1669021")
	g.Has(string(out), "const ProtocolRoll = 1666840")
	g.Has(string(out), "const ChromiumPosition = 1668000")

	// Keys in sorted order, so the same pins always give the same bytes.
	src := string(out)
	g.Lt(strings.Index(src, `"chrome":`), strings.Index(src, `"chrome-headless-shell":`))
	g.Lt(strings.Index(src, `"linux-arm64"`), strings.Index(src, `"linux64"`))
	g.Lt(strings.Index(src, `"linux64"`), strings.Index(src, `"win64"`))
	g.Lt(strings.Index(src, `"Linux_x64"`), strings.Index(src, `"Win"`))
	// Values aligned by gofmt, so any run of spaces after the colon.
	g.Regex(`"win64": +"`+hashOf("c")+`",`, src)
	g.Regex(`"Win": +"`+hashOf("e")+`",`, src)
}

// writeOutputs gives a temporary module root the three outputs for p: pins.go
// as rendered, and two READMEs whose block the Roll owns.
func writeOutputs(t *testing.T, p Pins) string {
	t.Helper()

	root := t.TempDir()
	g := setup(t)

	g.E(os.MkdirAll(filepath.Join(root, "lib", "launcher", "pins"), 0o755))
	skeleton := "# wand\n\nprose\n\n<!-- pins:begin -->\n<!-- pins:end -->\n\n## Roadmap\n"
	g.E(os.WriteFile(filepath.Join(root, "README.md"), []byte(skeleton), 0o644))
	g.E(os.WriteFile(filepath.Join(root, "README.zh-CN.md"), []byte(skeleton), 0o644))
	g.E(os.WriteFile(filepath.Join(root, pinsFile), []byte("placeholder"), 0o644))

	r := &roller{root: root}
	g.E(r.write(p))

	return root
}

func TestWrite(t *testing.T) {
	g := setup(t)

	p := samplePins()
	root := writeOutputs(t, p)

	file, err := os.ReadFile(filepath.Join(root, pinsFile))
	g.E(err)
	want, err := render(p)
	g.E(err)
	g.Eq(string(file), string(want))

	readme, err := os.ReadFile(filepath.Join(root, "README.md"))
	g.E(err)
	block, err := readmeBlock(p, "README.md")
	g.E(err)
	g.Eq(string(readme), "# wand\n\nprose\n\n<!-- pins:begin -->\n"+block+"<!-- pins:end -->\n\n## Roadmap\n")

	// Writing again changes nothing.
	r := &roller{root: root}
	g.E(r.write(p))
	again, err := os.ReadFile(filepath.Join(root, "README.md"))
	g.E(err)
	g.Eq(string(again), string(readme))

	// A README without the markers cannot take the table.
	g.E(os.WriteFile(filepath.Join(root, "README.zh-CN.md"), []byte("no markers\n"), 0o644))
	err = r.write(p)
	g.Err(err)
	g.Has(err.Error(), "README.zh-CN.md")
}

func TestCheck(t *testing.T) {
	g := setup(t)

	p := samplePins()
	root := writeOutputs(t, p)

	s := testRoller(t, newBucket(), []int{1669207, 1666840, 1663043})
	s.root = root
	g.E(s.check(context.Background(), p))

	// A Windows checkout with autocrlf must pass too.
	for _, name := range []string{pinsFile, "README.md"} {
		file, err := os.ReadFile(filepath.Join(root, name))
		g.E(err)
		crlf := bytes.ReplaceAll(file, []byte("\n"), []byte("\r\n"))
		g.E(os.WriteFile(filepath.Join(root, name), crlf, 0o644))
	}
	g.E(s.check(context.Background(), p))

	// The branch position derives another roll than the one recorded.
	s.rolls = func(context.Context) ([]int, error) { return []int{1666022, 1669207}, nil }
	err := s.check(context.Background(), p)
	g.Err(err)
	g.Has(err.Error(), "r1666840")
	g.Has(err.Error(), "r1666022")
	s.rolls = func(context.Context) ([]int, error) { return []int{1666840}, nil }

	// pins.go is not byte for byte what the Roll writes for its own values:
	// a hand edit gofmt tolerates, here an extra blank line.
	file, err := os.ReadFile(filepath.Join(root, pinsFile))
	g.E(err)
	edited := bytes.Replace(file, []byte("\r\n\r\n"), []byte("\r\n\r\n\r\n"), 1)
	g.E(os.WriteFile(filepath.Join(root, pinsFile), edited, 0o644))
	err = s.check(context.Background(), p)
	g.Err(err)
	g.Has(err.Error(), pinsFile)
	g.E(os.WriteFile(filepath.Join(root, pinsFile), file, 0o644))
	g.E(s.check(context.Background(), p))

	// A README table edited by hand.
	readme, err := os.ReadFile(filepath.Join(root, "README.zh-CN.md"))
	g.E(err)
	g.E(os.WriteFile(filepath.Join(root, "README.zh-CN.md"), bytes.Replace(readme, []byte("✅"), []byte("❌"), 1), 0o644))
	err = s.check(context.Background(), p)
	g.Err(err)
	g.Has(err.Error(), "README.zh-CN.md")

	// The tag source failing.
	s.rolls = func(context.Context) ([]int, error) { return nil, fmt.Errorf("git: boom") }
	err = s.check(context.Background(), p)
	g.Err(err)
	g.Has(err.Error(), "boom")
}

// TestCommittedOutputs is the zero-diff property on the files actually
// committed: rendering the pins package's own values reproduces pins.go and
// both README tables byte for byte.
func TestCommittedOutputs(t *testing.T) {
	g := setup(t)

	r := &roller{root: filepath.Join("..", "..", "..", "..")}
	g.E(r.verify(current()))
}

func TestParseArgs(t *testing.T) {
	g := setup(t)

	opts, err := parseArgs(nil)
	g.E(err)
	g.False(opts.check)
	g.Eq(opts.version, "")

	opts, err = parseArgs([]string{"152.0.7977.82"})
	g.E(err)
	g.False(opts.check)
	g.Eq(opts.version, "152.0.7977.82")

	opts, err = parseArgs([]string{"-check"})
	g.E(err)
	g.True(opts.check)
	g.False(opts.render)

	opts, err = parseArgs([]string{"-render"})
	g.E(err)
	g.True(opts.render)
	g.False(opts.check)

	for _, bad := range [][]string{
		{"-check", "152.0.7977.82"},
		{"-render", "152.0.7977.82"},
		{"-check", "-render"},
		{"152.0.7977.82", "153.0.8010.27"},
		{"152.0.7977"},
		{"v152.0.7977.82"},
		{"-bogus"},
	} {
		_, err = parseArgs(bad)
		g.Desc("%q", bad).Err(err)
	}
}

func TestUsage(t *testing.T) {
	g := setup(t)

	out := &bytes.Buffer{}
	usage(out)
	g.Has(out.String(), "go run ./lib/launcher/pins/generate")
	g.Has(out.String(), "-check")
	g.Has(out.String(), "-render")
}
