package wand_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/headlesslab/lazyjson"
	"github.com/headlesslab/wand"
	"github.com/headlesslab/wand/lib/cdp"
	"github.com/headlesslab/wand/lib/devices"
	"github.com/headlesslab/wand/lib/launcher"
	"github.com/headlesslab/wand/lib/launcher/pins"
	"github.com/headlesslab/wand/lib/proto"
	"github.com/headlesslab/wand/lib/utils"
)

func TestIncognito(t *testing.T) {
	g := setup(t)

	k := g.RandStr(16)

	b := g.browser.MustIncognito().Sleeper(wand.DefaultSleeper)
	defer b.MustClose()

	page := b.MustPage(g.blank())
	defer page.MustClose()
	page.MustEval(`k => localStorage[k] = 1`, k)

	g.True(g.page.MustNavigate(g.blank()).MustEval(`k => localStorage[k]`, k).Nil())
	g.Eq(page.MustEval(`k => localStorage[k]`, k).Str(), "1") // localStorage can only store string

	g.Panic(func() {
		g.mc.stubErr(1, proto.TargetCreateBrowserContext{})
		g.browser.MustIncognito()
	})
}

func TestBrowserResetControlURL(_ *testing.T) {
	wand.New().ControlURL("test").ControlURL("")
}

func TestDefaultDevice(t *testing.T) {
	g := setup(t)

	ua := ""

	s := g.Serve()
	s.Mux.HandleFunc("/t", func(_ http.ResponseWriter, r *http.Request) {
		ua = r.Header.Get("User-Agent")
	})

	// TODO: https://github.com/golang/go/issues/51459
	b := *g.browser
	b.DefaultDevice(devices.IPhoneX)

	b.MustPage(s.URL("/t")).MustClose()
	g.Eq(ua, devices.IPhoneX.UserAgentEmulation().UserAgent)

	b.NoDefaultDevice()
	b.MustPage(s.URL("/t")).MustClose()
	g.Neq(ua, devices.IPhoneX.UserAgentEmulation().UserAgent)
}

func TestPageErr(t *testing.T) {
	g := setup(t)

	g.Panic(func() {
		g.mc.stubErr(1, proto.TargetAttachToTarget{})
		g.browser.MustPage()
	})
}

func TestPageFromTarget(t *testing.T) {
	g := setup(t)

	g.Panic(func() {
		res, err := proto.TargetCreateTarget{URL: "about:blank"}.Call(g.browser)
		g.E(err)
		defer func() {
			g.browser.MustPageFromTargetID(res.TargetID).MustClose()
		}()

		g.mc.stubErr(1, proto.EmulationSetDeviceMetricsOverride{})
		g.browser.MustPageFromTargetID(res.TargetID)
	})
}

func TestBrowserPages(t *testing.T) {
	g := setup(t)

	b := g.browser
	pages := b.MustPages()
	g.Gte(len(pages), 1)

	{
		g.mc.stub(1, proto.TargetGetTargets{}, func(send StubSend) (lazyjson.JSON, error) {
			d, _ := send()
			return *d.Set("targetInfos.0.type", "iframe"), nil
		})
		b.MustPages()
	}

	g.Panic(func() {
		g.mc.stubErr(1, proto.TargetCreateTarget{})
		b.MustPage()
	})
	g.Panic(func() {
		g.mc.stubErr(1, proto.TargetGetTargets{})
		b.MustPages()
	})
	g.Panic(func() {
		_, err := proto.TargetCreateTarget{URL: "about:blank"}.Call(b)
		g.E(err)
		g.mc.stubErr(1, proto.TargetAttachToTarget{})
		b.MustPages()
	})
}

func TestBrowserClearStates(t *testing.T) {
	g := setup(t)

	g.E(proto.EmulationClearGeolocationOverride{}.Call(g.page))
}

func TestBrowserEvent(t *testing.T) {
	g := setup(t)

	ctx := g.Context()
	messages := g.browser.Context(ctx).Event()
	p := g.newPage()
	wait := make(chan struct{})
	for msg := range messages {
		e := proto.TargetAttachedToTarget{}
		if msg.Load(&e) {
			g.Eq(e.TargetInfo.TargetID, p.TargetID)
			close(wait)
			break
		}
	}
	<-wait

	// Nobody reads messages any more: raise an event, let the forwarding
	// goroutine block on delivering it, then end the context, so the context
	// is what stops a delivery in progress.
	p.MustNavigate(g.blank())
	utils.Sleep(0.3)
	ctx.Cancel()
}

