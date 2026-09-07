package wand_test

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/headlesslab/lazyjson"
	"github.com/headlesslab/wand"
	"github.com/headlesslab/wand/lib/proto"
	"github.com/headlesslab/wand/lib/utils"
)

func TestHijack(t *testing.T) {
	g := setup(t)

	s := g.Serve()

	// to simulate a backend server
	s.Route("/", slash("fixtures/fetch.html"))
	s.Mux.HandleFunc("/a", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			panic("wrong http method")
		}

		g.Eq("header", r.Header.Get("Test"))

		b, err := io.ReadAll(r.Body)
		g.E(err)
		g.Eq("a", string(b))

		g.HandleHTTP(".html", "test")(w, r)
	})
	s.Route("/b", "", "b")

	router := g.page.HijackRequests()
	defer router.MustStop()

	router.MustAdd(s.URL("/a"), func(ctx *wand.Hijack) {
		r := ctx.Request.SetContext(g.Context())
		r.Req().Header.Set("Test", "header") // override request header
		r.SetBody([]byte("test"))            // override request body
		r.SetBody(123)                       // override request body
		r.SetBody(r.Body())                  // override request body

		type MyState struct {
			Val int
		}

		ctx.CustomState = &MyState{10}

		g.Eq(http.MethodPost, r.Method())
		g.Eq(s.URL("/a"), r.URL().String())

		g.Eq(proto.NetworkResourceTypeXHR, ctx.Request.Type())
		g.Is(ctx.Request.IsNavigation(), false)
		g.Has(s.URL(), ctx.Request.Header("Origin"))
		g.Len(ctx.Request.Headers(), 6)
		g.True(ctx.Request.JSONBody().Nil())

		// send request load response from real destination as the default value to hijack
		ctx.MustLoadResponse()

		g.Eq(200, ctx.Response.Payload().ResponseCode)

		// override status code
		ctx.Response.Payload().ResponseCode = http.StatusCreated

		g.Eq("4", ctx.Response.Headers().Get("Content-Length"))
		g.Has(ctx.Response.Headers().Get("Content-Type"), "text/html; charset=utf-8")

		// override response header
		ctx.Response.AddHeader("Set-Cookie", "key=val1")
		// This should override the previous one
		ctx.Response.SetHeader("Set-Cookie", "key=val")

		// override response body
		ctx.Response.SetBody([]byte("test"))
		ctx.Response.SetBody("test")
		ctx.Response.SetBody(map[string]string{
			"text": "test",
		})

		g.Eq("{\"text\":\"test\"}", ctx.Response.Body())
	})

	router.MustAdd(s.URL("/b"), func(_ *wand.Hijack) {
		panic("should not come to here")
	})
	router.MustRemove(s.URL("/b"))

	router.MustAdd(s.URL("/b"), func(ctx *wand.Hijack) {
		// transparent proxy
		ctx.MustLoadResponse()
	})

	go router.Run()

	g.page.MustNavigate(s.URL())

	g.Eq("201 test key=val", g.page.MustElement("#a").MustText())
	g.Eq("b", g.page.MustElement("#b").MustText())
}

func TestHijackContinue(t *testing.T) {
	g := setup(t)

	s := g.Serve().Route("/", ".html", `<body>ok</body>`)

	router := g.page.HijackRequests()
	defer router.MustStop()

	wg := &sync.WaitGroup{}
	wg.Add(1)
	router.MustAdd(s.URL("/a"), func(ctx *wand.Hijack) {
		ctx.ContinueRequest(&proto.FetchContinueRequest{})
		wg.Done()
	})

	go router.Run()

	g.page.MustNavigate(s.URL("/a"))

	g.Eq("ok", g.page.MustElement("body").MustText())
	wg.Wait()
}

func TestHijackMockWholeResponseEmptyBody(t *testing.T) {
	g := setup(t)

	router := g.page.HijackRequests()
	defer router.MustStop()

	router.MustAdd("*", func(ctx *wand.Hijack) {
		ctx.Response.SetBody("")
	})

	go router.Run()

	// A fulfilment with a null body hung the navigation (rod #1128, fixed in
	// HijackResponse.SetBody), which upstream bounded with a one-second
	// timeout. The bound stays as a failure that names this test, but wide:
	// on a hosted runner under -race with four browsers, one second was not
	// always enough for the browser to reach the request.
	timed := g.page.Timeout(15 * time.Second)
	timed.MustNavigate(g.Serve().Route("/", ".txt", "OK").URL())

	g.Eq("", g.page.MustElement("body").MustText())
}

