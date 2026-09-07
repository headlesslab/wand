// This is the setup file for this test suite.

package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/headlesslab/wand"
	"github.com/ysmood/got"
)

// test context.
type G struct {
	got.G

	browser *wand.Browser
}

var (
	browser *wand.Browser

	// app serves the calculator under ./app on a port the OS picks, so the
	// suite needs no network and no port of the machine reserved for it.
	app *httptest.Server

	// appURL is where the app under test is served for this run.
	appURL string
)

// TestMain launches one browser and one server for the whole suite, and
// closes both whatever the tests do.
func TestMain(m *testing.M) {
	browser = wand.New().MustConnect()
	app = httptest.NewServer(http.FileServer(http.Dir("app")))
	appURL = app.URL + "/"

	code := m.Run()

	app.Close()
	browser.MustClose()

	os.Exit(code)
}

// setup for tests.
func setup(t *testing.T) G {
	t.Parallel() // run each test concurrently

	return G{got.New(t), browser}
}

// a helper function to create an incognito page.
func (g G) page(url string) *wand.Page {
	page := g.browser.MustIncognito().MustPage(url)
	g.Cleanup(page.MustClose)
	return page
}
