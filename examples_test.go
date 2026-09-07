package wand_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/headlesslab/lazyjson"
	"github.com/headlesslab/wand"
	"github.com/headlesslab/wand/lib/cdp"
	"github.com/headlesslab/wand/lib/input"
	"github.com/headlesslab/wand/lib/launcher"
	"github.com/headlesslab/wand/lib/proto"
	"github.com/headlesslab/wand/lib/utils"
)

// exampleFixtures serves fixtures/examples, started by the first example that
// asks for a page and stopped by run once the tests are over.
var exampleFixtures struct {
	sync.Mutex

	srv *httptest.Server
}

// fixtureURL is the address of a page under fixtures/examples, on a loopback
// port the OS picks. The examples below drive those pages instead of live
// websites, so they need no network, take no port of the machine, and their
// output never changes under them (spec #33, section 12). Your own code would
// pass the address of the site you automate.
func fixtureURL(name string) string {
	exampleFixtures.Lock()
	defer exampleFixtures.Unlock()

	if exampleFixtures.srv == nil {
		exampleFixtures.srv = httptest.NewServer(http.FileServer(http.Dir(slash("fixtures/examples"))))
	}

	return exampleFixtures.srv.URL + "/" + name
}

// stopExampleFixtures shuts the fixtures server down, so that a run leaves no
// listener and no goroutine of it behind.
func stopExampleFixtures() {
	exampleFixtures.Lock()
	defer exampleFixtures.Unlock()

	if exampleFixtures.srv != nil {
		exampleFixtures.srv.Close()
		exampleFixtures.srv = nil
	}
}

// outFile is where the examples below write the files they produce: the run's
// tmp directory, which git ignores. Your own code would pass a path of its
// own, such as "my.png".
func outFile(name string) string {
	path := slash("tmp/examples/" + name)
	utils.E(os.MkdirAll(filepath.Dir(path), 0o755))

	return path
}

// This example opens a search page, searches for "git",
// and then gets the element that gives the description for Git.
//
// The page is one of this repository's own, under fixtures/examples, served
// on a loopback port: every example here drives a local page rather than a
// live website, so the whole set runs offline. Where an example calls
// fixtureURL, pass the address of the site you automate instead.
func Example_basic() {
	// Launch a new browser with default options, and connect to it.
	browser := wand.New().MustConnect()

	// Even you forget to close, wand will close it after main process ends.
	defer browser.MustClose()

	// Create a new page
	page := browser.MustPage(fixtureURL("search.html")).MustWaitStable()

	// Trigger the search input with hotkey "/"
	page.Keyboard.MustType(input.Slash)

	// We use css selector to get the search input element and input "git"
	page.MustElement("#query").MustInput("git").MustType(input.Enter)

	// Wait until css selector get the element then get the text content of it.
	text := page.MustElementR("span", "most widely used").MustText()

	fmt.Println(text)

	// Get all input elements. wand supports query elements by css selector, xpath, and regex.
	// For more detailed usage, check the query_test.go file.
	fmt.Println("Found", len(page.MustElements("input")), "input elements")

	// Eval js on the page
	page.MustEval(`() => console.log("hello world")`)

	// Pass parameters as json objects to the js function. This MustEval will result 3
	fmt.Println("1 + 2 =", page.MustEval(`(a, b) => a + b`, 1, 2).Int())

	// When eval on an element, "this" in the js is the current DOM element.
	fmt.Println(page.MustElement("title").MustEval(`() => this.innerText`).String())

	// Output:
	// Git is the most widely used version control system.
	// Found 3 input elements
	// 1 + 2 = 3
	// git - wand search
}