// TestHijackMockWholeResponseNoBody: a handler that sets no body fulfils an
// empty 200 response. Upstream sent the body as null, which the browser
// rejects as invalid parameters ("binary value expected"), so the request
// stayed paused and the navigation hung until the caller's deadline; its test
// asserted that hang, and was skipped as flaky.
func TestHijackMockWholeResponseNoBody(t *testing.T) {
	g := setup(t)

	router := g.page.HijackRequests()
	defer router.MustStop()

	// intercept and reply without setting a body
	router.MustAdd("*", func(_ *wand.Hijack) {})

	go router.Run()

	// The deadline turns a request left paused into a failure, not a hang.
	g.page.Timeout(10 * time.Second).MustNavigate(g.Serve().Route("/", ".html", "<body>served</body>").URL())

	g.Eq("", g.page.MustElement("body").MustText())
}

func TestHijackMockWholeResponse(t *testing.T) {
	g := setup(t)

	router := g.page.HijackRequests()
	defer router.MustStop()

	router.MustAdd("*", func(ctx *wand.Hijack) {
		ctx.Response.SetHeader("Content-Type", mime.TypeByExtension(".html"))
		ctx.Response.SetBody("<body>ok</body>")
	})

	go router.Run()

	g.page.MustNavigate("http://localhost")

	g.Eq("ok", g.page.MustElement("body").MustText())
}

func TestHijackSkip(t *testing.T) {
	g := setup(t)

	s := g.Serve()

	router := g.page.HijackRequests()
	defer router.MustStop()

	wg := &sync.WaitGroup{}
	wg.Add(2)
	router.MustAdd(s.URL("/a"), func(ctx *wand.Hijack) {
		ctx.Skip = true
		wg.Done()
	})
	router.MustAdd(s.URL("/a"), func(ctx *wand.Hijack) {
		ctx.ContinueRequest(&proto.FetchContinueRequest{})
		wg.Done()
	})

	go router.Run()

	g.page.MustNavigate(s.URL("/a"))

	wg.Wait()
}

func TestHijackOnErrorLog(t *testing.T) {
	g := setup(t)

	s := g.Serve().Route("/", ".html", `<body>ok</body>`)

	router := g.page.HijackRequests()
	defer router.MustStop()

	wg := &sync.WaitGroup{}
	wg.Add(1)
	var err error

	router.MustAdd(s.URL("/a"), func(ctx *wand.Hijack) {
		ctx.OnError = func(e error) {
			err = e
			wg.Done()
		}
		ctx.ContinueRequest(&proto.FetchContinueRequest{})
	})

	go router.Run()

	g.mc.stub(1, proto.FetchContinueRequest{}, func(_ StubSend) (lazyjson.JSON, error) {
		return lazyjson.New(nil), errors.New("err")
	})

	go func() {
		_ = g.page.Context(g.Context()).Navigate(s.URL("/a"))
	}()
	wg.Wait()

	g.Eq(err.Error(), "err")
}

func TestHijackFailRequest(t *testing.T) {
	g := setup(t)

	s := g.Serve().Route("/page", ".html", `<html>
	<body></body>
	<script>
		fetch('/a').catch(async (err) => {
			document.title = err.message
		})
	</script></html>`)

	router := g.browser.HijackRequests()
	defer router.MustStop()

	router.MustAdd(s.URL("/a"), func(ctx *wand.Hijack) {
		ctx.Response.Fail(proto.NetworkErrorReasonAborted)
	})

	go router.Run()

	g.page.MustNavigate(s.URL("/page")).MustWaitLoad()

	g.page.MustWait(`() => document.title === 'Failed to fetch'`)

	{ // test error log
		g.mc.stub(1, proto.FetchFailRequest{}, func(send StubSend) (lazyjson.JSON, error) {
			_, _ = send()
			return lazyjson.JSON{}, errors.New("err")
		})
		_ = g.page.Navigate(s.URL("/a"))
	}
}

