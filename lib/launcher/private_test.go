package launcher

import (
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/headlesslab/wand/lib/cdp"
	"github.com/headlesslab/wand/lib/defaults"
	"github.com/headlesslab/wand/lib/launcher/flags"
	"github.com/headlesslab/wand/lib/launcher/pins"
	"github.com/headlesslab/wand/lib/utils"
	"github.com/ysmood/got"
)

var setup = got.Setup(nil)

func TestToHTTP(t *testing.T) {
	g := setup(t)

	u, _ := url.Parse("wss://a.com")
	g.Eq("https", toHTTP(*u).Scheme)

	u, _ = url.Parse("ws://a.com")
	g.Eq("http", toHTTP(*u).Scheme)
}

func TestToWS(t *testing.T) {
	g := setup(t)

	u, _ := url.Parse("https://a.com")
	g.Eq("wss", toWS(*u).Scheme)

	u, _ = url.Parse("http://a.com")
	g.Eq("ws", toWS(*u).Scheme)
}

func TestLaunchOptions(t *testing.T) {
	g := setup(t)

	defaults.Show = true
	defaults.Devtools = true
	inContainer = true

	// restore
	defer func() {
		defaults.ResetWith("")
		inContainer = utils.InContainer
	}()

	l := New()

	g.False(l.Has(flags.Headless))

	g.True(l.Has(flags.NoSandbox))

	g.True(l.Has("auto-open-devtools-for-tabs"))
}

func TestManagedOptions(t *testing.T) {
	g := setup(t)

	// A revision selects the Chromium source, a version the Chrome for Testing
	// source, so neither is silently ignored.
	l := New().Revision(7)
	g.Eq(l.browser.Source, SourceChromium)
	g.Eq(l.browser.Revision, 7)

	l.Version("1.2.3.4")
	g.Eq(l.browser.Source, SourceChrome)
	g.Eq(l.browser.Version, "1.2.3.4")

	l.Source(SourceChromium).Binary(BinaryHeadlessShell).Hosts("https://a.example/{archive}")
	g.Eq(l.browser.Source, SourceChromium)
	g.Eq(l.browser.Binary, BinaryHeadlessShell)
	g.Eq(l.browser.Hosts, []string{"https://a.example/{archive}"})
}

