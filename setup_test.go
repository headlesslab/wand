package wand_test

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/headlesslab/lazyjson"
	"github.com/headlesslab/leakcheck"
	"github.com/headlesslab/wand"
	"github.com/headlesslab/wand/lib/cdp"
	"github.com/headlesslab/wand/lib/defaults"
	"github.com/headlesslab/wand/lib/launcher"
	"github.com/headlesslab/wand/lib/proto"
	"github.com/headlesslab/wand/lib/utils"
	"github.com/ysmood/got"
)

// TimeoutEach is how long one test may run. A test still running after it
// has its browser closed, so that a call hung on the browser returns an error
// and the test fails on its own, with the goroutine dump above its output,
// while the run goes on; a test still running another TimeoutEach later, hung
// on something else, ends the run.
var TimeoutEach = flag.Duration("timeout-each", time.Minute, "timeout for each test")

var LogDir = slash(fmt.Sprintf("tmp/cdp-log/%s", time.Now().Format("2006-01-02_15-04-05")))

// browserBin is the browser Browser resolution found in TestMain, which every
// tester launches: an explicit path, WAND_BROWSER_BIN in the Gate, a System
// browser on a developer machine, the Managed browser otherwise (spec #33,
// section 12). There is no test-only browser knob.
var browserBin string

var testerPool wand.Pool[G]

// noInternet is the browser switch every tester and every browser a test
// launches through launch gets: no host but loopback resolves, so a test
// that reaches the public internet fails on every machine, instead of
// passing where there is a network and failing where there is none (spec
// #33, section 12). The Managed browser download, the one network access
// the suite may make, happens before any browser starts.
const noInternet = "MAP * ~NOTFOUND, EXCLUDE localhost, EXCLUDE 127.0.0.1"

func TestMain(m *testing.M) {
	got.DefaultFlags("timeout=5m", "run=/")

	utils.E(os.MkdirAll(slash("tmp/cdp-log"), 0o755))

	os.Exit(run(m))
}

// run is the body of TestMain, so that the browsers are closed and their user
// data directories removed whatever the outcome, before the process exits.
func run(m *testing.M) int {
	bin, err := launcher.New().ResolveBin()
	if err != nil {
		log.Printf("browser resolution: %v", err)
		return 1
	}
	browserBin = bin

	testerPool = newTesterPool()

	// The first tester is launched here, once: a browser that does not start
	// fails the run at once, with its path, and one that does prints its
	// version, so every log shows the browser actually used.
	first, err := testerPool.Get(launchTester)
	if err != nil {
		stopTesters()
		log.Printf("browser %s: %v", bin, err)
		return 1
	}
	version, err := first.browser.Version()
	if err != nil {
		stopTesters()
		log.Printf("browser %s: %v", bin, err)
		return 1
	}
	fmt.Printf("browser: %s (%s), %d pooled testers\n", bin, version.Product, cap(testerPool)) //nolint: forbidigo
	testerPool.Put(first)

	code := m.Run()

	stopTesters()

	if code != 0 {
		return code
	}

	if err := leakcheck.Check(0, leakcheck.IgnoreFuncs("internal/poll.runtime_pollWait")); err != nil {
		log.Print(err)
		return 1
	}

	return 0
}

// G is a tester. Testers are thread-safe, they shouldn't race each other.
type G struct {
	got.G

	// mock client for proxy the cdp requests
	mc *MockClient

	// a random browser instance from the pool. If you have changed state of it, you must reset it
	// or it may affect other test cases.
	browser *wand.Browser

	// a random page instance from the pool. If you have changed state of it, you must reset it
	// or it may affect other test cases.
	page *wand.Page

	// launcher of the browser, kept so that the browser and its user data
	// directory are cleaned up whatever happens to the tests that use it.
	launcher *launcher.Launcher

	// use is who holds the tester, and whether it was retired; gen is the
	// generation of the test that holds it now. Pointers, as a G is passed by
	// value.
	use *testerUse
	gen int

	// expectPanic counts the Panic blocks in progress, inside which a failing
	// Must call panics as the block expects (see onPanic).
	expectPanic *atomic.Int32

	// testGoroutine is the goroutine running the current test.
	testGoroutine *atomic.Int64

	// stop closes the browser and removes its user data directory, once.
	stop func()

	// use it to cancel the TimeoutEach for current test case
	cancelTimeout func()
}

// The pooled tester count follows go test's -parallel, which is GOMAXPROCS
// unless set, so a hosted 4-vCPU runner launches four browsers per job. If we
// don't use pool to cache, the total time will be much longer.
func newTesterPool() wand.Pool[G] {
	parallel := got.Parallel()
	if parallel == 0 {
		parallel = runtime.GOMAXPROCS(0)
	}

	return wand.NewPool[G](parallel)
}