func TestHijackLoadResponseErr(t *testing.T) {
	g := setup(t)

	p := g.newPage().Context(g.Context())
	router := p.HijackRequests()
	defer router.MustStop()

	wg := &sync.WaitGroup{}
	wg.Add(1)

	router.MustAdd("http://localhost/a", func(ctx *wand.Hijack) {
		g.Err(ctx.LoadResponse(&http.Client{
			Transport: &MockRoundTripper{err: errors.New("err")},
		}, true))

		g.Err(ctx.LoadResponse(&http.Client{
			Transport: &MockRoundTripper{res: &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(&MockReader{err: errors.New("err")}),
			}},
		}, true))

		// a request body that cannot be read
		ctx.Request.Req().Body = io.NopCloser(&MockReader{err: errors.New("err")})
		g.Err(ctx.LoadResponse(http.DefaultClient, true))

		wg.Done()

		ctx.Response.Fail(proto.NetworkErrorReasonAborted)
	})

	go router.Run()

	_ = p.Navigate("http://localhost/a")

	wg.Wait()
}

func TestHijackResponseErr(t *testing.T) {
	g := setup(t)

	s := g.Serve().Route("/", ".html", `ok`)

	p := g.newPage().Context(g.Context())
	router := p.HijackRequests()
	defer router.MustStop()

	wg := &sync.WaitGroup{}
	wg.Add(1)

	router.MustAdd(s.URL("/a"), func(ctx *wand.Hijack) { // to ignore favicon
		ctx.OnError = func(err error) {
			g.Err(err)
			wg.Done()
		}

		ctx.MustLoadResponse()
		g.mc.stub(1, proto.FetchFulfillRequest{}, func(send StubSend) (lazyjson.JSON, error) {
			res, _ := send()
			return res, errors.New("err")
		})
	})

	go router.Run()

	p.MustNavigate(s.URL("/a"))

	wg.Wait()
}

