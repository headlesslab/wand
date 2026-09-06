package launcher

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/headlesslab/fetch"
	"github.com/headlesslab/wand/lib/launcher/pins"
	"github.com/headlesslab/wand/lib/utils"
)

// Source is a Browser source: the archive family a Managed browser is
// downloaded from.
type Source string

const (
	// SourceChrome is Chrome for Testing, the default: Google's build of the
	// Chrome stable branch, pinned to the Target Chrome.
	SourceChrome Source = "chrome"

	// SourceChromium is Chromium trunk builds from Google's continuous-build
	// archive, pinned to the Companion Chromium: BSD-licensed Chromium for
	// deployments that cannot accept Google Chrome's terms.
	SourceChromium Source = "chromium"
)

// Binary is the executable a Chrome for Testing archive holds.
type Binary string

const (
	// BinaryChrome is the full browser, the default.
	BinaryChrome Binary = "chrome"

	// BinaryHeadlessShell is chrome-headless-shell, the smaller build that
	// only runs headless.
	BinaryHeadlessShell Binary = "chrome-headless-shell"
)

// The environment variables NewBrowser reads. A launcher option set in code
// overrides them.
const (
	// EnvBrowserCache sets the browser cache, DefaultBrowserDir, when the
	// process starts.
	EnvBrowserCache = "WAND_BROWSER_CACHE"

	// EnvBrowserHosts overrides the Download hosts: URL templates separated
	// by commas, in the form DefaultHosts describes.
	EnvBrowserHosts = "WAND_BROWSER_HOSTS"

	// EnvBrowserSource selects the Browser source: "chrome" for Chrome for
	// Testing, the default, or "chromium" for Chromium trunk builds.
	EnvBrowserSource = "WAND_BROWSER_SOURCE"

	// EnvBrowserBinary selects the Chrome for Testing binary: "chrome", the
	// default, or "chrome-headless-shell".
	EnvBrowserBinary = "WAND_BROWSER_BINARY"
)

// DefaultHosts are the Download hosts of a Browser source: Google's bucket
// and npmmirror, as URL templates. A template names the archive with three
// placeholders: {version}, the Chrome for Testing version or the Chromium
// trunk position; {platform}, the Chrome for Testing platform such as
// linux64 or the Chromium bucket prefix such as Linux_x64; and {archive},
// the file name such as chrome-linux64.zip or chrome-linux.zip. Every host
// is probed concurrently, the first to answer serves the download and the
// others are its fallbacks.
func DefaultHosts(source Source) []string {
	if source == SourceChromium {
		return []string{
			"https://storage.googleapis.com/chromium-browser-snapshots/{platform}/{version}/{archive}",
			"https://registry.npmmirror.com/-/binary/chromium-browser-snapshots/{platform}/{version}/{archive}",
		}
	}

	return []string{
		"https://storage.googleapis.com/chrome-for-testing-public/{version}/{platform}/{archive}",
		"https://registry.npmmirror.com/-/binary/chrome-for-testing/{version}/{platform}/{archive}",
	}
}

// DefaultBrowserDir is the browser cache: EnvBrowserCache when set, otherwise
// wand/browser under [os.UserCacheDir] ($XDG_CACHE_HOME or ~/.cache on Linux,
// ~/Library/Caches on macOS, %LocalAppData% on Windows), or under the
// temporary directory for a user without a cache directory. Each Managed
// browser lives in its own subdirectory: chrome-<version>,
// chrome-headless-shell-<version> or chromium-<revision>.
var DefaultBrowserDir = cacheDir()

func cacheDir() string {
	if dir := os.Getenv(EnvBrowserCache); dir != "" {
		return dir
	}

	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}

	return filepath.Join(base, "wand", "browser")
}

// The pins the launcher verifies downloads against; variables so that tests
// can pin archives of their own.
var (
	chromeSHA256   = pins.ChromeSHA256
	chromiumSHA256 = pins.ChromiumSHA256
)

// chromePlatforms maps GOOS/GOARCH to the Chrome for Testing platform.
var chromePlatforms = map[string]string{
	"darwin/amd64":  "mac-x64",
	"darwin/arm64":  "mac-arm64",
	"linux/amd64":   "linux64",
	"linux/arm64":   "linux-arm64",
	"windows/386":   "win32",
	"windows/amd64": "win64",
}