func TestBrowserWaitEvent(t *testing.T) {
	g := setup(t)

	g.NotNil(g.browser.Context(g.Context()).Event())

	wait := g.page.WaitEvent(proto.PageFrameNavigated{})
	g.page.MustNavigate(g.blank())
	wait()

	wait = g.browser.EachEvent(func(_ *proto.PageFrameNavigated, _ proto.TargetSessionID) bool {
		return true
	})
	g.page.MustNavigate(g.blank())
	wait()
}

func TestBrowserCrash(t *testing.T) {
	g := setup(t)

	browser := wand.New().Context(g.Context()).MustConnect()
	// The close of a crashed browser fails, and still removes its directory.
	defer func() { _ = browser.Close() }()

	page := browser.MustPage()
	js := `() => new Promise(r => setTimeout(r, 10000))`

	// The pending call fails with the crash; it is asserted here, once it has
	// returned, never from its goroutine.
	pending := make(chan error, 1)
	go func() {
		pending <- wand.Try(func() { page.MustEval(js) })
	}()

	utils.Sleep(0.2)

	_ = proto.BrowserCrash{}.Call(browser)

	g.Err(<-pending)

	_, err := page.Eval(js)
	g.Has(err.Error(), "use of closed network connection")
}

func TestBrowserCall(t *testing.T) {
	g := setup(t)

	v, err := proto.BrowserGetVersion{}.Call(g.browser)
	g.E(err)

	g.Regex("1.3", v.ProtocolVersion)
}

func TestBlockingNavigation(t *testing.T) {
	g := setup(t)

	/*
		Navigate can take forever if a page doesn't response.
		If one page is blocked, other pages should still work.
	*/

	s := g.Serve()
	pause := g.Context()

	s.Mux.HandleFunc("/a", func(_ http.ResponseWriter, _ *http.Request) {
		<-pause.Done()
	})
	s.Route("/b", ".html", `<html>ok</html>`)

	blocked := g.newPage()

	go func() {
		g.Panic(func() {
			blocked.MustNavigate(s.URL("/a"))
		})
	}()

	utils.Sleep(0.3)

	g.newPage(s.URL("/b"))
}

func TestResolveBlocking(t *testing.T) {
	g := setup(t)

	s := g.Serve()

	pause := g.Context()

	s.Mux.HandleFunc("/", func(_ http.ResponseWriter, _ *http.Request) {
		<-pause.Done()
	})

	p := g.newPage()

	go func() {
		utils.Sleep(0.1)
		p.MustStopLoading()
	}()

	g.Panic(func() {
		p.MustNavigate(s.URL())
	})
}

func TestTestTry(t *testing.T) {
	g := setup(t)

	g.Nil(wand.Try(func() {}))

	err := wand.Try(func() { panic(1) })
	var errVal *wand.TryError
	g.True(errors.As(err, &errVal))
	g.Is(err, &wand.TryError{})
	g.Eq(errVal.Unwrap().Error(), "1")
	g.Eq(1, errVal.Value)
	g.Has(errVal.Error(), "error value: 1\ngoroutine")

	errVal = wand.Try(func() { panic(errors.New("t")) }).(*wand.TryError)
	g.Eq(errVal.Unwrap().Error(), "t")
}

func TestBrowserOthers(t *testing.T) {
	g := setup(t)

	g.browser.Timeout(time.Second).CancelTimeout().MustGetCookies()
	g.browser.MustIgnoreCertErrors(false)
}

func TestBinarySize(t *testing.T) {
	g := setup(t)

	if runtime.GOOS == "windows" || utils.InContainer {
		g.SkipNow()
	}

	cmd := exec.Command("go", "build",
		"-trimpath",
		"-ldflags", "-w -s",
		"-o", "tmp/translator",
		"./lib/examples/translator")

	cmd.Env = append(os.Environ(), "GOOS=linux")

	g.Nil(cmd.Run())

	stat, err := os.Stat("tmp/translator")
	g.E(err)

	// With leakless's 2.4 MB of embedded guard binaries gone, Go 1.23 builds
	// it at 8.0 MB and Go 1.27, the Gate's, at about 8.9 MB (11.3 MB with
	// them). The bound locks the removal in, which #53 no longer has to, and
	// still catches a test framework or another heavy dependency linked into
	// user binaries.
	g.Lte(float64(stat.Size())/1024/1024, 10) // mb
}

