package launcher_test

import (
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/headlesslab/wand/lib/cdp"
	"github.com/headlesslab/wand/lib/defaults"
	"github.com/headlesslab/wand/lib/launcher"
	"github.com/headlesslab/wand/lib/launcher/flags"
	"github.com/headlesslab/wand/lib/launcher/pins"
	"github.com/headlesslab/wand/lib/utils"
	"github.com/ysmood/got"
)

var setup = got.Setup(nil)

// stop kills the browser l launched and removes its user data directory.
func stop(l *launcher.Launcher) {
	l.Kill()
	l.Cleanup()
}

// freePort is a TCP port nothing listens on right now, for the User mode
// tests: User mode reuses a browser already on its port, so a fixed port
// would meet a browser of another test binary on a shared machine.
func freePort(g got.G) int {
	g.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	g.E(err)
	defer func() { g.E(listener.Close()) }()
	return listener.Addr().(*net.TCPAddr).Port
}

func TestLaunch(t *testing.T) {
	g := setup(t)

	defaults.Proxy = "test.com"
	defer func() { defaults.ResetWith("") }()

	l := launcher.New().Preferences("").AlwaysOpenPDFExternally()
	defer stop(l)

	u := l.MustLaunch()
	g.Regex(`\Aws://.+\z`, u)

	parsed, _ := url.Parse(u)

	{ // test GetWebSocketDebuggerURL
		for _, prefix := range []string{"", ":", "127.0.0.1:", "ws://127.0.0.1:"} {
			u2 := launcher.MustResolveURL(prefix + parsed.Port())
			g.Regex(u, u2)
		}

		_, err := launcher.ResolveURL("")
		g.Err(err)
	}

	{
		_, err := launcher.NewManaged("")
		g.Err(err)

		_, err = launcher.NewManaged("1://")
		g.Err(err)

		_, err = launcher.NewManaged("ws://not-exists")
		g.Err(err)
	}

	{
		g.Panic(func() { launcher.New().Set("a=b") })
	}
}

func TestWindowSize(t *testing.T) {
	g := setup(t)

	g.Eq(launcher.New().WindowSize(800, 600).Get(flags.WindowSize), "800,600")
}

func TestWindowPosition(t *testing.T) {
	g := setup(t)

	g.Eq(launcher.New().WindowPosition(10, 20).Get(flags.WindowPosition), "10,20")
}

func TestLaunchUserMode(t *testing.T) {
	g := setup(t)

	l := launcher.NewUserMode()
	defer stop(l)

	l.Kill() // empty kill should do nothing

	has := l.Has("not-exists")
	g.False(has)

	l.Append("test-append", "a")
	f := l.Get("test-append")
	g.Eq("a", f)

	// A profile of this test's own, missing until the launch makes it, so
	// that the persistent one of User mode is not written by the browser
	// under test here, which need not be the one a user keeps it for.
	dir := filepath.Join(t.TempDir(), "user-mode")
	port := freePort(g)

	l = l.Context(g.Context()).Delete("test").Bin("").
		Version(pins.ChromeVersion).
		Logger(io.Discard).
		Leakless(false).Leakless(true).
		HeadlessNew(true).HeadlessNew(false).
		Headless(false).Headless(true).RemoteDebuggingPort(port).
		NoSandbox(true).NoSandbox(false).
		Devtools(true).Devtools(false).
		StartURL("about:blank").
		Proxy("test.com").
		UserDataDir("test").UserDataDir(dir).
		WorkingDir("").
		Env(append(os.Environ(), "TZ=Asia/Tokyo")...)

	g.Eq(l.FormatArgs(), []string{
		"--headless",
		"--no-first-run",
		"--no-startup-window",
		"--proxy-server=test.com",
		fmt.Sprintf("--remote-debugging-port=%d", port),
		"--test-append=a",
		"--user-data-dir=" + dir,
		"about:blank",
	})

	// The flag list above exercised NoSandbox in both directions and left it
	// off, which no browser started by root accepts; a container is root, so
	// the launch takes the flag back, after the list was asserted.
	if utils.InContainer {
		l.NoSandbox(true)
	}

	url := l.MustLaunch()
	g.True(g.PathExists(dir))

	g.Eq(url, launcher.NewUserMode().RemoteDebuggingPort(port).MustLaunch())
}