// Shows how to disable headless mode and debug.
// wand provides a lot of debug options, you can set them with setter methods or use environment variables.
// Doc for environment variables: https://pkg.go.dev/github.com/headlesslab/wand/lib/defaults
//
// This example has no output comment, so it is compiled but never run: it puts
// a browser window on the screen and then blocks in [utils.Pause], which needs
// a display and a person in front of it.
func Example_disable_headless_to_debug() {
	// Headless runs the browser on foreground, you can also use flag "-wand=show"
	// Devtools opens the tab in each new tab opened automatically
	l := launcher.New().
		Headless(false).
		Devtools(true)

	defer l.Cleanup()

	url := l.MustLaunch()

	// Trace shows verbose debug information for each action executed
	// SlowMotion is a debug related function that waits 2 seconds between
	// each action, making it easier to inspect what your code is doing.
	browser := wand.New().
		ControlURL(url).
		Trace(true).
		SlowMotion(2 * time.Second).
		MustConnect()

	// ServeMonitor plays screenshots of each tab. This feature is extremely
	// useful when debugging with headless mode.
	// You can also enable it with flag "-wand=monitor"
	launcher.Open(browser.ServeMonitor(""))

	defer browser.MustClose()

	page := browser.MustPage(fixtureURL("search.html"))

	page.MustElement("#query").MustInput("git").MustType(input.Enter)

	text := page.MustElementR("span", "most widely used").MustText()

	fmt.Println(text)

	utils.Pause() // pause goroutine
}

// wand use https://golang.org/pkg/context to handle cancellations for IO blocking operations, most times it's timeout.
// Context will be recursively passed to all sub-methods.
// For example, methods like Page.Context(ctx) will return a clone of the page with the ctx,
// all the methods of the returned page will use the ctx if they have IO blocking operations.
// [Page.Timeout] or [Page.WithCancel] is just a shortcut for Page.Context.
// Of course, Browser or Element works the same way.
func Example_context_and_timeout() {
	browser := wand.New().MustConnect()
	defer browser.MustClose()

	page := browser.MustPage(fixtureURL("search.html"))

	title := page.
		// Set a 5-second timeout for all chained methods
		Timeout(5 * time.Second).

		// The total time for MustWaitLoad and MustElement must be less than 5 seconds
		MustWaitLoad().
		MustElement("title").

		// Methods after CancelTimeout won't be affected by the 5-second timeout
		CancelTimeout().

		// Set a 10-second timeout for all chained methods
		Timeout(10 * time.Second).

		// Panics if it takes more than 10 seconds
		MustText()

	fmt.Println(title)

	// The two code blocks below are basically the same:
	{
		page.Timeout(5 * time.Second).MustElement("a").CancelTimeout()
	}
	{
		// Use this way you can customize your own way to cancel long-running task
		page, cancel := page.WithCancel()

		cancelled := make(chan struct{})
		go func() {
			defer close(cancelled)
			time.Sleep(time.Second) // cancel it a second from now
			cancel()
		}()

		page.MustElement("a")

		<-cancelled
	}

	// Output: wand search
}

func Example_context_and_EachEvent() {
	browser := wand.New().MustConnect()
	defer browser.MustClose()

	page := browser.MustPage(fixtureURL("search.html")).MustWaitLoad()

	page, cancel := page.WithCancel()

	go func() {
		time.Sleep(time.Second)
		cancel()
	}()

	// It's a blocking method, it will wait until the context is cancelled
	page.EachEvent(func(_ *proto.PageLifecycleEvent) {})()

	if page.GetContext().Err() == context.Canceled {
		fmt.Println("cancelled")
	}

	// Output: cancelled
}

// We use "Must" prefixed functions to write example code. But in production you may want to use
// the no-prefix version of them.
// About why we use "Must" as the prefix, it's similar to https://golang.org/pkg/regexp/#MustCompile
func Example_error_handling() {
	browser := wand.New().MustConnect()
	defer browser.MustClose()

	page := browser.MustPage(fixtureURL("search.html"))

	// We use Go's standard way to check error types, no magic.
	check := func(err error) {
		var evalErr *wand.EvalError
		if errors.Is(err, context.DeadlineExceeded) { // timeout error
			fmt.Println("timeout err")
		} else if errors.As(err, &evalErr) { // eval error
			fmt.Println(evalErr.LineNumber)
		} else if err != nil {
			fmt.Println("can't handle", err)
		}
	}

	// The two code blocks below are doing the same thing in two styles:

	// The block below is better for debugging or quick scripting. We use panic to short-circuit logics.
	// So that we can take advantage of fluent interface (https://en.wikipedia.org/wiki/Fluent_interface)
	// and fail-fast (https://en.wikipedia.org/wiki/Fail-fast).
	// This style will reduce code, but it may also catch extra errors (less consistent and precise).
	{
		err := wand.Try(func() {
			fmt.Println(page.MustElement("a").MustHTML()) // use "Must" prefixed functions
		})
		check(err)
	}

	// The block below is better for production code. It's the standard way to handle errors.
	// Usually, this style is more consistent and precise.
	{
		el, err := page.Element("a")
		if err != nil {
			check(err)
			return
		}
		html, err := el.HTML()
		if err != nil {
			check(err)
			return
		}
		fmt.Println(html)
	}

	// Output:
	// <a href="./about.html">About</a>
	// <a href="./about.html">About</a>
}