func TestBrowserCookies(t *testing.T) {
	g := setup(t)

	b := g.browser.MustIncognito()
	defer b.MustClose()

	b.MustSetCookies(&proto.NetworkCookie{
		Name:   "a",
		Value:  "val",
		Domain: "test.com",
	})

	cookies := b.MustGetCookies()

	g.Len(cookies, 1)
	g.Eq(cookies[0].Name, "a")
	g.Eq(cookies[0].Value, "val")

	{
		b.MustSetCookies()
		cookies := b.MustGetCookies()
		g.Len(cookies, 0)
	}

	g.mc.stubErr(1, proto.StorageGetCookies{})
	g.Err(b.GetCookies())
}

func TestWaitDownload(t *testing.T) {
	g := setup(t)

	s := g.Serve()
	content := "test content"

	s.Route("/d", ".bin", []byte(content))
	s.Route("/page", ".html", fmt.Sprintf(`<html><a href="%s/d" download>click</a></html>`, s.URL()))

	page := g.page.MustNavigate(s.URL("/page"))

	wait := g.browser.MustWaitDownload()
	page.MustElement("a").MustClick()
	data := wait()

	g.Eq(content, string(data))
}

func TestWaitDownloadDataURI(t *testing.T) {
	g := setup(t)

	s := g.Serve()

	s.Route("/", ".html",
		`<html>
			<a id="a" href="data:text/plain;,test%20data" download>click</a>
			<a id="b" download>click</a>
			<script>
				const b = document.getElementById('b')
				b.href = URL.createObjectURL(new Blob(['test blob'], {
					type: "text/plain; charset=utf-8"
				}))
			</script>
		</html>`,
	)

	page := g.page.MustNavigate(s.URL())

	wait1 := g.browser.MustWaitDownload()
	page.MustElement("#a").MustClick()
	data := wait1()
	g.Eq("test data", string(data))

	wait2 := g.browser.MustWaitDownload()
	page.MustElement("#b").MustClick()
	data = wait2()
	g.Eq("test blob", string(data))
}

func TestWaitDownloadCancel(t *testing.T) {
	g := setup(t)

	wait := g.browser.Context(g.Timeout(0)).WaitDownload(os.TempDir())
	g.Eq(wait(), (*proto.PageDownloadWillBegin)(nil)) //nolint: staticcheck // what WaitDownload returns
}

func TestWaitDownloadFromNewPage(t *testing.T) {
	g := setup(t)

	s := g.Serve()
	content := "test content"

	s.Route("/d", ".bin", content)
	s.Route("/page", ".html", fmt.Sprintf(
		`<html><a href="%s/d" download target="_blank">click</a></html>`,
		s.URL()),
	)

	page := g.page.MustNavigate(s.URL("/page"))
	wait := g.browser.MustWaitDownload()
	page.MustElement("a").MustClick()
	data := wait()

	g.Eq(content, string(data))
}

func TestBrowserConnectErr(t *testing.T) {
	g := setup(t)

	g.Panic(func() {
		wand.New().ControlURL(g.RandStr(16)).MustConnect()
	})
}

func TestStreamReader(t *testing.T) {
	g := setup(t)

	r := wand.NewStreamReader(g.page, "")

	g.mc.stub(1, proto.IORead{}, func(_ StubSend) (lazyjson.JSON, error) {
		return lazyjson.New(proto.IOReadResult{
			Data: "test",
		}), nil
	})
	b := make([]byte, 4)
	_, _ = r.Read(b)
	g.Eq("test", string(b))

	g.mc.stubErr(1, proto.IORead{})
	_, err := r.Read(nil)
	g.Err(err)

	g.mc.stub(1, proto.IORead{}, func(_ StubSend) (lazyjson.JSON, error) {
		return lazyjson.New(proto.IOReadResult{
			Base64Encoded: true,
			Data:          "@",
		}), nil
	})
	_, err = r.Read(nil)
	g.Err(err)
}

func TestBrowserConnectFailure(t *testing.T) {
	g := setup(t)

	c := g.Context()
	c.Cancel()
	err := wand.New().Context(c).Connect()
	if err == nil {
		g.Fatal("expected an error on connect failure")
	}
}