// TestUserModeBrandedChrome is the Confirmed fix for rod #1189 and #1184:
// User mode launches a visible browser on wand's own profile directory,
// against the branded Google Chrome that LookPath finds on this machine,
// connects to it and closes it. Branded Chrome of 136 or later refuses
// remote debugging on its default profile, the one the Snapshot's
// NewUserMode launched on, so the Snapshot fails this test with Chrome's
// refusal. Chrome for Testing, Chromium and Edge are exempt from the rule
// and would not reproduce it, so the test skips, naming what it found, where
// LookPath finds no branded Chrome (ubuntu-24.04-arm ships none); for the
// same reason it launches the browser LookPath found, not the one
// WAND_BROWSER_BIN pins, and it must, since the profile it launches on is
// the persistent one. A visible browser needs a display: on Linux without
// one the launch goes through xvfb-run where it is installed
// (ubuntu-latest), and skips otherwise.
func TestUserModeBrandedChrome(t *testing.T) {
	g := setup(t)

	bin, has := launcher.LookPath()
	if !has || !brandedChrome(bin) {
		g.Skip(fmt.Sprintf("no branded Google Chrome among the System browsers (found %q)", bin))
	}

	l := launcher.NewUserMode().Bin(bin).RemoteDebuggingPort(freePort(g)).Context(g.Context())
	if runtime.GOOS == "linux" && os.Getenv("DISPLAY") == "" {
		if _, err := exec.LookPath("xvfb-run"); err != nil {
			g.Skip("a visible browser needs a display, and there is neither DISPLAY nor xvfb-run")
		}
		l.XVFB("-a")
	}
	// Kill, never Cleanup or stop: the profile is the persistent one of User
	// mode, on a developer's machine the user's own, and it stays.
	defer l.Kill()

	g.False(l.Has(flags.Headless))

	u, err := l.Launch()
	g.E(err)
	g.True(g.PathExists(l.Get(flags.UserDataDir)))

	c := cdp.MustStartWithURL(g.Context(), u, nil)
	res, err := c.Call(g.Context(), "", "Browser.getVersion", nil)
	g.E(err)
	// A visible browser reports itself as Chrome, a headless one as
	// HeadlessChrome.
	g.Has(string(res), `"product":"Chrome/`)

	_, _ = c.Call(g.Context(), "", "Browser.close", nil)
	// The connection ends when the browser has gone; one that lingers fails
	// the test rather than holding the run to its timeout.
	gone := make(chan struct{})
	go func() {
		for range c.Event() {
			continue
		}
		close(gone)
	}()
	select {
	case <-gone:
	case <-time.After(30 * time.Second):
		g.Fatal("the browser is still connected 30 s after Browser.close")
	}
}

// brandedChrome is whether bin, a System browser LookPath found, is branded
// Google Chrome by its path, of any channel: the one browser that refuses
// remote debugging on its default profile since Chrome 136, which Chrome for
// Testing, Chromium and Microsoft Edge do not. The path decides, since
// chrome.exe prints nothing for --version on Windows: Google's install
// directories on Windows and Linux, the app bundle on macOS, the google-chrome
// names on PATH, a link to any of them included; Chrome for Testing's bundle
// is the one Google name to leave out.
func brandedChrome(bin string) bool {
	p := strings.ToLower(filepath.ToSlash(bin))
	if resolved, err := filepath.EvalSymlinks(bin); err == nil {
		p += " " + strings.ToLower(filepath.ToSlash(resolved))
	}
	google := strings.Contains(p, "google/chrome") || strings.Contains(p, "google chrome") || strings.Contains(p, "google-chrome")

	return google && !strings.Contains(p, "for testing")
}

// TestUserDataDirErr: a user data directory that cannot be made fails the
// launch before any browser starts.
func TestUserDataDirErr(t *testing.T) {
	g := setup(t)

	file := filepath.Join(t.TempDir(), "file")
	g.E(os.WriteFile(file, nil, 0o644))

	l := launcher.New().UserDataDir(filepath.Join(file, "user-data"))
	_, err := l.Launch()
	g.Err(err)
	g.Eq(l.PID(), 0)
}