func TestResolve(t *testing.T) {
	g := setup(t)

	// Pins of the test's own, so that the outcome does not move with the Roll.
	chromeSHA256 = map[string]map[string]string{
		"chrome":                {"linux64": "aa", "win64": "bb"},
		"chrome-headless-shell": {"linux64": "cc"},
	}
	chromiumSHA256 = map[string]string{"Linux_x64": "dd"}
	defer func() {
		chromeSHA256 = pins.ChromeSHA256
		chromiumSHA256 = pins.ChromiumSHA256
	}()

	pinned := NewBrowser()
	pinned.Source = SourceChrome
	pinned.Binary = BinaryChrome

	{ // the Target Chrome where its archive is pinned
		a, err := pinned.resolve("linux/amd64", false)
		g.E(err)
		g.Eq(a, archive{platform: "linux64", version: pins.ChromeVersion, name: "chrome-linux64.zip", sha256: "aa"})
		g.Eq(a.url(DefaultHosts(SourceChrome)[0]),
			"https://storage.googleapis.com/chrome-for-testing-public/"+pins.ChromeVersion+"/linux64/chrome-linux64.zip")
	}

	{ // a platform Chrome for Testing builds for, but not at the Target Chrome
		_, err := pinned.resolve("darwin/arm64", false)
		g.Has(err.Error(), "no chrome "+pins.ChromeVersion+" archive is pinned for darwin/arm64")
		g.Has(err.Error(), "WAND_BROWSER_BIN")
	}

	{ // platforms Chrome for Testing never builds for
		for _, platform := range []string{"windows/arm64", "linux/loong64", "freebsd/amd64"} {
			_, err := pinned.resolve(platform, false)
			g.Desc(platform).Has(err.Error(), "no Chrome for Testing build exists for "+platform)
			g.Desc(platform).Has(err.Error(), "Launcher.Bin()")
		}
	}

	{ // the musl C library
		_, err := pinned.resolve("linux/amd64", true)
		g.Has(err.Error(), "musl")
		g.Has(err.Error(), "linux/amd64")
		g.Has(err.Error(), "WAND_BROWSER_BIN")
	}

	{ // chrome-headless-shell
		b := NewBrowser()
		b.Source = SourceChrome
		b.Binary = BinaryHeadlessShell

		a, err := b.resolve("linux/amd64", false)
		g.E(err)
		g.Eq(a, archive{platform: "linux64", version: pins.ChromeVersion, name: "chrome-headless-shell-linux64.zip", sha256: "cc"})

		_, err = b.resolve("windows/amd64", false)
		g.Has(err.Error(), "no chrome-headless-shell "+pins.ChromeVersion+" archive is pinned for windows/amd64")
	}

	{ // a version of the user's own: no hash, on any Chrome for Testing platform
		b := NewBrowser()
		b.Source = SourceChrome
		b.Binary = BinaryChrome
		b.Version = "1.2.3.4"

		a, err := b.resolve("darwin/arm64", false)
		g.E(err)
		g.Eq(a, archive{platform: "mac-arm64", version: "1.2.3.4", name: "chrome-mac-arm64.zip"})
	}

	{ // the Companion Chromium
		b := NewBrowser()
		b.Source = SourceChromium
		b.Binary = BinaryChrome

		a, err := b.resolve("linux/amd64", false)
		g.E(err)
		position := strconv.Itoa(pins.ChromiumPosition)
		g.Eq(a, archive{platform: "Linux_x64", version: position, name: "chrome-linux.zip", sha256: "dd"})
		g.Eq(a.url(DefaultHosts(SourceChromium)[1]),
			"https://registry.npmmirror.com/-/binary/chromium-browser-snapshots/Linux_x64/"+position+"/chrome-linux.zip")

		_, err = b.resolve("windows/amd64", false)
		g.Has(err.Error(), "no Chromium "+position+" archive is pinned for windows/amd64")

		for _, platform := range []string{"linux/arm64", "windows/arm64", "linux/loong64"} {
			_, err = b.resolve(platform, false)
			g.Desc(platform).Has(err.Error(), "no Chromium trunk build exists for "+platform)
			g.Desc(platform).Has(err.Error(), "Launcher.Bin()")
		}

		_, err = b.resolve("linux/amd64", true)
		g.Has(err.Error(), "musl")

		b.Revision = 1
		a, err = b.resolve("darwin/amd64", false)
		g.E(err)
		g.Eq(a, archive{platform: "Mac", version: "1", name: "chrome-mac.zip"})
	}
}

// TestPinnedPlatforms ties the platform tables to the pins: every platform
// the Target Chrome has an archive for resolves to that archive's hash, every
// other Chrome for Testing platform is refused, and the Companion Chromium is
// pinned under every prefix the launcher can download from.
func TestPinnedPlatforms(t *testing.T) {
	g := setup(t)

	for _, binary := range []Binary{BinaryChrome, BinaryHeadlessShell} {
		b := NewBrowser()
		b.Source = SourceChrome
		b.Binary = binary

		for platform, cft := range chromePlatforms {
			a, err := b.resolve(platform, false)
			sum, pinned := pins.ChromeSHA256[string(binary)][cft]
			if pinned {
				g.Desc("%s %s", binary, platform).E(err)
				g.Desc("%s %s", binary, platform).Eq(a.sha256, sum)
			} else {
				g.Desc("%s %s", binary, platform).Has(err.Error(), "is pinned for "+platform)
			}
		}
	}

	b := NewBrowser()
	b.Source = SourceChromium
	b.Binary = BinaryChrome

	for platform, bucket := range chromiumPlatforms {
		a, err := b.resolve(platform, false)
		g.Desc(platform).E(err)
		g.Desc(platform).Eq(a.sha256, pins.ChromiumSHA256[bucket.prefix])
	}
}