// testers is every tester the run launched, in the pool or held by a test,
// so that stopTesters reaches them all.
var testers struct {
	sync.Mutex

	list []*G
}

// testerUse is who holds a tester: the test in progress, by generation, so
// that a retirement asked for by the TimeoutEach watcher of an earlier test,
// which may fire as that test ends, never reaches a tester that has gone back
// to the pool or on to another test.
type testerUse struct {
	sync.Mutex

	gen     int  // incremented each time a test takes the tester
	held    bool // a test holds the tester
	retired bool // out of service; the pool gets a placeholder instead
}

// take marks the tester held by a new test and returns its generation.
func (u *testerUse) take() int {
	u.Lock()
	defer u.Unlock()

	u.gen++
	u.held = true

	return u.gen
}

// release marks the tester returned and reports whether it was retired.
func (u *testerUse) release() bool {
	u.Lock()
	defer u.Unlock()

	u.held = false

	return u.retired
}

// retire marks the tester retired if the test of generation gen still holds
// it, and reports whether it did.
func (u *testerUse) retire(gen int) bool {
	u.Lock()
	defer u.Unlock()

	if !u.held || u.gen != gen || u.retired {
		return false
	}

	u.retired = true

	return true
}

// launchTester launches a browser through launcher.New() on the browser
// TestMain resolved, and connects a tester to it.
func launchTester() (*G, error) {
	// No proxy, whatever the machine has: loopback stays local, and the
	// host resolver rules see every other host, which a proxy would resolve.
	l := launcher.New().Bin(browserBin).
		Set("no-proxy-server").
		Set("host-resolver-rules", noInternet).
		NoSandbox(true)

	u, err := l.Launch()
	if err != nil {
		return nil, err
	}

	g := &G{
		launcher:      l,
		use:           &testerUse{},
		expectPanic:   &atomic.Int32{},
		testGoroutine: &atomic.Int64{},
	}
	g.mc = newMockClient(u)
	g.browser = wand.New().Client(g.mc).WithPanic(g.onPanic)
	g.stop = sync.OnceFunc(func() {
		_ = g.browser.Close()
		l.Cleanup()
	})

	if err := g.browser.Connect(); err != nil {
		g.stop()
		return nil, err
	}

	if err := g.browser.IgnoreCertErrors(false); err != nil {
		g.stop()
		return nil, err
	}

	pages, err := g.browser.Pages()
	if err != nil {
		g.stop()
		return nil, err
	}

	if pages.Empty() {
		g.page, err = g.browser.Page(proto.TargetCreateTarget{})
		if err != nil {
			g.stop()
			return nil, err
		}
	} else {
		g.page = pages.First()
	}

	testers.Lock()
	testers.list = append(testers.list, g)
	testers.Unlock()

	return g, nil
}

// stopTesters closes every browser the run launched, in the pool or held by
// a test, and waits until their user data directories are removed; the
// launcher kills a browser that does not exit within its bound.
func stopTesters() {
	testers.Lock()
	list := testers.list
	testers.list = nil
	testers.Unlock()

	wg := sync.WaitGroup{}
	for _, g := range list {
		wg.Add(1)
		go func(g *G) {
			defer wg.Done()
			g.stop()
		}(g)
	}
	wg.Wait()
}

// retire takes the tester out of service on behalf of the test of generation
// gen, if that test still holds it: its browser is closed, or killed when it
// does not go, its user data directory removed, and the pool gets a
// placeholder in its place once the test returns it, so what a failed or hung
// test left in a browser never reaches another test.
func (g *G) retire(gen int) {
	if g.use.retire(gen) {
		g.stop()
	}
}

// onPanic is what a failing Must call on the pooled browser, its pages and
// their elements does instead of panicking. Inside a Panic block, or off the
// goroutine running the test, it panics all the same, which is what the caller
// waits for. On the test's goroutine it fails the test with the error and the
// stack and ends the test, so that the run goes on to the next test and ends
// with its cleanup and its coverage, where a panic would have killed the
// binary, browsers and directories left where they were.
func (g *G) onPanic(v interface{}) {
	if g.expectPanic.Load() > 0 || leakcheck.Get(false)[0].GoroutineID != g.testGoroutine.Load() {
		panic(v)
	}

	g.Fatalf("%v\n%s", v, debug.Stack())
}

// Panic is got's Panic, with the tester told that fn is expected to panic, so
// that a failing Must call inside it panics rather than ending the test.
func (g G) Panic(fn func()) interface{} {
	g.Helper()

	g.expectPanic.Add(1)
	defer g.expectPanic.Add(-1)

	return g.G.Panic(fn)
}

