package cdp_test

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	header := http.Header{}
	header.Set("Sec-WebSocket-Key", sampleKey())
	ws = &cdp.WebSocket{}
	g.E(ws.Connect(g.Context(), u, header))
	g.E(ws.Close())
	g.Eq(<-keys, sampleKey())
}

// sampleKey is RFC 6455's own example of a Sec-WebSocket-Key. The framing
// tests hand it to the client, so they stand apart from the handshake fix
// TestWebSocketKey covers.
func sampleKey() string {
	return base64.StdEncoding.EncodeToString([]byte("the sample nonce"))
}

// The Confirmed fix for rod #1092's framing half, harvested from rod #1187:
// a server that checks liveness with pings (Lightpanda; any gorilla/websocket
// proxy) had its control frames handed to the CDP JSON decoder, which
// panicked on them (rod #1186).
func TestWebSocketControlFrames(t *testing.T) {
	g := setup(t)

	pong := make(chan frame, 1)
	u := wsServer(g, func(_ string, conn net.Conn) {
		defer func() { _ = conn.Close() }()

		req, err := clientFrame(conn)
		if err != nil {
			pong <- frame{err: err}
			return
		}

		// A ping and an unsolicited pong (RFC 6455 section 5.5.3) ahead of
		// the response, as a server checking liveness sends them.
		_, _ = conn.Write(serverFrame(opPing, []byte("keepalive")))
		_, _ = conn.Write(serverFrame(opPong, []byte("unsolicited")))
		_, _ = conn.Write(serverFrame(opText, []byte(fmt.Sprintf(`{"id":%d,"result":{"ok":true}}`, req.id()))))

		// The ping must be answered with a pong carrying its payload
		// (section 5.5.2), or a server that pings drops the client.
		f, err := clientFrame(conn)
		f.err = err
		pong <- f
	})

	ws := &cdp.WebSocket{}
	g.E(ws.Connect(g.Context(), u, http.Header{"Sec-WebSocket-Key": {sampleKey()}}))
	client := cdp.New().Start(ws)

	res, err := client.Call(g.Context(), "", "Browser.getVersion", nil)
	g.E(err)
	g.Eq(string(res), `{"ok":true}`)

	f := <-pong
	g.E(f.err)
	g.Eq(f.op, opPong)
	g.Eq(string(f.payload), "keepalive")
}

// A close frame ends the connection with the peer's own code and reason (what
// Browserless says when a session runs past its limit) rather than with the
// EOF, or the hang, that followed the Snapshot's silence.
func TestWebSocketClose(t *testing.T) {
	g := setup(t)

	// closing serves an endpoint that answers the first request with the
	// close frame given, then reads the close the client sends in return
	// (RFC 6455 section 5.5.1) before the TCP connection goes.
	closing := func(payload []byte) (string, chan frame) {
		reply := make(chan frame, 1)
		u := wsServer(g, func(_ string, conn net.Conn) {
			defer func() { _ = conn.Close() }()

			if _, err := clientFrame(conn); err != nil {
				reply <- frame{err: err}
				return
			}
			_, _ = conn.Write(serverFrame(opClose, payload))
			f, err := clientFrame(conn)
			f.err = err
			reply <- f
		})
		return u, reply
	}
	call := func(u string) error {
		ws := &cdp.WebSocket{}
		g.E(ws.Connect(g.Context(), u, http.Header{"Sec-WebSocket-Key": {sampleKey()}}))
		_, err := cdp.New().Start(ws).Call(g.Context(), "", "Browser.getVersion", nil)
		return err
	}

	// 1001 going away: the caller gets the code and the reason, the server
	// gets the code back.
	u, reply := closing(append([]byte{0x03, 0xE9}, "going away"...))
	err := call(u)
	var closeErr *cdp.CloseError
	g.True(errors.As(err, &closeErr))
	g.Eq(closeErr.Code, 1001)
	g.Eq(closeErr.Reason, "going away")
	g.Eq(err.Error(), "websocket closed by the peer: 1001 going away")
	f := <-reply
	g.E(f.err)
	g.Eq(f.op, opClose)
	g.Eq(f.payload, []byte{0x03, 0xE9})

	// No status code: 1005 for the caller (section 7.4.1), none sent back.
	u, reply = closing(nil)
	err = call(u)
	g.True(errors.As(err, &closeErr))
	g.Eq(closeErr.Code, 1005)
	g.Eq(closeErr.Reason, "")
	g.Eq(err.Error(), "websocket closed by the peer: 1005")
	f = <-reply
	g.E(f.err)
	g.Eq(f.op, opClose)
	g.Eq(len(f.payload), 0)
}

// Opcodes of RFC 6455 section 5.2.
const (
	opText  = 0x1
	opClose = 0x8
	opPing  = 0x9
	opPong  = 0xA
)

// frame is one frame as a server reads it from the client.
type frame struct {
	op      byte
	payload []byte
	err     error
}

// id of the CDP request the frame carries.
func (f frame) id() int {
	var req struct {
		ID int `json:"id"`
	}
	_ = json.Unmarshal(f.payload, &req)
	return req.ID
}

// serverFrame is one unmasked frame with FIN set, as a server sends them;
// the payload fits the 7-bit length, as every frame these tests send does.
func serverFrame(op byte, payload []byte) []byte {
	return append([]byte{0x80 | op, byte(len(payload))}, payload...)
}

// clientFrame reads one frame the way a server does: FIN set, masked as a
// client must, any of the three length forms (RFC 6455 section 5.2).
func clientFrame(r io.Reader) (frame, error) {
	var head [2]byte
	if _, err := io.ReadFull(r, head[:]); err != nil {
		return frame{}, err
	}
	if head[0]&0x80 == 0 {
		return frame{}, errors.New("fragmented frame")
	}
	if head[1]&0x80 == 0 {
		return frame{}, errors.New("unmasked client frame")
	}

	size := uint64(head[1] & 0x7f)
	switch size {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			return frame{}, err
		}
		size = uint64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			return frame{}, err
		}
		size = binary.BigEndian.Uint64(ext[:])
	}

	var mask [4]byte
	if _, err := io.ReadFull(r, mask[:]); err != nil {
		return frame{}, err
	}
	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return frame{}, err
	}
	for i := range payload {
		payload[i] ^= mask[i%4]
	}

	return frame{op: head[0] & 0x0f, payload: payload}, nil
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
