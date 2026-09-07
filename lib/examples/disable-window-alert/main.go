// Package main ...
package main

import (
	"fmt"
	"net"
	"net/http"

	"github.com/headlesslab/wand"
	"github.com/headlesslab/wand/lib/utils"
)

func main() {
	url := serve()

	browser := wand.New().MustConnect()
	defer browser.MustClose()

	// Creating a Page Object
	page := browser.MustPage()

	// Evaluates given script in every frame upon creation
	// Disable all alerts by making window.alert no-op.
	page.MustEvalOnNewDocument(`window.alert = () => {}`)

	// Navigate to the website you want to visit
	page.MustNavigate(url)

	fmt.Println(page.MustElement("script").MustText())
}

const testPage = `<html><script>alert("message")</script></html>`

// serve mocks a server on a port the OS picks, so that the example takes no
// port of the machine for itself.
func serve() string {
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	utils.E(err)

	go func() {
		_ = http.Serve(l, http.HandlerFunc(func(res http.ResponseWriter, _ *http.Request) {
			utils.E(fmt.Fprint(res, testPage))
		}))
	}()

	return "http://" + l.Addr().String()
}