// chromiumPlatforms maps GOOS/GOARCH to the prefix of the Chromium trunk
// build bucket and the archive under it.
var chromiumPlatforms = map[string]struct{ prefix, archive string }{
	"darwin/amd64":  {"Mac", "chrome-mac.zip"},
	"darwin/arm64":  {"Mac_Arm", "chrome-mac.zip"},
	"linux/amd64":   {"Linux_x64", "chrome-linux.zip"},
	"windows/386":   {"Win", "chrome-win.zip"},
	"windows/amd64": {"Win_x64", "chrome-win.zip"},
}

// binaries is the path of the executable inside an extracted archive, by
// source, binary and GOOS.
var binaries = map[Source]map[Binary]map[string]string{
	SourceChrome: {
		BinaryChrome: {
			"darwin":  "Google Chrome for Testing.app/Contents/MacOS/Google Chrome for Testing",
			"linux":   "chrome",
			"windows": "chrome.exe",
		},
		BinaryHeadlessShell: {
			"darwin":  "chrome-headless-shell",
			"linux":   "chrome-headless-shell",
			"windows": "chrome-headless-shell.exe",
		},
	},
	SourceChromium: {
		BinaryChrome: {
			"darwin":  "Chromium.app/Contents/MacOS/Chromium",
			"linux":   "chrome",
			"windows": "chrome.exe",
		},
	},
}

// muslLoaderGlob matches the dynamic loader of the musl C library, which
// Alpine and the other musl distributions install as /lib/ld-musl-<arch>.so.1.
// Every Managed browser is linked against glibc, so none runs there.
const muslLoaderGlob = "/lib/ld-musl-*.so.1"

func hasMuslLoader() bool {
	return runtime.GOOS == "linux" && muslLoaderPresent(muslLoaderGlob)
}

func muslLoaderPresent(glob string) bool {
	matches, _ := filepath.Glob(glob)

	return len(matches) > 0
}

// wayOut is how a platform with nothing to download still gets a browser.
const wayOut = "point WAND_BROWSER_BIN or Launcher.Bin() at a browser already on this machine"

// Browser is a helper to download a Managed browser: the Target Chrome from
// Chrome for Testing by default, or the Companion Chromium from Chromium
// trunk builds, verified against the pins and cached under RootDir.
type Browser struct {
	Context context.Context

	// Source of the archive: SourceChrome, the default, or SourceChromium.
	// EnvBrowserSource sets the default.
	Source Source

	// Binary of a Chrome for Testing archive: BinaryChrome, the default, or
	// BinaryHeadlessShell. A Chromium trunk build holds only BinaryChrome.
	// EnvBrowserBinary sets the default.
	Binary Binary

	// Version of Chrome for Testing to use, the Target Chrome by default.
	Version string

	// Revision is the trunk position of the Chromium trunk build to use, the
	// Companion Chromium by default.
	Revision int

	// Hosts to download the archive from, as the URL templates DefaultHosts
	// describes; empty means DefaultHosts of the Source. EnvBrowserHosts
	// sets the default.
	Hosts []string

	// RootDir is the browser cache, DefaultBrowserDir by default.
	RootDir string

	// Logger to print output
	Logger utils.Logger

	// HTTPClient to download the browser
	HTTPClient *http.Client
}

// NewBrowser with default values, the environment variables applied.
func NewBrowser() *Browser {
	b := &Browser{
		Context:  context.Background(),
		Source:   SourceChrome,
		Binary:   BinaryChrome,
		Version:  pins.ChromeVersion,
		Revision: pins.ChromiumPosition,
		RootDir:  DefaultBrowserDir,
		Logger:   log.New(os.Stdout, "[launcher.Browser] ", log.LstdFlags),
	}

	if source := os.Getenv(EnvBrowserSource); source != "" {
		b.Source = Source(source)
	}

	if binary := os.Getenv(EnvBrowserBinary); binary != "" {
		b.Binary = Binary(binary)
	}

	for _, host := range strings.Split(os.Getenv(EnvBrowserHosts), ",") {
		if host = strings.TrimSpace(host); host != "" {
			b.Hosts = append(b.Hosts, host)
		}
	}

	return b
}

// name of the Managed browser, which is its directory under RootDir:
// chrome-<version>, chrome-headless-shell-<version> or chromium-<revision>.
func (lc *Browser) name() string {
	if lc.Source == SourceChromium {
		return fmt.Sprintf("chromium-%d", lc.Revision)
	}

	return fmt.Sprintf("%s-%s", lc.Binary, lc.Version)
}

