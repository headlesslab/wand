package launcher_test

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
	"strings"
	"testing"

	"github.com/headlesslab/fetch"
	"github.com/headlesslab/wand/lib/launcher"
	"github.com/headlesslab/wand/lib/launcher/pins"
	"github.com/headlesslab/wand/lib/utils"
	"github.com/ysmood/got"
)

// template is the host template every test server serves under, so that a
// download resolves its version, platform and archive the way it would on a
// real Download host.
const template = "/{version}/{platform}/{archive}"

// newBrowser is a Browser of a version no pin records, caching under a
// directory of the test's own, with its log silenced.
func newBrowser(t *testing.T) *launcher.Browser {
	t.Helper()

	b := launcher.NewBrowser()
	b.Logger = utils.LoggerQuiet
	b.RootDir = filepath.Join(t.TempDir(), "browser")
	b.Version = "0.0.0.1"

	return b
}

// binaryContent stands in for the browser binary inside a test archive.
const binaryContent = "browser"

// archiveOf is the zip a Download host serves for b: one top-level folder
// holding the browser binary, as Chrome for Testing and Chromium archives
// nest theirs, with binaryContent as the binary. It returns the bytes and
// their SHA-256 in hex.
func archiveOf(g got.G, b *launcher.Browser) ([]byte, string) {
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

func TestDownloadHosts(t *testing.T) {
	g := setup(t)

	chrome := launcher.DefaultHosts(launcher.SourceChrome)
	g.Len(chrome, 2)
	g.Has(chrome[0], "https://storage.googleapis.com/chrome-for-testing-public/")
	g.Has(chrome[1], "https://registry.npmmirror.com/-/binary/chrome-for-testing/")

	chromium := launcher.DefaultHosts(launcher.SourceChromium)
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

	b := launcher.NewBrowser()
	g.Eq(b.Source, launcher.SourceChrome)
	g.Eq(b.Binary, launcher.BinaryChrome)
	g.Eq(b.Version, pins.ChromeVersion)
	g.Eq(b.Revision, pins.ChromiumPosition)
	g.Eq(b.RootDir, launcher.DefaultBrowserDir)
	g.Len(b.Hosts, 0)
	g.Eq(b.SHA256, "")

	g.Eq(b.Dir(), filepath.Join(launcher.DefaultBrowserDir, "chrome-"+pins.ChromeVersion))

	b.Binary = launcher.BinaryHeadlessShell
	g.Eq(filepath.Base(b.Dir()), "chrome-headless-shell-"+pins.ChromeVersion)

	b.Source = launcher.SourceChromium
	g.Eq(filepath.Base(b.Dir()), fmt.Sprintf("chromium-%d", pins.ChromiumPosition))
}

func TestBrowserEnv(t *testing.T) {
	g := setup(t)

	t.Setenv(launcher.EnvBrowserCache, filepath.Join("some", "cache"))
	t.Setenv(launcher.EnvBrowserSource, "chromium")
	t.Setenv(launcher.EnvBrowserBinary, "chrome-headless-shell")
	t.Setenv(launcher.EnvBrowserHosts, " https://a.example/{archive}, https://b.example/{archive} ,")

	b := launcher.NewBrowser()
	g.Eq(b.RootDir, filepath.Join("some", "cache"))
	g.Eq(b.Source, launcher.SourceChromium)
	g.Eq(b.Binary, launcher.BinaryHeadlessShell)
	g.Eq(b.Hosts, []string{"https://a.example/{archive}", "https://b.example/{archive}"})
}

func TestDownload(t *testing.T) {
	g := setup(t)

	b := newBrowser(t)
	data, sum := archiveOf(g, b)

	s := g.Serve()
	s.Route("/", ".zip", data)
	b.Hosts = []string{s.URL(template)}
	b.SHA256 = sum

	g.Eq(b.MustGet(), b.BinPath())

	content, err := os.ReadFile(b.BinPath())
	g.E(err)
	g.Eq(string(content), binaryContent)
}

func TestDownloadHashMismatch(t *testing.T) {
	g := setup(t)

	b := newBrowser(t)
	data, _ := archiveOf(g, b)

	s := g.Serve()
	s.Route("/", ".zip", data)
	b.Hosts = []string{s.URL(template)}
	b.SHA256 = strings.Repeat("0", 64)

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

	dead := g.Serve()
	dead.Mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "gone", http.StatusNotFound)
	})

	live := g.Serve()
	live.Route("/", ".zip", data)

	b.Hosts = []string{dead.URL(template), live.URL(template)}
	b.SHA256 = sum

	g.E(b.Download())
	g.PathExists(b.BinPath())
}

func TestDownloadUnverified(t *testing.T) {
	g := setup(t)

	b := newBrowser(t)
	data, _ := archiveOf(g, b)

	s := g.Serve()
	s.Route("/", ".zip", data)
	b.Hosts = []string{s.URL(template)}

	logs := bytes.NewBuffer(nil)
	b.Logger = log.New(logs, "", 0)

	g.E(b.Download())
	g.PathExists(b.BinPath())
	g.Has(logs.String(), "chrome-0.0.0.1")
	g.Has(logs.String(), "not verified")
}

func TestDownloadErr(t *testing.T) {
	g := setup(t)

	// Not an archive.
	b := newBrowser(t)
	s := g.Serve()
	s.Route("/", ".txt", "ok")
	b.Hosts = []string{s.URL(template)}
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
	b.Source = launcher.SourceChromium
	b.Binary = launcher.BinaryHeadlessShell
	g.Has(b.Download().Error(), "no chrome-headless-shell in Chromium trunk builds")
}
