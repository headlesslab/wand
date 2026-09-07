package cdp_test

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/headlesslab/lazyjson"
	"github.com/headlesslab/wand/lib/cdp"
	"github.com/headlesslab/wand/lib/launcher"
	"github.com/headlesslab/wand/lib/utils"
	"github.com/ysmood/got"
)

func TestWebSocketLargePayload(t *testing.T) {
	g := setup(t)

	ctx := g.Context()
	client, id := newPage(ctx, g)

	const size = 2 * 1024 * 1024

	res, err := client.Call(ctx, id, "Runtime.evaluate", map[string]interface{}{
		"expression":    fmt.Sprintf(`"%s"`, strings.Repeat("a", size)),
		"returnByValue": true,
	})
	g.E(err)
	g.Gt(len(res), size) // 2MB
}

func ConcurrentCall(t *testing.T) {
	t.Helper()

	g := setup(t)

	ctx := g.Context()
	client, id := newPage(ctx, g)

	wg := sync.WaitGroup{}
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			res, err := client.Call(ctx, id, "Runtime.evaluate", map[string]interface{}{
				"expression": `10`,
			})
			g.Nil(err)
			g.Eq(string(res), "{\"result\":{\"type\":\"number\",\"value\":10,\"description\":\"10\"}}")
			wg.Done()
		}()
	}
	wg.Wait()
}

func TestWebSocketHeader(t *testing.T) {
	g := setup(t)

	s := g.Serve()

	wait := make(chan struct{})
	s.Mux.HandleFunc("/a", func(_ http.ResponseWriter, r *http.Request) {
		g.Eq(r.Header.Get("Test"), "header")
		g.Eq(r.Host, "test.com")
		g.Eq(r.URL.Query().Get("q"), "ok")
		close(wait)
	})

	// A map literal spells the names as it likes; Host is matched
	// case-insensitively into the request's Host line, and the key case is
	// TestWebSocketKey's.
	ws := cdp.WebSocket{}
	err := ws.Connect(g.Context(), s.URL("/a?q=ok"), http.Header{
		"host":              {"test.com"},
		"Test":              {"header"},
		"Sec-WebSocket-Key": {"key"},
	})
	<-wait

	g.Eq(err.Error(), "websocket bad handshake: 200 OK. ")
}

// The Confirmed fix for rod #1092's handshake half, harvested from rod #1228:
// a WebSocket server that validates the handshake (Browserless, Lightpanda,
// any gorilla/websocket proxy) refused the literal key the Snapshot sent.
func TestWebSocketKey(t *testing.T) {
	g := setup(t)

	keys := make(chan string, 3)
	u := wsServer(g, func(key string, conn net.Conn) {
		keys <- key
		_ = conn.Close()
	})

	// The server accepts nothing but base64 of 16 bytes, so two handshakes
	// that succeed with different keys prove a real nonce per handshake.
	ws := &cdp.WebSocket{}
	g.E(ws.Connect(g.Context(), u, nil))
	g.E(ws.Close())
	ws = &cdp.WebSocket{}
	g.E(ws.Connect(g.Context(), u, nil))
	g.E(ws.Close())
	first, second := <-keys, <-keys
	g.Neq(first, second)

	// A key of the caller's own, spelled the way http.Header spells it
	// (Sec-Websocket-Key), is the one sent, and the only one sent.
	const sample = "dGhlIHNhbXBsZSBub25jZQ==" // RFC 6455's own example
	header := http.Header{}
	header.Set("Sec-WebSocket-Key", sample)
	ws = &cdp.WebSocket{}
	g.E(ws.Connect(g.Context(), u, header))
	g.E(ws.Close())
	g.Eq(<-keys, sample)
}

// wsServer serves one WebSocket endpoint the way gorilla/websocket and
// Browserless do: the handshake is validated per RFC 6455 section 4.2.1
// (exactly one Sec-WebSocket-Key, base64 of 16 bytes) and refused with 400
// otherwise. An accepted connection is handed to serve, with the key it came
// with, on the handler's goroutine; serve closes it.
func wsServer(g got.G, serve func(key string, conn net.Conn)) string {
	g.Helper()

	s := g.Serve()
	s.Mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		keys := r.Header["Sec-Websocket-Key"] // as Go's server spells it
		if len(keys) != 1 {
			http.Error(w, fmt.Sprintf("%d Sec-WebSocket-Key headers", len(keys)), http.StatusBadRequest)
			return
		}
		nonce, err := base64.StdEncoding.DecodeString(keys[0])
		if err != nil || len(nonce) != 16 {
			http.Error(w, "Sec-WebSocket-Key must be base64 of 16 bytes", http.StatusBadRequest)
			return
		}

		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		accept := sha1.Sum([]byte(keys[0] + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
		_, err = fmt.Fprintf(conn, "HTTP/1.1 101 Switching Protocols\r\n"+
			"Upgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n",
			base64.StdEncoding.EncodeToString(accept[:]))
		g.Nil(err)

		serve(keys[0], conn)
	})

	return "ws" + strings.TrimPrefix(s.URL("/"), "http")
}

func newPage(ctx context.Context, g got.G) (*cdp.Client, string) {
	l := launcher.New()
	client := cdp.New().Start(cdp.MustConnectWS(launch(g, l)))
	g.Cleanup(l.Kill) // nobody closes this browser; killed at once, not after the cleanup bound

	go func() {
		for range client.Event() {
			utils.Noop()
		}
	}()

	file, err := filepath.Abs(filepath.FromSlash("fixtures/basic.html"))
	g.E(err)

	res, err := client.Call(ctx, "", "Target.createTarget", map[string]interface{}{
		"url": "file://" + file,
	})
	g.E(err)

	targetID := lazyjson.New(res).Get("targetId").String()

	res, err = client.Call(ctx, "", "Target.attachToTarget", map[string]interface{}{
		"targetId": targetID,
		"flatten":  true,
	})
	g.E(err)

	sessionID := lazyjson.New(res).Get("sessionId").String()

	return client, sessionID
}

func TestDuplicatedConnectErr(t *testing.T) {
	g := setup(t)

	l := launcher.New()
	u := launch(g, l)
	g.Cleanup(l.Kill) // nobody closes this browser; killed at once, not after the cleanup bound

	ws := &cdp.WebSocket{}
	g.E(ws.Connect(g.Context(), u, nil))

	g.Panic(func() {
		_ = ws.Connect(g.Context(), u, nil)
	})
}