// Dir to download the browser.
func (lc *Browser) Dir() string {
	return filepath.Join(lc.RootDir, lc.name())
}

// BinPath to download the browser executable.
func (lc *Browser) BinPath() string {
	return lc.binPath(runtime.GOOS)
}

func (lc *Browser) binPath(goos string) string {
	return filepath.Join(lc.Dir(), filepath.FromSlash(binaries[lc.Source][lc.Binary][goos]))
}

// archive is what a Managed browser resolves to on one platform: what the
// host templates take and the digest to verify the download against, which
// the pins record for the Target Chrome and the Companion Chromium and
// nothing else.
type archive struct {
	platform string
	version  string
	name     string
	sha256   string
}

// url of the archive on a Download host.
func (a archive) url(template string) string {
	return strings.NewReplacer("{version}", a.version, "{platform}", a.platform, "{archive}", a.name).Replace(template)
}

// resolve finds the archive for a platform, given as GOOS/GOARCH, refusing
// the platforms with nothing to download: the ones no Browser source builds
// for, the ones the pins record no archive for, and Linux on the musl C
// library.
func (lc *Browser) resolve(platform string, musl bool) (archive, error) {
	if musl {
		return archive{}, fmt.Errorf("no Managed browser runs on %s with the musl C library (Alpine): %s", platform, wayOut)
	}

	switch lc.Source {
	case SourceChrome:
		return lc.resolveChrome(platform)
	case SourceChromium:
		return lc.resolveChromium(platform)
	default:
		return archive{}, fmt.Errorf("unknown Browser source %q: %q or %q", lc.Source, SourceChrome, SourceChromium)
	}
}

func (lc *Browser) resolveChrome(platform string) (archive, error) {
	cft, has := chromePlatforms[platform]
	if !has {
		return archive{}, fmt.Errorf("no Chrome for Testing build exists for %s: %s", platform, wayOut)
	}

	if _, has := binaries[SourceChrome][lc.Binary]; !has {
		return archive{}, fmt.Errorf("unknown Chrome for Testing binary %q: %q or %q",
			lc.Binary, BinaryChrome, BinaryHeadlessShell)
	}

	a := archive{platform: cft, version: lc.Version, name: fmt.Sprintf("%s-%s.zip", lc.Binary, cft)}

	if lc.Version == pins.ChromeVersion {
		sum, has := chromeSHA256[string(lc.Binary)][cft]
		if !has {
			return archive{}, fmt.Errorf("no %s %s archive is pinned for %s: %s", lc.Binary, lc.Version, platform, wayOut)
		}

		a.sha256 = sum
	}

	return a, nil
}

func (lc *Browser) resolveChromium(platform string) (archive, error) {
	bucket, has := chromiumPlatforms[platform]
	if !has {
		return archive{}, fmt.Errorf("no Chromium trunk build exists for %s: %s", platform, wayOut)
	}

	if lc.Binary != BinaryChrome {
		return archive{}, fmt.Errorf("no %s in Chromium trunk builds, only %s", lc.Binary, BinaryChrome)
	}

	a := archive{platform: bucket.prefix, version: strconv.Itoa(lc.Revision), name: bucket.archive}

	if lc.Revision == pins.ChromiumPosition {
		sum, has := chromiumSHA256[bucket.prefix]
		if !has {
			return archive{}, fmt.Errorf("no Chromium %d archive is pinned for %s: %s", lc.Revision, platform, wayOut)
		}

		a.sha256 = sum
	}

	return a, nil
}

// hosts is Hosts, or DefaultHosts of the Source when none is set.
func (lc *Browser) hosts() []string {
	if len(lc.Hosts) == 0 {
		return DefaultHosts(lc.Source)
	}

	return lc.Hosts
}

