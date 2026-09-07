package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/headlesslab/wand/lib/launcher/pins"
	"github.com/ysmood/got"
)

var setup = got.Setup(nil)

func TestOldestMilestone(t *testing.T) {
	g := setup(t)

	milestone, err := oldest("153.0.8010.27")
	g.E(err)
	g.Eq(milestone, 150)

	// The pins are what the command runs on, so the arithmetic is proven on
	// them too rather than only on a version written here. The milestone is
	// read back with Sscanf, which is not the code under test.
	var target int
	_, err = fmt.Sscanf(pins.ChromeVersion, "%d.", &target)
	g.E(err)

	milestone, err = oldest(pins.ChromeVersion)
	g.E(err)
	g.Eq(milestone, target-milestones)

	// A Target Chrome with no milestone in it, and one whose milestone the
	// window reaches to zero or below: neither names a browser to run on.
	for _, version := range []string{"", "153", "x.0.8010.27", "3.0.0.0"} {
		_, err := oldest(version)
		g.Desc("%q", version).Err(err)
	}
}

func TestLastKnownGood(t *testing.T) {
	g := setup(t)

	site := serve(g, http.StatusOK, `{"milestones":{
		"149":{"milestone":"149","version":"149.0.7632.83","revision":"1523467"},
		"150":{"milestone":"150","version":"150.0.7767.9","revision":"1549132"}
	}}`)

	version, err := lastKnownGood(context.Background(), http.DefaultClient, site, 150)
	g.E(err)
	g.Eq(version, "150.0.7767.9")
}

func TestLastKnownGoodRefusals(t *testing.T) {
	g := setup(t)

	ask := func(site string, milestone int) error {
		_, err := lastKnownGood(context.Background(), http.DefaultClient, site, milestone)

		return err
	}

	// A milestone the list does not carry: one old enough to have dropped
	// off, which is what a Support window reaching past Chrome for Testing's
	// own history looks like.
	err := ask(serve(g, http.StatusOK, `{"milestones":{"150":{"version":"150.0.7767.9"}}}`), 149)
	g.Err(err)
	g.Has(err.Error(), "no build of milestone 149 is published")

	// A version of another milestone under the key asked for: taking it would
	// run the suite on the wrong browser and read as a green Support window.
	err = ask(serve(g, http.StatusOK, `{"milestones":{"149":{"version":"151.0.7900.1"}}}`), 149)
	g.Err(err)
	g.Has(err.Error(), "which is not a version of it")

	// Chrome for Testing answering anything but 200, and answering something
	// that is not the JSON it documents.
	err = ask(serve(g, http.StatusNotFound, "no such file"), 149)
	g.Err(err)
	g.Has(err.Error(), "404")

	g.Err(ask(serve(g, http.StatusOK, "<html>a proxy sign-in page</html>"), 149))

	// A URL no request can be made of, and a site nothing answers on.
	g.Err(ask("://milestones", 149))

	gone := httptest.NewServer(http.NotFoundHandler())
	url := gone.URL
	gone.Close()
	g.Err(ask(url, 149))
}

// serve answers every request with one status and one body, for as long as
// the test runs.
func serve(g got.G, status int, body string) string {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	g.Cleanup(s.Close)

	return s.URL
}