func TestBrowserPool(t *testing.T) {
	g := setup(t)

	u := g.launch(launcher.New())

	pool := wand.NewBrowserPool(3)

	b, err := pool.Get(func() (*wand.Browser, error) {
		browser := wand.New().ControlURL(u)
		return browser, browser.Connect()
	})
	g.E(err)
	pool.Put(b)

	b = pool.MustGet(func() *wand.Browser { return wand.New().ControlURL(u).MustConnect() })
	pool.Put(b)

	pool.Cleanup(func(p *wand.Browser) {
		p.MustClose()
	})
}

// TestBrowserCloseLaunched: a browser Connect launched itself is wand's own,
// so Close waits for it to exit and removes its user data directory, whether
// the browser took the close or had to be killed.
func TestBrowserCloseLaunched(t *testing.T) {
	g := setup(t)

	userDataDir := func(b *wand.Browser) string {
		res, err := proto.BrowserGetBrowserCommandLine{}.Call(b)
		g.E(err)
		for _, arg := range res.Arguments {
			if dir, ok := strings.CutPrefix(arg, "--user-data-dir="); ok {
				g.PathExists(dir)
				return dir
			}
		}
		g.Fatal("no --user-data-dir in the browser command line")
		return ""
	}
	gone := func(dir string) {
		_, err := os.Stat(dir)
		g.True(os.IsNotExist(err))
	}

	b := wand.New().MustConnect()
	dir := userDataDir(b)
	b.MustClose()
	gone(dir)

	// A close the browser does not take, here one whose context is over.
	b = wand.New().MustConnect()
	dir = userDataDir(b)
	ctx, cancel := context.WithCancel(g.Context())
	cancel()
	g.Err(b.Context(ctx).Close())
	gone(dir)
}

func TestBrowserLostConnection(t *testing.T) {
	g := setup(t)

	l := launcher.New()
	p := wand.New().ControlURL(g.launch(l)).MustConnect().MustPage(g.blank())

	go func() {
		utils.Sleep(1)
		l.Kill()
	}()

	_, err := p.Eval(`() => new Promise(r => {})`)
	g.Err(err)
}

func TestBrowserConnectConflict(t *testing.T) {
	g := setup(t)
	g.Panic(func() {
		wand.New().Client(&cdp.Client{}).ControlURL("test").MustConnect()
	})
}

func TestBrowserVersionMismatch(t *testing.T) {
	g := setup(t)

	// A browser of its own, so that each connection below is a fresh one
	// whose Browser.getVersion answer the mock decides.
	u := g.launch(launcher.New())

	// connect a fresh client whose Browser.getVersion answer is stubbed, and
	// return what the browser logged.
	connect := func(stub func(mc *MockClient)) string {
		mc := newMockClient(u)
		mc.t = t
		stub(mc)

		logs := &bytes.Buffer{}
		b := wand.New().Client(mc).Logger(log.New(logs, "", 0))
		g.E(b.Connect())
		g.Cleanup(func() { _ = b.Close() })

		return logs.String()
	}
	product := func(product string) func(mc *MockClient) {
		return func(mc *MockClient) {
			mc.stub(1, proto.BrowserGetVersion{}, func(_ StubSend) (lazyjson.JSON, error) {
				return lazyjson.New(proto.BrowserGetVersionResult{Product: product}), nil
			})
		}
	}

	// Another major version than the Target Chrome's: one line, both
	// versions, and the connection stands.
	line := connect(product("Chrome/1.0.0.0"))
	g.Has(line, "Chrome/1.0.0.0")
	g.Has(line, pins.ChromeVersion)
	g.Eq(strings.Count(line, "\n"), 1)

	// The Target Chrome's major version, headless or not: nothing.
	g.Eq(connect(product("HeadlessChrome/"+pins.ChromeVersion)), "")

	// A version that cannot be read: one line saying so, and the connection
	// stands, since wand never refuses a browser.
	line = connect(func(mc *MockClient) { mc.stubErr(1, proto.BrowserGetVersion{}) })
	g.Has(line, "could not be read")
	g.Eq(strings.Count(line, "\n"), 1)

	// Target discovery that cannot be switched on still fails the connection.
	mc := newMockClient(u)
	mc.t = t
	mc.stubErr(1, proto.TargetSetDiscoverTargets{})
	g.Err(wand.New().Client(mc).Connect())
}
