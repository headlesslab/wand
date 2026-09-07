package cdp

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/headlesslab/wand/lib/utils"
)

var _ WebSocketable = &WebSocket{}

// WebSocket client for chromium. It only implements a subset of WebSocket protocol:
// text frames in one piece each way, masked with a fixed key, the control frames
// handled inside Read, no extensions, and the peer's framing taken on trust.
// Both the Read and Write are thread-safe.
// Limitation: https://bugs.chromium.org/p/chromium/issues/detail?id=1069431
// Ref: https://tools.ietf.org/html/rfc6455
type WebSocket struct {
	// Dialer is usually used for proxy
	Dialer Dialer

	readLock  sync.Mutex
	writeLock sync.Mutex
	conn      net.Conn
	r         *bufio.Reader
}

// Connect to browser.
func (ws *WebSocket) Connect(ctx context.Context, wsURL string, header http.Header) error {
	if ws.conn != nil {
		panic("duplicated connection: " + wsURL)
	}

	u, err := url.Parse(wsURL)
	if err != nil {
		return err
	}

	ws.initDialer(u)

	conn, err := ws.Dialer.DialContext(ctx, "tcp", u.Host)
	if err != nil {
		return err
	}

	ws.conn = conn
	ws.r = bufio.NewReader(conn)
	return ws.handshake(ctx, u, header)
}

// Close the underlying connection.
func (ws *WebSocket) Close() error {
	return ws.conn.Close()
}

func (ws *WebSocket) initDialer(u *url.URL) {
	if ws.Dialer != nil {
		return
	}

	if u.Scheme == "wss" {
		ws.Dialer = &tlsDialer{}
		if u.Port() == "" {
			u.Host += ":443"
		}
	} else {
		ws.Dialer = &net.Dialer{}
	}
}

// Opcodes of RFC 6455 section 5.2 that the client sends or acts on.
const (
	opText  = 0x1
	opClose = 0x8
	opPing  = 0x9
	opPong  = 0xA
)

// closeNoStatus is the code CloseError reports for a close frame without one
// (RFC 6455 section 7.4.1).
const closeNoStatus = 1005

// Send a message to browser.
// Because we use zero-copy design, it will modify the content of the msg.
// It won't allocate new memory.
func (ws *WebSocket) Send(msg []byte) error {
	err := ws.write(opText, msg)
	if err != nil {
		_ = ws.Close()
	}
	return err
}

// write sends one frame with FIN set, masked as a client's frames must be
// (RFC 6455 section 5.3; with the fixed key upstream chose, since the
// unpredictable one the section asks for guards proxies against a hostile
// page's payloads, not against a driver's own CDP messages), under the write
// lock, so frames from Send and the replies Read sends never interleave.
// The payload is masked in place.
func (ws *WebSocket) write(op byte, msg []byte) error {
	ws.writeLock.Lock()
	defer ws.writeLock.Unlock()

	header := [18]byte{0b1000_0000 | op, 0b1000_0000}
	mask := []byte{0, 1, 2, 3}

	size := len(msg)
	fieldLen := 0
	switch {
	case size <= 125:
		header[1] |= byte(size)
	case size < 65536:
		header[1] |= 126
		fieldLen = 2
	default:
		header[1] |= 127
		fieldLen = 8
	}

	var i int
	for i = 0; i < fieldLen; i++ {
		digit := (fieldLen - i - 1) * 8
		header[i+2] = byte((size >> digit) & 0xff)
	}

	copy(header[i+2:], mask)

	for i := range msg {
		msg[i] ^= mask[i%4]
	}

	data := make([]byte, i+6+len(msg))
	copy(data, header[:i+6])
	copy(data[i+6:], msg)

	_, err := ws.conn.Write(data)
	return err
}

// Read a message from browser. Control frames never come back as messages
// (rod #1187): a ping is answered with a pong carrying its payload, a pong
// is dropped, and a close frame is answered with a close frame and ends the
// connection with a CloseError. Chrome sends none of them, other CDP servers
// and proxies do.
func (ws *WebSocket) Read() ([]byte, error) {
	for {
		op, data, err := ws.read()
		if err == nil {
			switch op {
			case opPing:
				err = ws.write(opPong, data)
			case opPong:
				// Unsolicited pongs are allowed (RFC 6455 section 5.5.3).
			case opClose:
				err = ws.replyClose(data)
			default:
				return data, nil
			}
		}
		if err != nil {
			_ = ws.Close()
			return nil, err
		}
	}
}

