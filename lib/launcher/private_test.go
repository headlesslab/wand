package launcher

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/headlesslab/wand/lib/cdp"
	"github.com/headlesslab/wand/lib/defaults"
	"github.com/headlesslab/wand/lib/launcher/flags"
	"github.com/headlesslab/wand/lib/utils"
	"github.com/ysmood/got"
)

func HostTest(host string) Host {
	return func(revision int) string {
		return fmt.Sprintf(
			"%s/chromium-browser-snapshots/%s/%d/%s",
			host,
			hostConf.urlPrefix,
			revision,
			hostConf.zipName,
		)
	}
}

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
	// launcher's port lock, which the other package suites contend for on a
	// 4-vCPU runner under -race), a crash, the cleanup and a second handshake.
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
	l.browser.RootDir = filepath.Join("tmp", "browser-from-mirror", g.RandStr(16))
	l.browser.Hosts = []Host{HostTest(s.URL())}
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
