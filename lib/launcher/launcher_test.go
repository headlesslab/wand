package launcher_test

import (
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

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
		"--no-startup-window",
		"--proxy-server=test.com",
		fmt.Sprintf("--remote-debugging-port=%d", port),
		"--test-append=a",
		"--user-data-dir=" + dir,
		"about:blank",
	})

	url := l.MustLaunch()
	g.PathExists(dir)

	g.Eq(url, launcher.NewUserMode().RemoteDebuggingPort(port).MustLaunch())
}

// TestUserModeBrandedChrome is the Confirmed fix for rod #1189 and #1184:
// User mode launches a visible browser on wand's own profile directory,
// against the branded Google Chrome that discovery finds, connects to it and
// closes it. Since Chrome 136 branded Chrome refuses remote debugging on its
// default profile, the one the Snapshot's NewUserMode launched on, so the
// Snapshot fails this test with Chrome's refusal. Chrome for Testing,
// Chromium and Edge are exempt from the rule and would not reproduce it, so
// the test skips, naming what it found, where discovery finds no branded
// Chrome (ubuntu-24.04-arm ships none). A visible browser needs a display:
// on Linux without one the launch goes through xvfb-run where it is
// installed (ubuntu-latest), and skips otherwise.
func TestUserModeBrandedChrome(t *testing.T) {
	g := setup(t)

	bin, has := launcher.LookPath()
	if !has || !brandedChrome(bin) {
		g.Skip(fmt.Sprintf("no branded Google Chrome by discovery (found %q)", bin))
	}

	l := launcher.NewUserMode().Bin(bin).RemoteDebuggingPort(freePort(g)).Context(g.Context())
	if runtime.GOOS == "linux" && os.Getenv("DISPLAY") == "" {
		if _, err := exec.LookPath("xvfb-run"); err != nil {
			g.Skip("a visible browser needs a display, and there is neither DISPLAY nor xvfb-run")
		}
		l.XVFB("-a")
	}
	// Kill, never Cleanup: the profile is the persistent one of User mode,
	// on a developer's machine the user's own.
	defer l.Kill()

	g.False(l.Has(flags.Headless))
	g.False(l.Has(flags.Leakless))

	u, err := l.Launch()
	g.E(err)
	g.PathExists(l.Get(flags.UserDataDir))

	c := cdp.MustStartWithURL(g.Context(), u, nil)
	res, err := c.Call(g.Context(), "", "Browser.getVersion", nil)
	g.E(err)
	// A visible browser reports itself as Chrome, a headless one as
	// HeadlessChrome.
	g.Has(string(res), `"product":"Chrome/`)

	_, _ = c.Call(g.Context(), "", "Browser.close", nil)
	// The connection ends when the browser has gone.
	for range c.Event() {
		continue
	}
}

// brandedChrome is whether bin, as discovery found it, is branded Google
// Chrome by its install path: the one browser that refuses remote debugging
// on its default profile since Chrome 136, which Chrome for Testing, Chromium
// and Microsoft Edge do not. The path decides, since chrome.exe prints
// nothing for --version on Windows.
func brandedChrome(bin string) bool {
	p := strings.ToLower(filepath.ToSlash(bin))

	switch {
	case strings.HasSuffix(p, "/google/chrome/application/chrome.exe"): // Windows
		return true
	case strings.Contains(p, "/google chrome.app/"): // macOS
		return true
	case strings.HasPrefix(path.Base(p), "google-chrome"), strings.Contains(p, "/opt/google/chrome/"): // Linux
		return true
	}

	return false
}

// TestUserDataDirErr: a user data directory that cannot be made fails the
// launch before any browser starts.
func TestUserDataDirErr(t *testing.T) {
	g := setup(t)

	file := filepath.Join(t.TempDir(), "file")
	g.E(os.WriteFile(file, nil, 0o644))

	l := launcher.New().Preferences("").UserDataDir(filepath.Join(file, "user-data"))
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
	g.E(err)
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
	// browser's own default profile has, there is none to make.
	_, err := launcher.NewUserMode().RemoteDebuggingPort(freePort(g)).UserDataDir("").Bin("not-exists").Launch()
	g.Err(err)

	_, err = launcher.NewUserMode().RemoteDebuggingPort(freePort(g)).Bin("echo").Launch()
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