// replyClose answers a close frame with one echoing its status code, as
// RFC 6455 section 5.5.1 has it, and reports the close. The reply is best
// effort: the peer's close is the fact to report, whether or not the socket
// still takes a write.
func (ws *WebSocket) replyClose(data []byte) error {
	e := &CloseError{Code: closeNoStatus}
	var code []byte
	if len(data) >= 2 {
		code = data[:2]
		e.Code = int(binary.BigEndian.Uint16(code))
		e.Reason = string(data[2:])
	}
	_ = ws.write(opClose, code)
	return e
}

// read one frame: its opcode and payload.
func (ws *WebSocket) read() (byte, []byte, error) {
	ws.readLock.Lock()
	defer ws.readLock.Unlock()

	b, err := ws.r.ReadByte()
	if err != nil {
		return 0, nil, err
	}
	op := b & 0x0f

	b, err = ws.r.ReadByte()
	if err != nil {
		return 0, nil, err
	}

	size := 0
	fieldLen := 0

	b &= 0x7f
	switch {
	case b <= 125:
		size = int(b)
	case b == 126:
		fieldLen = 2
	case b == 127:
		fieldLen = 8
	}

	for i := 0; i < fieldLen; i++ {
		b, err := ws.r.ReadByte()
		if err != nil {
			return 0, nil, err
		}

		size = size<<8 + int(b)
	}

	data := make([]byte, size)
	_, err = io.ReadFull(ws.r, data)
	return op, data, err
}

// CloseError is the error Read returns once the peer has sent a close frame
// (RFC 6455 section 5.5.1): the connection is over, and Code and Reason are
// what the peer gave for it. Code is 1005 when the frame carried none.
type CloseError struct {
	Code   int
	Reason string
}

func (e *CloseError) Error() string {
	msg := fmt.Sprintf("websocket closed by the peer: %d", e.Code)
	if e.Reason != "" {
		msg += " " + e.Reason
	}
	return msg
}

// BadHandshakeError type.
type BadHandshakeError struct {
	Status string
	Body   string
}

func (e *BadHandshakeError) Error() string {
	return fmt.Sprintf(
		"websocket bad handshake: %s. %s",
		e.Status, e.Body,
	)
}

func verifyWebSocketAccept(responseHeaders http.Header, websocketKey string) bool {
	expectedKey := websocketKey + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	hash := sha1.New()
	hash.Write([]byte(expectedKey))
	expectedAccept := base64.StdEncoding.EncodeToString(hash.Sum(nil))

	return responseHeaders.Get("Sec-WebSocket-Accept") == expectedAccept
}

func (ws *WebSocket) handshake(ctx context.Context, u *url.URL, header http.Header) error {
	// A fresh nonce per handshake, as RFC 6455 section 4.1 has it. Chrome
	// never looks at the key, but Browserless, Lightpanda and every
	// gorilla/websocket server refuse the literal upstream sent (rod #1092,
	// harvested from rod #1228). A system whose random source fails is not
	// one to go on with, and a constant key is what going on would mean;
	// Go 1.24 and later crash the program on that themselves.
	nonce := make([]byte, 16)
	_, err := rand.Read(nonce)
	utils.E(err)
	secKey := base64.StdEncoding.EncodeToString(nonce)

	req := (&http.Request{Method: http.MethodGet, URL: u, Header: http.Header{
		"Upgrade":               {"websocket"},
		"Connection":            {"Upgrade"},
		"Sec-WebSocket-Version": {"13"},
	}}).WithContext(ctx)

	// Names match case-insensitively: http.Header.Set spells the key
	// Sec-Websocket-Key, and a caller's key must replace the generated one,
	// not travel beside it as a second header.
	for k, vs := range header {
		switch {
		case strings.EqualFold(k, "Host") && len(vs) > 0:
			req.Host = vs[0]
		case strings.EqualFold(k, "Sec-WebSocket-Key") && len(vs) > 0:
			secKey = vs[0]
		default:
			req.Header[k] = vs
		}
	}
	req.Header["Sec-WebSocket-Key"] = []string{secKey}

	err = req.Write(ws.conn)
	if err != nil {
		return err
	}

	res, err := http.ReadResponse(ws.r, req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusSwitchingProtocols || !verifyWebSocketAccept(res.Header, secKey) {
		body, _ := io.ReadAll(res.Body)
		return &BadHandshakeError{
			Status: res.Status,
			Body:   string(body),
		}
	}

	return nil
}