// Example_search shows how to use Search to get element inside nested iframes or shadow DOMs.
// It works the same as https://developers.google.com/web/tools/chrome-devtools/dom#search
func Example_search() {
	browser := wand.New().MustConnect()
	defer browser.MustClose()

	page := browser.MustPage(fixtureURL("map.html"))

	// Click the zoom-in button of the map widget. The button is in a shadow DOM
	// inside an iframe, which a css selector on the page cannot reach.
	page.MustSearch(".zoom-in").MustClick()

	fmt.Println("done")

	// Output: done
}

func Example_page_screenshot() {
	browser := wand.New().MustConnect()
	defer browser.MustClose()

	page := browser.MustPage(fixtureURL("search.html")).MustWaitLoad()

	// simple version
	page.MustScreenshot(outFile("my.png"))

	// customization version
	img, _ := page.Screenshot(true, &proto.PageCaptureScreenshot{
		Format:  proto.PageCaptureScreenshotFormatJpeg,
		Quality: lazyjson.Int(90),
		Clip: &proto.PageViewport{
			X:      0,
			Y:      0,
			Width:  300,
			Height: 200,
			Scale:  1,
		},
		FromSurface: true,
	})
	_ = utils.OutputFile(outFile("my.jpg"), img)

	fmt.Println("done")

	// Output: done
}

func Example_page_scroll_screenshot() {
	browser := wand.New().MustConnect()
	defer browser.MustClose()

	// capture entire browser viewport, returning jpg with quality=90
	img, err := browser.MustPage(fixtureURL("tall.html")).MustWaitStable().ScrollScreenshot(&wand.ScrollScreenshotOptions{
		Format:  proto.PageCaptureScreenshotFormatJpeg,
		Quality: lazyjson.Int(90),
	})
	if err != nil {
		panic(err)
	}

	_ = utils.OutputFile(outFile("scroll.jpg"), img)

	fmt.Println("done")

	// Output: done
}

func Example_page_pdf() {
	browser := wand.New().MustConnect()
	defer browser.MustClose()

	page := browser.MustPage(fixtureURL("tall.html")).MustWaitLoad()

	// simple version
	page.MustPDF(outFile("my.pdf"))

	// customized version
	pdf, _ := page.PDF(&proto.PagePrintToPDF{
		PaperWidth:  lazyjson.Num(8.5),
		PaperHeight: lazyjson.Num(11),
		PageRanges:  "1-3",
	})
	_ = utils.OutputFile(outFile("custom.pdf"), pdf)

	fmt.Println("done")

	// Output: done
}

// Show how to handle multiple results of an action.
// Such as when you login a page, the result can be success or wrong password.
func Example_race_selectors() {
	// The login fixture takes any username, and "wand" as the password.
	const username = "gopher"
	const password = "wand"

	browser := wand.New().MustConnect()
	defer browser.MustClose()

	page := browser.MustPage(fixtureURL("login.html"))

	page.MustElement("#id_login").MustInput(username)
	page.MustElement("#id_password").MustInput(password).MustType(input.Enter)

	// It will keep retrying until one selector has found a match
	elm := page.Race().Element(".nav-user-icon-base").MustHandle(func(e *wand.Element) {
		// print the username after successful login
		fmt.Println(*e.MustAttribute("title"))
	}).Element("[data-cy=sign-in-error]").MustDo()

	if elm.MustMatches("[data-cy=sign-in-error]") {
		// when wrong username or password
		panic(elm.MustText())
	}

	// Output: gopher
}