func TestGuardFlags(t *testing.T) {
	g := setup(t)

	// The Orphan guard is on in New and off in User mode; the switch keeps
	// its name.
	g.True(launcher.New().Has(flags.Leakless))
	g.False(launcher.NewUserMode().Has(flags.Leakless))
	g.False(launcher.New().Leakless(false).Has(flags.Leakless))
	g.True(launcher.NewUserMode().Leakless(true).Has(flags.Leakless))

	// The Pipe tether's flag is passed at launch, with its descriptors, and
	// never through FormatArgs, whose output a caller may hand to exec.Command.
	l := launcher.New()
	g.False(l.Has("remote-debugging-pipe"))
	for _, arg := range l.FormatArgs() {
		g.Neq(arg, "--remote-debugging-pipe")
	}
}

// TestUserModeDir: User mode's profile is wand/user-mode under the user's
// configuration directory, a persistent directory wand owns rather than
// Chrome's default profile, which branded Chrome refuses remote debugging on
// since Chrome 136 (ADR-0010); UserDataDir overrides it, and the Orphan
// guard stays off.
func TestUserModeDir(t *testing.T) {
	g := setup(t)

	l := launcher.NewUserMode()
	dir := l.Get(flags.UserDataDir)
	g.Eq(dir, launcher.DefaultUserModeDir)
	config, err := os.UserConfigDir()
	if err != nil {
		// A user without a configuration directory, as in a container with
		// no HOME.
		config = os.TempDir()
	}
	g.Eq(dir, filepath.Join(config, "wand", "user-mode"))
	g.Has(l.FormatArgs(), "--user-data-dir="+dir)
	g.False(l.Has(flags.Leakless))
	g.False(l.Has(flags.Headless))

	// The override, by name and back to the default.
	named := t.TempDir()
	g.Eq(l.UserDataDir(named).Get(flags.UserDataDir), named)
	g.Has(l.FormatArgs(), "--user-data-dir="+named)
	g.False(l.UserDataDir("").Has(flags.UserDataDir))
}

func TestUserModeErr(t *testing.T) {
	g := setup(t)

	// With no user data directory at all, which a caller after the
	// browser's own default profile has, there is none to make; and none of
	// these failed launches makes the persistent profile of User mode.
	_, err := launcher.NewUserMode().RemoteDebuggingPort(freePort(g)).UserDataDir("").Bin("not-exists").Launch()
	g.Err(err)

	_, err = launcher.NewUserMode().RemoteDebuggingPort(freePort(g)).UserDataDir("").Bin("echo").Launch()
	g.Err(err)
}

func TestAppMode(t *testing.T) {
	g := setup(t)

	l := launcher.NewAppMode("http://example.com")

	g.Eq(l.Get(flags.App), "http://example.com")
}

func TestGetWebSocketDebuggerURLErr(t *testing.T) {
	g := setup(t)

	_, err := launcher.ResolveURL("1://")
	g.Err(err)
}