func TestHandleAuth(t *testing.T) {
	g := setup(t)

	s := g.Serve()

	// mock the server
	s.Mux.HandleFunc("/a", func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok {
			w.Header().Add("WWW-Authenticate", `Basic realm="web"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		g.Eq("a", u)
		g.Eq("b", p)
		g.HandleHTTP(".html", `<p>ok</p>`)(w, r)
	})
	s.Route("/err", ".html", "err page")

	go g.browser.MustHandleAuth("a", "b")()

	page := g.newPage(s.URL("/a"))
	page.MustElementR("p", "ok")

	wait := g.browser.HandleAuth("a", "b")
	var page2 *wand.Page
	wait2 := utils.All(func() {
		page2, _ = g.browser.Page(proto.TargetCreateTarget{URL: s.URL("/err")})
	})
	g.mc.stubErr(1, proto.FetchContinueRequest{})
	g.Err(wait())
	wait2()
	page2.MustClose()
}

// TestHijackLoadResponseRedirectBody: LoadResponse sends the body the handler
// set with its length, and the client sends it again on every 307 the server
// answers with, so the redirect chain is followed by the client and the
// browser sees one response (rod #1128).
func TestHijackLoadResponseRedirectBody(t *testing.T) {
	g := setup(t)

	var mu sync.Mutex
	redirects, hits, status := 0, 0, 0
	var loadErr error

	s := g.Serve()
	s.Mux.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		g.Eq(r.Method, http.MethodPost)
		b, err := io.ReadAll(r.Body)
		g.E(err)
		g.Eq(string(b), "test")
		g.Eq(r.ContentLength, int64(4))

		mu.Lock()
		defer mu.Unlock()
		if redirects < 3 {
			redirects++
			w.Header().Set("Location", s.URL("/test"))
			w.WriteHeader(http.StatusTemporaryRedirect)
			return
		}
		g.HandleHTTP(".html", "OK")(w, r)
	})

	router := g.page.HijackRequests()
	defer router.MustStop()

	router.MustAdd(s.URL("/test"), func(ctx *wand.Hijack) {
		ctx.Request.Req().Method = http.MethodPost
		ctx.Request.SetBody("test")

		err := ctx.LoadResponse(http.DefaultClient, true)

		mu.Lock()
		defer mu.Unlock()
		hits++
		loadErr = err
		if err == nil {
			status = ctx.Response.RawResponse.StatusCode
		}
	})

	go router.Run()

	g.page.MustNavigate(s.URL("/test"))
	g.Eq(g.page.MustElement("body").MustText(), "OK")

	mu.Lock()
	defer mu.Unlock()
	g.E(loadErr)
	g.Eq(redirects, 3)
	g.Eq(hits, 1)
	g.Eq(status, http.StatusOK)
}

// TestHijackLoadResponseBodyAgain: whatever body the request carries when
// LoadResponse runs goes out with its length, an empty one as no body, and a
// LoadResponse after a failed one sends the body again.
func TestHijackLoadResponseBodyAgain(t *testing.T) {
	g := setup(t)

	type seen struct {
		method string
		length int64
		body   string
	}
	var mu sync.Mutex
	seenBy := map[string]seen{}

	// Every request is redirected once, so that the client sends its body twice.
	s := g.Serve()
	s.Mux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		if !r.URL.Query().Has("hop") {
			w.Header().Set("Location", r.URL.String()+"&hop=1")
			w.WriteHeader(http.StatusTemporaryRedirect)
			return
		}
		b, err := io.ReadAll(r.Body)
		g.E(err)
		mu.Lock()
		seenBy[r.URL.Query().Get("case")] = seen{r.Method, r.ContentLength, string(b)}
		mu.Unlock()
		g.HandleHTTP(".html", "OK")(w, r)
	})

	router := g.page.HijackRequests()
	defer router.MustStop()

	router.MustAdd(s.URL("/echo*"), func(ctx *wand.Hijack) {
		switch ctx.Request.URL().Query().Get("case") {
		case "swap":
			ctx.Request.Req().Method = http.MethodPost
			ctx.Request.Req().Body = io.NopCloser(strings.NewReader("swapped"))
		case "empty":
			ctx.Request.Req().Method = http.MethodPost
			ctx.Request.SetBody("")
		case "again":
			ctx.Request.Req().Method = http.MethodPost
			ctx.Request.SetBody("again")
			g.Err(ctx.LoadResponse(&http.Client{Transport: readThenFail{}}, true))
		}
		ctx.MustLoadResponse()
	})

	go router.Run()

	for _, c := range []string{"swap", "empty", "again"} {
		g.page.MustNavigate(s.URL("/echo?case=" + c))
		g.Eq(g.page.MustElement("body").MustText(), "OK")
	}

	mu.Lock()
	defer mu.Unlock()
	g.Eq(seenBy["swap"], seen{http.MethodPost, 7, "swapped"})
	g.Eq(seenBy["empty"], seen{http.MethodPost, 0, ""})
	g.Eq(seenBy["again"], seen{http.MethodPost, 5, "again"})
}

// readThenFail is a transport that consumes the request body, as a real one
// does before the connection fails, and then fails.
type readThenFail struct{}

func (readThenFail) RoundTrip(req *http.Request) (*http.Response, error) {
	_, _ = io.ReadAll(req.Body)
	_ = req.Body.Close()
	return nil, errors.New("connection lost")
}

// TestHijackAddPattern: a pattern with wildcards in a row is the browser's
// pattern and matches (rod #982); a "?" matches no character as well as one,
// as the browser reads it, so a request the browser pauses for the pattern
// reaches the handler instead of staying paused; and the one thing the regexp
// package refuses, a pattern that is not valid UTF-8, is an error rather than
// a panic (rod #983), after which the router works as before.
func TestHijackAddPattern(t *testing.T) {
	g := setup(t)

	s := g.Serve().Route("/a", ".html", `<body>ok</body>`).Route("/b", ".html", `<body>ok</body>`)

	router := g.page.HijackRequests()
	defer router.MustStop()

	var hits atomic.Int32
	handler := func(ctx *wand.Hijack) {
		hits.Add(1)
		ctx.MustLoadResponse()
	}
	router.MustAdd("**"+s.URL("/a")+"**", handler)
	router.MustAdd(s.URL("/b?"), handler)

	g.Err(router.Add("\xff", "", func(*wand.Hijack) {}))

	go router.Run()

	g.page.MustNavigate(s.URL("/a"))
	g.Eq(g.page.MustElement("body").MustText(), "ok")
	g.page.Timeout(10 * time.Second).MustNavigate(s.URL("/b"))
	g.Eq(g.page.MustElement("body").MustText(), "ok")
	g.Eq(hits.Load(), int32(2))
}