func setup(t *testing.T) G {
	t.Helper()

	if got.Parallel() != 1 {
		t.Parallel()
	}

	// A tester that fails to launch fails this test alone; its slot goes back
	// to the pool as a placeholder, so the next test tries again.
	tester, err := testerPool.Get(launchTester)
	if err != nil {
		testerPool.Put(nil)
		t.Fatalf("launching a tester: %v", err)
	}

	tester.gen = tester.use.take()
	t.Cleanup(func() {
		if tester.use.release() {
			testerPool.Put(nil)
			return
		}
		testerPool.Put(tester)
	})

	tester.G = got.New(t)
	tester.mc.t = t
	tester.mc.log.SetOutput(tester.Open(true, filepath.Join(LogDir, tester.mc.id, t.Name()+".log")))

	tester.checkLeaking()

	tester.page.MustNavigate("")

	return *tester
}

func (g G) blank() string {
	return g.srcFile("./fixtures/blank.html")
}

func (g G) html(content string) string {
	return g.Serve().Route("/", "", content).URL()
}

// Get abs file path from fixtures folder, such as "file:///a/b/click.html".
// Usually the path can be used for html src attribute like:
//
//	<img src="file:///a/b">
func (g G) srcFile(path string) string {
	g.Helper()
	f, err := filepath.Abs(slash(path))
	g.E(err)
	return "file://" + f
}

func (g G) newPage(u ...string) *wand.Page {
	g.Helper()
	p := g.browser.MustPage(u...)
	g.Cleanup(func() {
		if !g.Failed() {
			p.MustClose()
		}
	})
	return p
}

// otherTargets lists the page targets of the tester's browser other than its
// own page: what a test opens and may leave behind. The browser's own UI
// targets (browser_ui, chrome://omnibox-popup in the new headless mode),
// workers and frames belong to the browser or to a page and go with them.
func (g *G) otherTargets() []*proto.TargetTargetInfo {
	res, err := proto.TargetGetTargets{}.Call(g.browser)
	g.E(err)

	var others []*proto.TargetTargetInfo
	for _, info := range res.TargetInfos {
		if info.Type == "page" && info.TargetID != g.page.TargetID {
			others = append(others, info)
		}
	}

	return others
}

// launch starts a browser of the test's own through l, on the browser
// TestMain resolved, and makes sure it is gone with its user data directory
// when the test ends, whether the test closed it or not (the launcher kills
// a browser that does not exit within its bound).
func (g G) launch(l *launcher.Launcher) string {
	g.Helper()
	u := l.Bin(browserBin).Set("no-proxy-server").Set("host-resolver-rules", noInternet).MustLaunch()
	g.Cleanup(l.Cleanup)
	return u
}

func (g *G) checkLeaking() {
	ig := leakcheck.CombineIgnores(leakcheck.IgnoreCurrent(), leakcheck.IgnoreNonChildren())
	leakcheck.CheckLeak(g.Testable, 0, ig)

	self := leakcheck.Get(false)[0]
	g.testGoroutine.Store(self.GoroutineID)

	// What the watcher below may use once the test has finished: its name
	// and its generation, never the testing.T, whose Log or Fail after the
	// test completed panics the binary.
	name, gen := g.Name(), g.gen

	done := make(chan struct{})
	g.cancelTimeout = g.DoAfter(*TimeoutEach, func() {
		trace := leakcheck.Get(true).Filter(func(t *leakcheck.Trace) bool {
			if t.GoroutineID == self.GoroutineID {
				return false
			}
			return ig(t)
		}).String()

		// The browser is closed instead, so a call hung on the browser
		// returns an error and the test fails on its own, with this above.
		_, _ = fmt.Fprintf(os.Stderr, "[wand_test.TimeoutEach] %s timeout after %v, closing its browser\nrunning goroutines: %s\n",
			name, *TimeoutEach, trace)
		g.retire(gen)

		// A test hung on something other than its browser cannot be failed
		// from outside; the run ends, as go test would at its own timeout,
		// once the other browsers are gone.
		select {
		case <-done:
		case <-time.After(*TimeoutEach):
			_, _ = fmt.Fprintf(os.Stderr, "[wand_test.TimeoutEach] %s still running %v after its browser was closed, ending the run\n",
				name, *TimeoutEach)
			stopTesters()
			os.Exit(2)
		}
	})

	g.Cleanup(func() {
		close(done)

		if g.Failed() {
			g.retire(gen)
			return
		}

		// Close every target but g.page, then make sure none is left. A
		// target can still be listed while the browser finishes destroying
		// it: Target.closeTarget answers before the target is gone, and the
		// error path of Browser.Page closes the target it created that way,
		// without waiting. Closing such a target fails with "No target with
		// given id found", which is what closing it here was for; the outcome
		// is what is checked, once the browser has had a moment to finish.
		for _, info := range g.otherTargets() {
			_, err := proto.TargetCloseTarget{TargetID: info.TargetID}.Call(g.browser)
			if err != nil && !strings.Contains(err.Error(), "No target with given id found") {
				g.E(err)
			}
		}

		deadline := time.Now().Add(3 * time.Second)
		left := g.otherTargets()
		for len(left) > 0 && time.Now().Before(deadline) {
			time.Sleep(50 * time.Millisecond)
			left = g.otherTargets()
		}
		for _, info := range left {
			g.Logf("target left after the test: %s %s %s", info.Type, info.TargetID, info.URL)
			g.Fail()
		}

		if g.browser.LoadState(g.page.SessionID, &proto.FetchEnable{}) {
			g.Logf("leaking FetchEnable")
			g.Fail()
		}

		g.mc.setCall(nil)

		if g.Failed() {
			g.retire(gen)
		}
	})
}