// TestResolveURL is the regression test of rod #1176: an answer that is not a
// browser's is an error, not a URL with "<nil>" for a path, whatever the
// status: a proxy's 502 in the way of the port, a web server that happens to
// listen on it and answers 200 with HTML, a null for the field, a URL that
// does not parse, or a body cut short. A browser's answer gives the
// WebSocket URL on the host that was asked. Each answer comes from an
// ephemeral server of its own; the error is printed rather than called, so
// that a nil one reads as nil on the Snapshot.
func TestResolveURL(t *testing.T) {
	g := setup(t)

	serve := func(status int, body string, cut bool) *got.Router {
		s := g.Serve()
		s.Mux.HandleFunc("/json/version", func(w http.ResponseWriter, _ *http.Request) {
			if cut {
				w.Header().Set("Content-Length", "100")
			}
			w.WriteHeader(status)
			_, _ = w.Write([]byte(body))
		})
		return s
	}

	notBrowsers := []struct {
		name   string
		status int
		body   string
		cut    bool
		err    string
	}{
		{"a proxy in the way", http.StatusBadGateway, "<html><body>502 Bad Gateway</body></html>", false, "502 Bad Gateway"},
		{"a web server on the port", http.StatusOK, "<html><body>hello</body></html>", false, "webSocketDebuggerUrl"},
		{"a null field", http.StatusOK, `{"webSocketDebuggerUrl": null}`, false, "webSocketDebuggerUrl"},
		{"a URL that does not parse", http.StatusOK, `{"webSocketDebuggerUrl": "ws://[::1"}`, false, "missing ']'"},
		{"a body cut short", http.StatusOK, `{"webSocketDebuggerUrl":`, true, "unexpected EOF"},
	}
	for _, c := range notBrowsers {
		_, err := launcher.ResolveURL(serve(c.status, c.body, c.cut).URL())
		g.Desc(c.name).Err(err)
		g.Desc(c.name).Has(fmt.Sprint(err), c.err)
	}

	browser := serve(http.StatusOK, `{"webSocketDebuggerUrl": "ws://localhost:1/devtools/browser/abc"}`, false)
	u, err := launcher.ResolveURL(browser.URL())
	g.E(err)
	g.Eq(u, "ws://"+browser.HostURL.Host+"/devtools/browser/abc")
}

func TestLaunchErr(t *testing.T) {
	g := setup(t)

	g.Panic(func() {
		launcher.New().Bin("not-exists").MustLaunch()
	})
	g.Panic(func() {
		launcher.New().Headless(false).Bin("not-exists").MustLaunch()
	})
	g.Panic(func() {
		launcher.New().ClientHeader()
	})
	{
		// Under xvfb-run, where it is installed, a browser starts.
		l := launcher.New().XVFB()
		_, _ = l.Launch()
		stop(l)
	}
}

var testProfileDir = flag.Bool("test-profile-dir", false, "set it to test profile dir")

func TestProfileDir(t *testing.T) {
	g := setup(t)

	l := launcher.New().Headless(false).
		ProfileDir("").ProfileDir("test-profile-dir")

	if !*testProfileDir {
		g.Skip("It's not CI friendly, so we skip it!")
	}

	l.MustLaunch()
	defer stop(l)

	userDataDir := l.Get(flags.UserDataDir)
	file, err := os.Stat(filepath.Join(userDataDir, "test-profile-dir"))

	g.E(err)
	g.True(file.IsDir())
}

func TestBrowserValid(t *testing.T) {
	g := setup(t)

	b := launcher.NewBrowser()
	b.RootDir = filepath.Join(t.TempDir(), "browser")
	b.Version = "0"
	g.Err(b.Validate())

	g.E(utils.Mkdir(filepath.Dir(b.BinPath())))

	g.E(exec.Command("go", "build", "-o", b.BinPath(), "./fixtures/chrome-exit-err").CombinedOutput())
	g.Has(b.Validate().Error(), "failed to run the browser")

	g.E(exec.Command("go", "build", "-o", b.BinPath(), "./fixtures/chrome-empty").CombinedOutput())
	g.Eq(b.Validate().Error(), "the browser executable doesn't support headless mode")

	g.E(exec.Command("go", "build", "-o", b.BinPath(), "./fixtures/chrome-lib-missing").CombinedOutput())
	g.Nil(b.Validate())

	g.E(exec.Command("go", "build", "-o", b.BinPath(), "./fixtures/chrome-headless").CombinedOutput())
	g.Nil(b.Validate())

	// A cached browser that validates is used as it is, without a download.
	p, err := b.Get()
	g.E(err)
	g.Eq(p, b.BinPath())
}