// wand uses mouse cursor to simulate clicks, so if a button is moving because of animation, the click may not work as expected.
// We usually use WaitStable to make sure the target isn't changing anymore.
func Example_wait_for_animation() {
	browser := wand.New().MustConnect()
	defer browser.MustClose()

	page := browser.MustPage(fixtureURL("modal.html"))

	page.MustWaitLoad().MustElement("[data-target='#dialog']").MustClick()

	saveBtn := page.MustElementR("#dialog button", "Close")

	// Here, WaitStable will wait until the button's position and size become stable.
	saveBtn.MustWaitStable().MustClick().MustWaitInvisible()

	fmt.Println("done")

	// Output: done
}

// When you want to wait for an ajax request to complete, this example will be useful.
func Example_wait_for_request() {
	browser := wand.New().MustConnect()
	defer browser.MustClose()

	page := browser.MustPage(fixtureURL("suggestions.html")).MustWaitLoad()

	// Start to analyze request events
	wait := page.MustWaitRequestIdle()

	// This will trigger the search ajax request
	page.MustElement("#search-input").MustClick().MustInput("lisp")

	// Wait until there's no active requests
	wait()

	// We want to make sure that after waiting, there are some autocomplete
	// suggestions available.
	fmt.Println(len(page.MustElements(".suggestion-link")) > 0)

	// Output: true
}

// Shows how to change the retry/polling options that is used to query elements.
// This is useful when you want to customize the element query retry logic.
func Example_customize_retry_strategy() {
	browser := wand.New().MustConnect()
	defer browser.MustClose()

	page := browser.MustPage(fixtureURL("search.html"))

	// sleep for 0.5 seconds before every retry
	sleeper := func() utils.Sleeper {
		return func(context.Context) error {
			time.Sleep(time.Second / 2)
			return nil
		}
	}
	el, _ := page.Sleeper(sleeper).Element("input")
	fmt.Println(el.MustProperty("name"))

	// If sleeper is nil page.ElementE will query without retrying.
	// If nothing found it will return an error.
	el, err := page.Sleeper(wand.NotFoundSleeper).Element("input")
	if errors.Is(err, &wand.ElementNotFoundError{}) {
		fmt.Println("element not found")
	} else if err != nil {
		panic(err)
	}

	fmt.Println(el.MustProperty("name"))

	// Output:
	// q
	// q
}

// Shows how we can further customize the browser with the launcher library.
// Usually you use launcher lib to set the browser's command line flags (switches).
// Doc for flags: https://peter.sh/experiments/chromium-command-line-switches
//
// This example has no output comment, so it is compiled but never run: it goes
// through a proxy of your own, which this repository does not ship.
func Example_customize_browser_launch() {
	// The address of a proxy you run yourself, such as the one the CLI tool
	// "mitmproxy --proxyauth user:pass" listens on. An example of this
	// repository names no port of its own, so fill this in before you run it.
	var proxyAddress string

	url := launcher.New().
		Proxy(proxyAddress).         // set flag "--proxy-server=<proxyAddress>"
		Delete("use-mock-keychain"). // delete flag "--use-mock-keychain"
		MustLaunch()

	browser := wand.New().ControlURL(url).MustConnect()
	defer browser.MustClose()

	// So that we don't have to self issue certs for MITM
	browser.MustIgnoreCertErrors(true)

	// Adding authentication to the proxy, for the next auth request.
	go browser.MustHandleAuth("user", "pass")()

	fmt.Println(browser.MustPage("https://mdn.dev/").MustElement("title").MustText())
}

// When wand doesn't have a feature that you need. You can easily call the cdp to achieve it.
// List of cdp API: https://github.com/headlesslab/wand/tree/main/lib/proto
func Example_direct_cdp() {
	browser := wand.New().MustConnect()
	defer browser.MustClose()

	page := browser.MustPage()

	// wand doesn't have a method to enable AD blocking,
	// but you can call cdp interface directly to achieve it.

	// The two code blocks below are equal to enable AD blocking

	{
		_ = proto.PageSetAdBlockingEnabled{
			Enabled: true,
		}.Call(page)
	}

	{
		// Interact with the cdp JSON API directly
		_, _ = page.Call(context.TODO(), "", "Page.setAdBlockingEnabled", map[string]bool{
			"enabled": true,
		})
	}

	fmt.Println("done")

	// Output: done
}

