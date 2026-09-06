package launcher

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/headlesslab/fetch"
	"github.com/headlesslab/wand/lib/launcher/pins"
	"github.com/headlesslab/wand/lib/utils"
	"github.com/ysmood/got"
)

// hostTemplate is the host template every test server serves under, so that a
// download resolves its version, platform and archive the way it would on a
// real Download host.
const hostTemplate = "/{version}/{platform}/{archive}"

// binaryContent stands in for the browser binary inside a test archive.
const binaryContent = "browser"

// newBrowser is a Browser of the Target Chrome, caching under a directory
// of the test's own, with its log silenced.
func newBrowser(t *testing.T) *Browser {
	t.Helper()

	b := NewBrowser()
	b.Source = SourceChrome
	b.Binary = BinaryChrome
	b.Logger = utils.LoggerQuiet
	b.RootDir = filepath.Join(t.TempDir(), "browser")

	return b
}

// archiveOf is the zip a Download host serves for b: one top-level folder
// holding the browser binary, as Chrome for Testing and Chromium archives
// nest theirs, with binaryContent as the binary. It returns the bytes and
// their SHA-256 in hex.
func archiveOf(g got.G, b *Browser) ([]byte, string) {
	rel, err := filepath.Rel(b.Dir(), b.BinPath())
	g.E(err)

	buf := bytes.NewBuffer(nil)
	z := zip.NewWriter(buf)
	f, err := z.Create("browser/" + filepath.ToSlash(rel))
	g.E(err)
	_, err = f.Write([]byte(binaryContent))
	g.E(err)
	g.E(z.Close())

	sum := sha256.Sum256(buf.Bytes())

	return buf.Bytes(), hex.EncodeToString(sum[:])
}

// pin records sum as the Target Chrome's archive hash for this platform, in
// place of the real pins, until the test ends.
func pin(g got.G, sum string) {
	cft, has := chromePlatforms[runtime.GOOS+"/"+runtime.GOARCH]
	if !has {
		g.Skip("Chrome for Testing has no build for this platform")
	}

	chromeSHA256 = map[string]map[string]string{string(BinaryChrome): {cft: sum}}
	g.Cleanup(func() { chromeSHA256 = pins.ChromeSHA256 })
}

func TestDownloadHosts(t *testing.T) {
	g := setup(t)

	chrome := DefaultHosts(SourceChrome)
	g.Len(chrome, 2)
	g.Has(chrome[0], "https://storage.googleapis.com/chrome-for-testing-public/")
	g.Has(chrome[1], "https://registry.npmmirror.com/-/binary/chrome-for-testing/")

	chromium := DefaultHosts(SourceChromium)
	g.Len(chromium, 2)
	g.Has(chromium[0], "https://storage.googleapis.com/chromium-browser-snapshots/")
	g.Has(chromium[1], "https://registry.npmmirror.com/-/binary/chromium-browser-snapshots/")

	for _, host := range append(chrome, chromium...) {
		for _, placeholder := range []string{"{version}", "{platform}", "{archive}"} {
			g.Desc(host).Has(host, placeholder)
		}
	}
}

func TestBrowserDefaults(t *testing.T) {
	g := setup(t)

	b := NewBrowser()
	g.Eq(b.Source, SourceChrome)
	g.Eq(b.Binary, BinaryChrome)
	g.Eq(b.Version, pins.ChromeVersion)
	g.Eq(b.Revision, pins.ChromiumPosition)
	g.Eq(b.RootDir, DefaultBrowserDir)
	g.Len(b.Hosts, 0)

	g.Eq(b.Dir(), filepath.Join(DefaultBrowserDir, "chrome-"+pins.ChromeVersion))

	b.Binary = BinaryHeadlessShell
	g.Eq(filepath.Base(b.Dir()), "chrome-headless-shell-"+pins.ChromeVersion)

	b.Source = SourceChromium
	g.Eq(filepath.Base(b.Dir()), fmt.Sprintf("chromium-%d", pins.ChromiumPosition))
}

func TestBrowserEnv(t *testing.T) {
	g := setup(t)

	t.Setenv(EnvBrowserSource, "chromium")
	t.Setenv(EnvBrowserBinary, "chrome-headless-shell")
	t.Setenv(EnvBrowserHosts, " https://a.example/{archive}, https://b.example/{archive} ,")

	b := NewBrowser()
	g.Eq(b.Source, SourceChromium)
	g.Eq(b.Binary, BinaryHeadlessShell)
	g.Eq(b.Hosts, []string{"https://a.example/{archive}", "https://b.example/{archive}"})
}