func TestHosts(t *testing.T) {
	g := setup(t)

	b := NewBrowser()
	b.Hosts = nil
	g.Eq(b.hosts(), DefaultHosts(b.Source))

	b.Source = SourceChromium
	g.Eq(b.hosts(), DefaultHosts(SourceChromium))

	b.Hosts = []string{"https://a.example/{archive}"}
	g.Eq(b.hosts(), b.Hosts)
}

func TestBinPaths(t *testing.T) {
	g := setup(t)

	b := NewBrowser()
	b.RootDir = "cache"
	b.Version = "1.0"
	b.Revision = 2

	cases := []struct {
		source Source
		binary Binary
		goos   string
		path   string
	}{
		{SourceChrome, BinaryChrome, "linux", "chrome-1.0/chrome"},
		{SourceChrome, BinaryChrome, "darwin", "chrome-1.0/Google Chrome for Testing.app/Contents/MacOS/Google Chrome for Testing"},
		{SourceChrome, BinaryChrome, "windows", "chrome-1.0/chrome.exe"},
		{SourceChrome, BinaryHeadlessShell, "linux", "chrome-headless-shell-1.0/chrome-headless-shell"},
		{SourceChrome, BinaryHeadlessShell, "darwin", "chrome-headless-shell-1.0/chrome-headless-shell"},
		{SourceChrome, BinaryHeadlessShell, "windows", "chrome-headless-shell-1.0/chrome-headless-shell.exe"},
		{SourceChromium, BinaryChrome, "linux", "chromium-2/chrome"},
		{SourceChromium, BinaryChrome, "darwin", "chromium-2/Chromium.app/Contents/MacOS/Chromium"},
		{SourceChromium, BinaryChrome, "windows", "chromium-2/chrome.exe"},
	}

	for _, c := range cases {
		b.Source, b.Binary = c.source, c.binary
		g.Desc("%s %s %s", c.source, c.binary, c.goos).Eq(b.binPath(c.goos), filepath.Join("cache", filepath.FromSlash(c.path)))
	}

	g.Eq(b.BinPath(), b.binPath(runtime.GOOS))
}

func TestCacheDir(t *testing.T) {
	g := setup(t)

	t.Setenv(EnvBrowserCache, "")
	g.True(strings.HasSuffix(cacheDir(), filepath.Join("wand", "browser")))
	g.Neq(filepath.Dir(filepath.Dir(cacheDir())), os.TempDir())

	t.Setenv(EnvBrowserCache, "cache")
	g.Eq(cacheDir(), "cache")

	// A user with no cache directory gets the temporary directory.
	t.Setenv(EnvBrowserCache, "")
	for _, name := range []string{"XDG_CACHE_HOME", "HOME", "LocalAppData"} {
		t.Setenv(name, "")
	}
	g.Eq(cacheDir(), filepath.Join(os.TempDir(), "wand", "browser"))
}

func TestMuslLoader(t *testing.T) {
	g := setup(t)

	dir := t.TempDir()
	glob := filepath.Join(dir, "ld-musl-*.so.1")
	g.False(muslLoaderPresent(glob))

	g.E(os.WriteFile(filepath.Join(dir, "ld-musl-x86_64.so.1"), nil, 0o644))
	g.True(muslLoaderPresent(glob))

	// Only a Linux distribution ships musl's loader.
	g.Eq(hasMuslLoader(), runtime.GOOS == "linux" && muslLoaderPresent(muslLoaderGlob))
}

func TestGetURLErr(t *testing.T) {
	g := setup(t)

	l := New()

	l.ctxCancel()
	_, err := l.getURL()
	g.Err(err)

	l = New()
	l.parser.lock.Lock()
	l.parser.Buffer = "err"
	l.parser.lock.Unlock()
	close(l.exit)
	_, err = l.getURL()
	g.Eq("[launcher] Failed to get the debug url: err", err.Error())
}