// Shows how to listen for events.
func Example_handle_events() {
	browser := wand.New().MustConnect()
	defer browser.MustClose()

	page := browser.MustPage()

	done := make(chan struct{})

	// Listen for all events of console output.
	go page.EachEvent(func(e *proto.RuntimeConsoleAPICalled) {
		if e.Type == proto.RuntimeConsoleAPICalledTypeLog {
			fmt.Println(page.MustObjectsToJSON(e.Args))
			close(done)
		}
	})()

	wait := page.WaitEvent(&proto.PageLoadEventFired{})
	page.MustNavigate(fixtureURL("search.html"))
	wait()

	// EachEvent allows us to achieve the same functionality as above.
	if false {
		// Subscribe events before they happen, run the "wait()" to start consuming
		// the events. We can return an optional stop signal to unsubscribe events.
		wait := page.EachEvent(func(_ *proto.PageLoadEventFired) (stop bool) {
			return true
		})
		page.MustNavigate(fixtureURL("search.html"))
		wait()
	}

	// Or the for-loop style to handle events to do the same thing above.
	if false {
		page.MustNavigate(fixtureURL("search.html"))

		for msg := range page.Event() {
			e := proto.PageLoadEventFired{}
			if msg.Load(&e) {
				break
			}
		}
	}

	page.MustEval(`() => console.log("hello", "world")`)

	<-done

	// Output:
	// [hello world]
}

func Example_download_file() {
	browser := wand.New().MustConnect()
	defer browser.MustClose()

	page := browser.MustPage(fixtureURL("download.html"))

	wait := browser.MustWaitDownload()

	page.MustElementR("a", "DOWNLOAD THE SAMPLE FILE").MustClick()

	data := wait()

	_ = utils.OutputFile(outFile("note.txt"), data)

	fmt.Println(string(data))

	// Output: A sample file for the download example.
}

// Shows how to intercept requests and modify
// both the request and the response.
// The entire process of hijacking one request:
//
//	browser --req-> wand ---> server ---> wand --res-> browser
//
// The --req-> and --res-> are the parts that can be modified.
func Example_hijack_requests() {
	browser := wand.New().MustConnect()
	defer browser.MustClose()

	router := browser.HijackRequests()
	defer router.MustStop()

	router.MustAdd("*.js", func(ctx *wand.Hijack) {
		// Here we update the request's header. wand gives functionality to
		// change or update all parts of the request. Refer to the documentation
		// for more information.
		ctx.Request.Req().Header.Set("My-Header", "test")

		// LoadResponse runs the default request to the destination of the request.
		// Not calling this will require you to mock the entire response.
		// This can be done with the SetXxx (Status, Header, Body) functions on the
		// ctx.Response struct.
		_ = ctx.LoadResponse(http.DefaultClient, true)

		// Here we append some code to every js file.
		// The code will update the document title to "hi"
		ctx.Response.SetBody(ctx.Response.Body() + "\n document.title = 'hi' ")
	})

	go router.Run()

	browser.MustPage(fixtureURL("hijack.html")).MustWait(`() => document.title === 'hi'`)

	fmt.Println("done")

	// Output: done
}

// Shows how to share a remote object reference between two Eval.
func Example_eval_reuse_remote_object() {
	browser := wand.New().MustConnect()
	defer browser.MustClose()

	page := browser.MustPage()

	// ByObject keeps the function in the browser and hands back a reference to
	// it, instead of copying its return value into Go.
	fn := page.MustEvaluate(wand.Eval(`() => n => n * 2`).ByObject())

	// Pass the reference to another Eval, which calls it in the browser.
	res := page.MustEval(`(f, n) => f(n)`, fn, 21)

	fmt.Println(res.Int())

	// Output: 42
}

