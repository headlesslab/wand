package cdp

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/ysmood/got"
)

var setup = got.Setup(nil)

func TestClientConnReset(t *testing.T) {
	g := setup(t)

	// A browser that dies with its socket open ends the stream with a
	// connection reset on Windows and with EOF elsewhere; an in-flight call
	// gets the same sentinel on every OS.
	ws := &resetWS{sent: make(chan struct{})}
	client := New().Start(ws)

	_, err := client.Call(g.Context(), "", "Browser.getVersion", nil)
	g.Eq(err, io.EOF)
}

// resetWS fails its first Read, once a request has been sent, the way the
// net package reports a reset by the peer.
type resetWS struct {
	sent chan struct{}
}

func (ws *resetWS) Send(_ []byte) error {
	close(ws.sent)
	return nil
}

func (ws *resetWS) Read() ([]byte, error) {
	<-ws.sent
	return nil, &net.OpError{Op: "read", Net: "tcp", Err: os.NewSyscallError("read", errConnReset)}
}

func TestWebSocketErr(t *testing.T) {
	g := setup(t)

	ws := WebSocket{}
	g.Err(ws.Connect(g.Context(), "://", nil))

	ws.Dialer = &net.Dialer{}
	ws.initDialer(nil)

	u, err := url.Parse("wss://no-exist")
	g.E(err)
	ws.Dialer = nil
	ws.initDialer(u)

	mc := &MockConn{}
	ws.conn = mc
	g.Err(ws.Send([]byte("test")))

	mc.errOnCount = 1
	mc.frame = []byte{0, 127, 1}
	ws.r = bufio.NewReader(mc)
	g.Err(ws.Read())

	mc.errOnCount = 1
	mc.frame = []byte{0}
	ws.r = bufio.NewReader(mc)
	g.Err(ws.Read())

	g.Err(ws.handshake(g.Timeout(0), nil, nil))

	mc.errOnCount = 1
	g.Err(ws.handshake(g.Context(), u, nil))

	tls := &tlsDialer{}
	g.Err(tls.DialContext(context.Background(), "", ""))
}

type MockConn struct {
	sync.Mutex

	errOnCount int
	frame      []byte
}

func (c *MockConn) checkErr(d int) error {
	c.Lock()
	defer c.Unlock()

	if c.errOnCount == 0 {
		return errors.New("err")
	}
	c.errOnCount += d
	return nil
}

func (c *MockConn) Read(b []byte) (int, error) {
	if err := c.checkErr(-1); err != nil {
		return 0, err
	}

	return copy(b, c.frame), nil
}

func (c *MockConn) Write(b []byte) (int, error) {
	if err := c.checkErr(-1); err != nil {
		return 0, err
	}
	return len(b), nil
}

func (c *MockConn) Close() error {
	return c.checkErr(0)
}

func (c *MockConn) LocalAddr() net.Addr {
	return nil
}

func (c *MockConn) RemoteAddr() net.Addr {
	return nil
}

func (c *MockConn) SetDeadline(_ time.Time) error {
	return nil
}

func (c *MockConn) SetReadDeadline(_ time.Time) error {
	return nil
}

func (c *MockConn) SetWriteDeadline(_ time.Time) error {
	return nil
}