func TestManaged(t *testing.T) {
	g := setup(t)

	// The budget covers a browser launch through the manager (behind the
	// download lock, which the other package suites contend for on a 4-vCPU
	// runner under -race), a crash, the cleanup and a second handshake.
	ctx := g.Timeout(15 * time.Second)

	s := got.New(g).Serve()
	rl := NewManager()
	s.Mux.Handle("/", rl)

	l := MustNewManaged(s.URL()).KeepUserDataDir().Delete(flags.KeepUserDataDir)
	c := l.MustClient()
	g.E(c.Call(ctx, "", "Browser.getVersion", nil))
	utils.Sleep(1)
	_, _ = c.Call(ctx, "", "Browser.crash", nil)
	dir := l.Get(flags.UserDataDir)

	for ctx.Err() == nil {
		utils.Sleep(0.1)
		_, err := os.Stat(dir)
		if err != nil {
			break
		}
	}
	g.Err(os.Stat(dir))

	u, h := MustNewManaged(s.URL()).Bin("go").ClientHeader()
	_, err := cdp.StartWithURL(ctx, u, h)
	g.Eq(err.(*cdp.BadHandshakeError).Body, "[wand-manager] not allowed wand-bin path: go (use --allow-all to disable the protection)")
}

func TestLaunchErrs(t *testing.T) {
	g := setup(t)

	l := New().Bin("echo")
	_, err := l.Launch()
	g.Err(err)

	s := g.Serve()
	s.Route("/", "", nil)
	l = New().Bin("")
	l.browser.Logger = utils.LoggerQuiet
	l.browser.RootDir = filepath.Join(t.TempDir(), "browser")
	l.browser.Hosts = []string{s.URL("/{version}/{platform}/{archive}")}
	_, err = l.Launch()
	g.Err(err)
}

func TestURLParserErr(t *testing.T) {
	g := setup(t)

	u := &URLParser{
		Buffer: "error",
		lock:   &sync.Mutex{},
	}

	g.Eq(u.Err().Error(), "[launcher] Failed to get the debug url: error")

	u.Buffer = "/tmp/rod/chromium-818858/chrome: error while loading shared libraries: libgobject-2.0.so.0: cannot open shared object file: No such file or directory"
	g.Eq(u.Err().Error(), "[launcher] Failed to launch the browser, the doc might help https://go-rod.github.io/#/compatibility?id=os: /tmp/rod/chromium-818858/chrome: error while loading shared libraries: libgobject-2.0.so.0: cannot open shared object file: No such file or directory")
}

func TestTestOpen(t *testing.T) {
	// Open releases the process it starts, so it needs one that exists and
	// exits at once: this test binary running no test. A zero-value os.Process
	// is not an option since Go 1.23 refuses to release one.
	openExec = func(_ string, _ ...string) *exec.Cmd {
		return exec.Command(os.Args[0], "-test.run=^$")
	}
	defer func() { openExec = exec.Command }()

	Open("about:blank")

	// A browser that cannot start leaves no process to release. Like the call
	// above, this one reaches openExec only where LookPath finds a browser, as
	// it does on every Tier 1 runner.
	openExec = func(_ string, _ ...string) *exec.Cmd {
		return exec.Command(filepath.Join(t.TempDir(), "no-such-browser"))
	}

	Open("about:blank")
}

func TestLaunchClient(t *testing.T) {
	g := setup(t)

	ctx := g.Timeout(5 * time.Second)

	s := got.New(g).Serve()
	rl := NewManager()
	s.Mux.Handle("/", rl)

	l := MustNewManaged(s.URL()).KeepUserDataDir().Delete(flags.KeepUserDataDir)
	c, err := l.Client()
	if err != nil {
		g.Err(err)
	}
	g.E(c.Call(ctx, "", "Browser.getVersion", nil))
}