func TestIgnoreCerts(t *testing.T) {
	g := setup(t)

	// https://travistidwell.com/jsencrypt/demo/
	testData := []string{
		`-----BEGIN PUBLIC KEY-----
MIGeMA0GCSqGSIb3DQEBAQUAA4GMADCBiAKBgF9pr2zok5bivQIEUN7Y58a9uB1o
sroMt3hxNfzOh/G+sXgYPPoEl2/Ys/2zbvym7Ze0eGbb6FrV8aueg89TPTNWAKlN
N49q6S3zLG1WmI2rVYz4LtPgpg1YR9FQRIg4Ll0C02daufXgvUBGjIARH19FTw6P
61kEhnEQxUHhdAqbAgMBAAE=
-----END PUBLIC KEY-----
		`,
		`-----BEGIN PUBLIC KEY-----
MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQCvBTz/TOYc66qB97OyYenSHk4T
hAUKX5RUWZ/80o0zyJoo1dfrrwW9PlT5o4DlGMs0NSbtJ8RMQRTLZwL/zxXjiEMv
dKFs2OrefYKANTc0e2XAtQAm3Is5Ro8AF1S4Fk+eZXr2yZtBRKXvhJ/A2bilVoSn
fmQnyBe7dVU43NXfrQIDAQAB
-----END PUBLIC KEY-----
		`,
	}

	keys := make([]crypto.PublicKey, 0, len(testData))

	for _, pubPEM := range testData {
		block, _ := pem.Decode([]byte(pubPEM))
		if block == nil {
			g.Fatal("failed to parse PEM block containing the public key")
			return // no-op because g.Fatal calls t.FailNow() but `staticcheck` doesn't know it
		}

		pub, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			g.Fatalf("failed to parse DER encoded public key: " + err.Error())
		}

		keys = append(keys, pub)
	}

	l := launcher.New()

	err := l.IgnoreCerts(keys)
	if err != nil {
		g.Fatalf("IgnoreCerts: %s", err)
	}

	expected := "--ignore-certificate-errors-spki-list=" + strings.Join([]string{
		"+ZqfrXb+V/36nZecO59bghHlNhiHTzImjYLnNWGUd1I=",
		"llpTCSqZ2/IKsMg4tz+o1mCkXIOdKcM6sKu9kC6o7S4=",
	}, ",")

	g.Has(l.FormatArgs(), expected)
}

func TestIgnoreCerts_InvalidCert(t *testing.T) {
	g := setup(t)

	l := launcher.New()

	err := l.IgnoreCerts([]crypto.PublicKey{nil})
	if err == nil {
		g.Fatalf("IgnoreCerts: %s", err)
	}
}

func TestLaunchMultiTimes(t *testing.T) {
	g := setup(t)

	// first time launch, success.
	l := launcher.New()
	defer stop(l)
	u, e := l.Launch()
	g.Neq(u, "")
	g.E(e)

	// second time launch, failed with ErrAlreadyLaunched.
	_, e = l.Launch()
	g.Eq(e, launcher.ErrAlreadyLaunched)
}

// TestLaunchAttach is the regression test of rod #1221: a launcher with the
// guard off attaches to a browser already listening on its port, and its Kill
// and Cleanup then have nothing to do: nothing waited for, nothing killed,
// nothing removed, since the browser and its profile are not the launcher's
// own. The Snapshot's Cleanup waited forever on the exit of a process it
// never started. The browser comes from a guarded launcher of this test on an
// ephemeral port, so that it goes with the test binary whatever happens. The
// bound is half of what Cleanup gives a browser of the launcher's own before
// killing it (cleanupBound), so a wait of any kind fails the test.
func TestLaunchAttach(t *testing.T) {
	g := setup(t)

	port := freePort(g)
	l := launcher.New().RemoteDebuggingPort(port)
	defer stop(l)
	u := l.MustLaunch()
	dir := l.Get(flags.UserDataDir)

	attached := launcher.New().Leakless(false).RemoteDebuggingPort(port).UserDataDir(dir)
	u2, err := attached.Launch()
	g.E(err)
	g.Eq(u2, u)
	g.Eq(attached.PID(), 0)

	done := make(chan struct{})
	go func() {
		attached.Kill()
		attached.Cleanup()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Kill and Cleanup of an attached launcher did not return")
	}

	u3, err := launcher.ResolveURL(fmt.Sprint(port))
	g.E(err)
	g.Eq(u3, u)
	g.True(g.PathExists(dir))
}