type Call func(ctx context.Context, sessionID, method string, params interface{}) ([]byte, error)

var _ wand.CDPClient = &MockClient{}

type MockClient struct {
	sync.RWMutex

	id        string
	t         got.Testable
	log       *log.Logger
	principal *cdp.Client
	call      Call
	event     <-chan *cdp.Event
}

var mockClientCount int32

func newMockClient(u string) *MockClient {
	id := fmt.Sprintf("%02d", atomic.AddInt32(&mockClientCount, 1))

	// create init log file
	utils.E(os.MkdirAll(filepath.Join(LogDir, id), 0o755))
	f, err := os.Create(filepath.Join(LogDir, id, "_.log"))
	log := log.New(f, "", log.Ltime)
	utils.E(err)

	client := cdp.New().Logger(utils.MultiLogger(defaults.CDP, log)).Start(cdp.MustConnectWS(u))

	return &MockClient{id: id, principal: client, log: log}
}

func (mc *MockClient) Event() <-chan *cdp.Event {
	if mc.event != nil {
		return mc.event
	}
	return mc.principal.Event()
}

func (mc *MockClient) Call(ctx context.Context, sessionID, method string, params interface{}) ([]byte, error) {
	return mc.getCall()(ctx, sessionID, method, params)
}

func (mc *MockClient) getCall() Call {
	mc.RLock()
	defer mc.RUnlock()

	if mc.call == nil {
		return mc.principal.Call
	}
	return mc.call
}

func (mc *MockClient) setCall(fn Call) {
	mc.Lock()
	defer mc.Unlock()

	if mc.call != nil {
		mc.t.Logf("leaking MockClient.stub")
		mc.t.Fail()
	}
	mc.call = fn
}

func (mc *MockClient) resetCall() {
	mc.Lock()
	defer mc.Unlock()
	mc.call = nil
}

type StubSend func() (lazyjson.JSON, error)

// When call the cdp.Client.Call the nth time use fn instead.
// Use p to filter method.
func (mc *MockClient) stub(nth int, p proto.Request, fn func(send StubSend) (lazyjson.JSON, error)) {
	if p == nil {
		mc.t.Logf("p must be specified")
		mc.t.FailNow()
	}

	count := int64(0)

	mc.setCall(func(ctx context.Context, sessionID, method string, params interface{}) ([]byte, error) {
		if method == p.ProtoReq() {
			if int(atomic.AddInt64(&count, 1)) == nth {
				mc.resetCall()
				j, err := fn(func() (lazyjson.JSON, error) {
					b, err := mc.principal.Call(ctx, sessionID, method, params)
					return lazyjson.New(b), err
				})
				if err != nil {
					return nil, err
				}
				return j.MarshalJSON()
			}
		}
		return mc.principal.Call(ctx, sessionID, method, params)
	})
}

// When call the cdp.Client.Call the nth time return error.
// Use p to filter method.
func (mc *MockClient) stubErr(nth int, p proto.Request) {
	mc.stub(nth, p, func(_ StubSend) (lazyjson.JSON, error) {
		return lazyjson.New(nil), errors.New("mock error")
	})
}

// panicMessage runs fn, which must panic, and returns the message of what it
// panicked with, so that a test asserts the reason and not only the panic.
func (g G) panicMessage(fn func()) string {
	g.Helper()

	val := g.Panic(fn)
	if err, ok := val.(error); ok {
		return err.Error()
	}

	return fmt.Sprint(val)
}

type MockRoundTripper struct {
	res *http.Response
	err error
}

func (mrt *MockRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return mrt.res, mrt.err
}

type MockReader struct {
	err error
}

func (mr *MockReader) Read(_ []byte) (n int, err error) {
	return 0, mr.err
}

var slash = filepath.FromSlash