func TestDownload(t *testing.T) {
	g := setup(t)

	b := newBrowser(t)
	data, sum := archiveOf(g, b)
	pin(g, sum)

	s := g.Serve()
	s.Route("/", ".zip", data)
	b.Hosts = []string{s.URL(hostTemplate)}

	g.Eq(b.MustGet(), b.BinPath())

	content, err := os.ReadFile(b.BinPath())
	g.E(err)
	g.Eq(string(content), binaryContent)

	// In place already: nothing to download.
	g.E(b.Download())
}

func TestDownloadHashMismatch(t *testing.T) {
	g := setup(t)

	b := newBrowser(t)
	data, _ := archiveOf(g, b)
	pin(g, strings.Repeat("0", 64))

	s := g.Serve()
	s.Route("/", ".zip", data)
	b.Hosts = []string{s.URL(hostTemplate)}

	err := b.Download()
	var mismatch *fetch.HashMismatchError
	g.Desc("%v", err).True(errors.As(err, &mismatch))

	// Nothing was extracted.
	_, err = os.Stat(b.Dir())
	g.True(errors.Is(err, os.ErrNotExist))
}

func TestDownloadDeadHost(t *testing.T) {
	g := setup(t)

	b := newBrowser(t)
	data, sum := archiveOf(g, b)
	pin(g, sum)

	dead := g.Serve()
	dead.Mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "gone", http.StatusNotFound)
	})

	live := g.Serve()
	live.Route("/", ".zip", data)

	b.Hosts = []string{dead.URL(hostTemplate), live.URL(hostTemplate)}

	g.E(b.Download())
	g.PathExists(b.BinPath())
}

func TestDownloadUnverified(t *testing.T) {
	g := setup(t)

	// A version no pin records.
	b := newBrowser(t)
	b.Version = "0.0.0.1"
	data, _ := archiveOf(g, b)

	s := g.Serve()
	s.Route("/", ".zip", data)
	b.Hosts = []string{s.URL(hostTemplate)}

	logs := bytes.NewBuffer(nil)
	b.Logger = log.New(logs, "", 0)

	g.E(b.Download())
	g.PathExists(b.BinPath())
	g.Has(logs.String(), "chrome-0.0.0.1")
	g.Has(logs.String(), "not verified")
}

// TestGetReplacesBrokenCache: a cache directory that is there but does not
// validate is removed and downloaded again; one that is absent is not
// touched, so a browser another process lands meanwhile survives.
func TestGetReplacesBrokenCache(t *testing.T) {
	g := setup(t)

	b := newBrowser(t)
	b.Version = "0.0.0.1"
	data, _ := archiveOf(g, b)

	s := g.Serve()
	s.Route("/", ".zip", data)
	b.Hosts = []string{s.URL(hostTemplate)}

	stray := filepath.Join(b.Dir(), "stray")
	g.E(utils.OutputFile(stray, "no browser here"))

	p, err := b.Get()
	g.E(err)
	g.Eq(p, b.BinPath())
	g.PathExists(b.BinPath())
	_, err = os.Stat(stray)
	g.True(errors.Is(err, os.ErrNotExist))
}

func TestDownloadErr(t *testing.T) {
	g := setup(t)

	// Not an archive.
	b := newBrowser(t)
	b.Version = "0.0.0.1"
	s := g.Serve()
	s.Route("/", ".txt", "ok")
	b.Hosts = []string{s.URL(hostTemplate)}
	g.Err(b.Download())

	// An unknown source or binary.
	b = newBrowser(t)
	b.Source = "firefox"
	_, err := b.Get()
	g.Has(err.Error(), `unknown Browser source "firefox"`)

	b = newBrowser(t)
	b.Binary = "driver"
	g.Has(b.Download().Error(), `unknown Chrome for Testing binary "driver"`)

	b = newBrowser(t)
	b.Source = SourceChromium
	b.Binary = BinaryHeadlessShell
	g.Has(b.Download().Error(), "no chrome-headless-shell in Chromium trunk builds")
}
