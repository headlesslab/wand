package launcher

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"github.com/headlesslab/lazyjson"
	"github.com/headlesslab/wand/lib/utils"
)

var _ io.Writer = &URLParser{}

// URLParser to get control url from stderr.
type URLParser struct {
	URL    chan string
	Buffer string // buffer for the browser stdout

	lock *sync.Mutex
	ctx  context.Context
	done bool
}

// NewURLParser instance.
func NewURLParser() *URLParser {
	return &URLParser{
		URL:  make(chan string),
		lock: &sync.Mutex{},
		ctx:  context.Background(),
	}
}

var regWS = regexp.MustCompile(`ws://.+/`)

// Context sets the context.
func (r *URLParser) Context(ctx context.Context) *URLParser {
	r.ctx = ctx
	return r
}

// Write interface.
func (r *URLParser) Write(p []byte) (n int, err error) {
	r.lock.Lock()
	defer r.lock.Unlock()

	if !r.done {
		r.Buffer += string(p)

		str := regWS.FindString(r.Buffer)
		if str != "" {
			u, err := url.Parse(strings.TrimSpace(str))
			utils.E(err)

			select {
			case <-r.ctx.Done():
			case r.URL <- "http://" + u.Host:
			}

			r.done = true
			r.Buffer = ""
		}
	}

	return len(p), nil
}

// Err returns the common error parsed from stdout and stderr.
func (r *URLParser) Err() error {
	r.lock.Lock()
	defer r.lock.Unlock()

	msg := "[launcher] Failed to get the debug url: "

	if strings.Contains(r.Buffer, "error while loading shared libraries") {
		msg = "[launcher] Failed to launch the browser, the doc might help https://go-rod.github.io/#/compatibility?id=os: "
	}

	return errors.New(msg + r.Buffer)
}

// MustResolveURL is similar to ResolveURL.
func MustResolveURL(u string) string {
	u, err := ResolveURL(u)
	utils.E(err)
	return u
}

var (
	regPort     = regexp.MustCompile(`^\:?(\d+)$`)
	regProtocol = regexp.MustCompile(`^\w+://`)
)

// ResolveURL by requesting the u, it will try best to normalize the u.
// The format of u can be "9222", ":9222", "host:9222", "ws://host:9222", "wss://host:9222",
// "https://host:9222" "http://host:9222". The return string will look like:
// "ws://host:9222/devtools/browser/4371405f-84df-4ad6-9e0f-eab81f7521cc"
//
// An answer that is not a browser's is an error: a status other than 200,
// as a proxy in the way of the port gives, or a body without a
// webSocketDebuggerUrl, as a web server that happens to listen on the port
// gives. The Snapshot took both for a browser and returned a URL with "<nil>"
// for a path (rod #1176).
func ResolveURL(u string) (string, error) {
	if u == "" {
		u = "9222"
	}

	u = strings.TrimSpace(u)
	u = regPort.ReplaceAllString(u, "127.0.0.1:$1")

	if !regProtocol.MatchString(u) {
		u = "http://" + u
	}

	parsed, err := url.Parse(u)
	if err != nil {
		return "", err
	}

	parsed = toHTTP(*parsed)
	parsed.Path = "/json/version"

	res, err := http.Get(parsed.String()) //nolint: noctx
	if err != nil {
		return "", err
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s answered %s, not a browser", parsed, res.Status)
	}

	data, err := io.ReadAll(res.Body)
	if err != nil {
		return "", fmt.Errorf("reading the answer of %s: %w", parsed, err)
	}

	// The value, not the key: a null or a number is as much not a browser's
	// as a missing field.
	wsURL, _ := lazyjson.New(data).Get("webSocketDebuggerUrl").Val().(string)
	if wsURL == "" {
		return "", fmt.Errorf("%s answered without a webSocketDebuggerUrl, not a browser", parsed)
	}

	parsedWS, err := url.Parse(wsURL)
	if err != nil {
		return "", fmt.Errorf("the webSocketDebuggerUrl %s answered: %w", parsed, err)
	}

	parsedWS.Host = parsed.Host

	return parsedWS.String(), nil
}
