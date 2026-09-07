package cdp_test

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/headlesslab/lazyjson"
	"github.com/headlesslab/wand/lib/cdp"
	"github.com/headlesslab/wand/lib/launcher"
	"github.com/headlesslab/wand/lib/proto"
	"github.com/headlesslab/wand/lib/utils"
)

func ExampleClient() {
	ctx := context.Background()

	// launch a browser
	l := launcher.New()
	defer l.Cleanup()

	// create a controller
	client := cdp.New().Start(cdp.MustConnectWS(l.MustLaunch()))

	go func() {
		for range client.Event() {
			// you must consume the events
			utils.Noop()
		}
	}()

	// A page of this package, so that the example runs with no network.
	page, err := filepath.Abs(filepath.FromSlash("fixtures/basic.html"))
	utils.E(err)

	// Such as call this endpoint on the api doc:
	// https://chromedevtools.github.io/devtools-protocol/tot/Page#method-navigate
	// This will create a new tab and navigate to the page
	res, err := client.Call(ctx, "", "Target.createTarget", map[string]string{
		"url": "file://" + page,
	})
	utils.E(err)

	fmt.Println(len(lazyjson.New(res).Get("targetId").Str()))

	// close browser by using the proto lib to encode json
	_ = proto.BrowserClose{}.Call(client)

	// Output: 32
}

func Example_customize_cdp_log() {
	l := launcher.New()
	defer l.Cleanup()

	ws := cdp.MustConnectWS(l.MustLaunch())

	client := cdp.New().
		// Here we can customize how to log the requests, responses and events.
		// This one reports the browser shutdown only, so that the example's
		// output is the same on every run.
		Logger(utils.Log(func(args ...interface{}) {
			switch v := args[0].(type) {
			case *cdp.Request:
				if v.Method == "Browser.close" {
					fmt.Printf("request: %s\n", v.Method)
				}
			}
		})).
		Start(ws)

	_ = proto.BrowserClose{}.Call(client)

	// Output: request: Browser.close
}