// Download the browser from the first of the Hosts to answer, verified
// against the pins before extraction, into [Browser.Dir]. When that directory
// exists already nothing is downloaded.
func (lc *Browser) Download() error {
	a, err := lc.resolve(runtime.GOOS+"/"+runtime.GOARCH, hasMuslLoader())
	if err != nil {
		return err
	}

	dir := lc.Dir()
	if _, err := os.Stat(dir); err == nil {
		return nil
	}

	if a.sha256 == "" {
		lc.Logger.Println("the download of", lc.name(), "is not verified: no SHA-256 is recorded for it")
	}

	hosts := lc.hosts()
	urls := make([]string, 0, len(hosts))
	for _, host := range hosts {
		urls = append(urls, a.url(host))
	}

	err = fetch.Zip(lc.Context, dir, urls,
		fetch.WithSHA256(a.sha256),
		fetch.WithStripFirstDir(),
		fetch.WithClient(lc.HTTPClient),
		fetch.WithLogger(lc.Logger),
	)
	if err != nil {
		return fmt.Errorf("can't download %s: %w", lc.name(), err)
	}

	return nil
}

// Get is a smart helper to get the browser executable path.
// If [Browser.BinPath] is not valid it will auto download the browser to [Browser.BinPath].
// Concurrent downloads of the same browser, in this process or another,
// serialize behind a file lock next to [Browser.Dir], and the ones that
// waited find it in place.
func (lc *Browser) Get() (string, error) {
	// Whether the browser was in place before validation: a directory that
	// was there and fails to validate is broken and goes before the download
	// replaces it. One that lands during the validation, downloaded by
	// another process, is complete and must stay, so the check comes first.
	_, err := os.Stat(lc.Dir())
	present := err == nil

	if lc.Validate() == nil {
		return lc.BinPath(), nil
	}

	if present {
		_ = os.RemoveAll(lc.Dir())
	}

	return lc.BinPath(), lc.Download()
}

// MustGet is similar with Get.
func (lc *Browser) MustGet() string {
	p, err := lc.Get()
	utils.E(err)
	return p
}

// Validate returns nil if the browser executable is valid.
// If the executable is malformed it will return error.
func (lc *Browser) Validate() error {
	_, err := os.Stat(lc.BinPath())
	if err != nil {
		return err
	}

	cmd := exec.Command(lc.BinPath(), "--headless", "--no-sandbox",
		"--use-mock-keychain", "--disable-dev-shm-usage",
		"--disable-gpu", "--dump-dom", "about:blank")
	b, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(b), "error while loading shared libraries") {
			// When the os is missing some dependencies for chromium we treat it as valid binary.
			return nil
		}

		return fmt.Errorf("failed to run the browser: %w\n%s", err, b)
	}
	if !bytes.Contains(b, []byte(`<html><head></head><body></body></html>`)) {
		return errors.New("the browser executable doesn't support headless mode")
	}

	return nil
}

// LookPath searches for the browser executable from often used paths on current operating system.
func LookPath() (found string, has bool) {
	list := map[string][]string{
		"darwin": {
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"/Applications/Google Chrome Canary.app/Contents/MacOS/Google Chrome Canary",
			"/usr/bin/google-chrome-stable",
			"/usr/bin/google-chrome",
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
		},
		"linux": {
			"chrome",
			"google-chrome",
			"/usr/bin/google-chrome",
			"microsoft-edge",
			"/usr/bin/microsoft-edge",
			"chromium",
			"chromium-browser",
			"google-chrome-stable",
			"/usr/bin/google-chrome-stable",
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
			"/snap/bin/chromium",
			"/data/data/com.termux/files/usr/bin/chromium-browser",
		},
		"openbsd": {
			"chrome",
			"chromium",
		},
		"windows": append([]string{"chrome", "edge"}, expandWindowsExePaths(
			`Google\Chrome\Application\chrome.exe`,
			`Chromium\Application\chrome.exe`,
			`Microsoft\Edge\Application\msedge.exe`,
		)...),
	}[runtime.GOOS]

	for _, path := range list {
		var err error
		found, err = exec.LookPath(path)
		has = err == nil
		if has {
			break
		}
	}

	return
}

// interface for testing.
var openExec = exec.Command

// Open tries to open the url via system's default browser.
func Open(url string) {
	// Windows doesn't support format [::]
	url = strings.Replace(url, "[::]", "[::1]", 1)

	if bin, has := LookPath(); has {
		p := openExec(bin, url)
		if p.Start() == nil {
			_ = p.Process.Release()
		}
	}
}

func expandWindowsExePaths(list ...string) []string {
	newList := []string{}
	for _, p := range list {
		newList = append(
			newList,
			filepath.Join(os.Getenv("ProgramFiles"), p),
			filepath.Join(os.Getenv("ProgramFiles(x86)"), p),
			filepath.Join(os.Getenv("LocalAppData"), p),
		)
	}

	return newList
}