// Shows how to update the state of the current page.
// In this example we enable the network domain.
func Example_states() {
	browser := wand.New().MustConnect()
	defer browser.MustClose()

	page := browser.MustPage()

	// LoadState detects whether the network domain is enabled or not.
	fmt.Println(page.LoadState(&proto.NetworkEnable{}))

	_ = proto.NetworkEnable{}.Call(page)

	// Check if the network domain is successfully enabled.
	fmt.Println(page.LoadState(&proto.NetworkEnable{}))

	// Output:
	// false
	// true
}

// We can use [wand.PagePool] to concurrently control and reuse pages.
func ExamplePage_pool() {
	browser := wand.New().MustConnect()
	defer browser.MustClose()

	// We create a pool that will hold at most 3 pages which means the max concurrency is 3
	pool := wand.NewPagePool(3)

	// Create a page if needed
	create := func() *wand.Page {
		// We use MustIncognito to isolate pages with each other
		return browser.MustIncognito().MustPage()
	}

	yourJob := func() {
		page := pool.MustGet(create)

		// Put the instance back to the pool after we're done,
		// so the instance can be reused by other goroutines.
		defer pool.Put(page)

		page.MustNavigate(fixtureURL("search.html")).MustWaitLoad()
		fmt.Println(page.MustInfo().Title)
	}

	// Run jobs concurrently
	wg := sync.WaitGroup{}
	for range "...." {
		wg.Add(1)
		go func() {
			defer wg.Done()
			yourJob()
		}()
	}
	wg.Wait()

	// cleanup pool
	pool.Cleanup(func(p *wand.Page) { p.MustClose() })

	// Output:
	// wand search
	// wand search
	// wand search
	// wand search
}

// We can use [wand.BrowserPool] to concurrently control and reuse browsers.
func ExampleBrowser_pool() {
	// Create a new browser pool with a limit of 3
	pool := wand.NewBrowserPool(3)

	// Create a function that returns a new browser instance
	create := func() *wand.Browser {
		browser := wand.New().MustConnect()
		return browser
	}

	// Use the browser instances in separate goroutines
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// Get a browser instance from the pool
			browser := pool.MustGet(create)

			// Put the instance back to the pool after we're done,
			// so the instance can be reused by other goroutines.
			defer pool.Put(browser)

			// Use the browser instance
			page := browser.MustPage(fixtureURL("search.html")).MustWaitLoad()
			fmt.Println(page.MustInfo().Title)
		}()
	}

	// Wait for all the goroutines to finish
	wg.Wait()

	// Cleanup the pool by closing all the browser instances
	pool.Cleanup(func(p *wand.Browser) {
		p.MustClose()
	})

	// Output:
	// wand search
	// wand search
	// wand search
}

// This example has no output comment, so it is compiled but never run: an
// extension needs a browser with a window on screen, and a run has no display.
// Reason: https://bugs.chromium.org/p/chromium/issues/detail?id=706008#c5
// You can use XVFB to get rid of it: https://github.com/headlesslab/wand/blob/main/lib/examples/launch-managed/main.go
func Example_load_extension() {
	extPath, _ := filepath.Abs("fixtures/chrome-extension")

	u := launcher.New().
		// Must use abs path for an extension
		Set("load-extension", extPath).
		Headless(false).
		MustLaunch()

	browser := wand.New().ControlURL(u).MustConnect()
	defer browser.MustClose()

	page := browser.MustPage(fixtureURL("search.html"))

	page.MustWait(`() => document.title === 'test-extension'`)

	fmt.Println("ok")
}

func Example_log_cdp_traffic() {
	l := launcher.New()
	defer l.Cleanup()

	client := cdp.New().
		// Here we can customize how to log the requests, responses, and events transferred between wand and the browser.
		// This one reports the navigations only, so that the example's output is the same on every run.
		Logger(utils.Log(func(args ...interface{}) {
			switch v := args[0].(type) {
			case *cdp.Request:
				if v.Method == "Page.navigate" {
					fmt.Printf("request: %s\n", v.Method)
				}
			}
		})).
		Start(cdp.MustConnectWS(l.MustLaunch()))

	browser := wand.New().Client(client).MustConnect()
	defer browser.MustClose()

	browser.MustPage(fixtureURL("search.html"))

	// Output: request: Page.navigate
}
