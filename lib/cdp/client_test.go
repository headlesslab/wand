package cdp_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/headlesslab/lazyjson"
	"github.com/headlesslab/leakcheck"
	"github.com/headlesslab/wand/lib/cdp"
	"github.com/headlesslab/wand/lib/defaults"
	"github.com/headlesslab/wand/lib/launcher"
	"github.com/headlesslab/wand/lib/utils"
	"github.com/ysmood/got"
)

var setup = got.Setup(nil)

// launch starts a browser through l and makes sure it is gone with its user
// data directory when the test ends, whether the test closed it or not (the
// launcher kills a browser that does not exit within its bound).
func launch(g got.G, l *launcher.Launcher) string {
	g.Helper()
	u := l.MustLaunch()
	g.Cleanup(l.Cleanup)
	return u
}

func TestBasic(t *testing.T) {
	g := setup(t)

	ctx := g.Context()

	client := cdp.New().Logger(defaults.CDP).Start(cdp.MustConnectWS(launch(g, launcher.New())))

	defer func() {
		_, _ = client.Call(ctx, "", "Browser.close", nil)
	}()

	go func() {
		for range client.Event() {
			utils.Noop()
		}
	}()

	file, err := filepath.Abs(filepath.FromSlash("fixtures/iframe.html"))
	g.E(err)

	res, err := client.Call(ctx, "", "Target.createTarget", map[string]string{
		"url": "file://" + file,
	})
	g.E(err)

	targetID := lazyjson.New(res).Get("targetId").String()

	res, err = client.Call(ctx, "", "Target.attachToTarget", map[string]interface{}{
		"targetId": targetID,
		"flatten":  true, // if it's not set no response will return
	})
	g.E(err)

	sessionID := lazyjson.New(res).Get("sessionId").String()

	_, err = client.Call(ctx, sessionID, "Page.enable", nil)
	g.E(err)

	_, err = client.Call(ctx, "", "Target.attachToTarget", map[string]interface{}{
		"targetId": "abc",
	})
	g.Err(err)

	timeout := g.Context()

	sleeper := func() utils.Sleeper {
		return utils.BackoffSleeper(30*time.Millisecond, 3*time.Second, nil)
	}

	// cancel call
	tmpCtx, tmpCancel := context.WithCancel(ctx)
	tmpCancel()
	_, err = client.Call(tmpCtx, sessionID, "Runtime.evaluate", map[string]interface{}{
		"expression": `10`,
	})
	g.Eq(err.Error(), context.Canceled.Error())

	g.E(utils.Retry(timeout, sleeper(), func() (bool, error) {
		res, err = client.Call(ctx, sessionID, "Runtime.evaluate", map[string]interface{}{
			"expression": `document.querySelector('iframe')`,
		})

		return err == nil && lazyjson.New(res).Get("result.subtype").String() != "null", nil
	}))

	res, err = client.Call(ctx, sessionID, "DOM.describeNode", map[string]interface{}{
		"objectId": lazyjson.New(res).Get("result.objectId").String(),
	})
	g.E(err)

	frameID := lazyjson.New(res).Get("node.frameId").String()

	timeout = g.Context()

	g.E(utils.Retry(timeout, sleeper(), func() (bool, error) {
		// we might need to recreate the world because world can be
		// destroyed after the frame is reloaded
		res, err = client.Call(ctx, sessionID, "Page.createIsolatedWorld", map[string]interface{}{
			"frameId": frameID,
		})
		g.E(err)

		res, err = client.Call(ctx, sessionID, "Runtime.evaluate", map[string]interface{}{
			"contextId":  lazyjson.New(res).Get("executionContextId").Int(),
			"expression": `document.querySelector('h4')`,
		})

		return err == nil && lazyjson.New(res).Get("result.subtype").String() != "null", nil
	}))

	res, err = client.Call(ctx, sessionID, "DOM.getOuterHTML", map[string]interface{}{
		"objectId": lazyjson.New(res).Get("result.objectId").String(),
	})
	g.E(err)

	g.Eq("<h4>it works</h4>", lazyjson.New(res).Get("outerHTML").String())
}

func TestError(t *testing.T) {
	g := setup(t)

	cdpErr := cdp.Error{10, "err", "data"}
	g.Eq(cdpErr.Error(), "{10 err data}")
	g.True(cdpErr.Is(&cdpErr))
	g.False(cdpErr.Is(io.EOF))
	g.False(cdpErr.Is(&cdp.Error{Code: 10, Message: "err"}))

	// An evaluation interrupted by a navigation is one condition under two
	// messages: Chromium 128 reports it from V8, Chrome 152 from the DevTools
	// session. A stale context id is a different error, one callers retry.
	navigated := &cdp.Error{Code: -32000, Message: "Inspected target navigated or closed"}
	g.True(errors.Is(navigated, cdp.ErrCtxDestroyed))
	g.True(cdp.ErrCtxDestroyed.Is(navigated))
	g.False(errors.Is(cdp.ErrCtxNotFound, cdp.ErrCtxDestroyed))
	g.False(errors.Is(&cdp.Error{Code: -32000, Message: navigated.Message, Data: "x"}, cdp.ErrCtxDestroyed))

	g.Panic(func() {
		cdp.MustStartWithURL(context.Background(), "", nil)
	})
}

func TestCrash(t *testing.T) {
	g := setup(t)

	ctx := g.Context()

	client := cdp.MustStartWithURL(ctx, launch(g, launcher.New()), nil)

	go func() {
		for range client.Event() {
			utils.Noop()
		}
	}()

	file, err := filepath.Abs(filepath.FromSlash("fixtures/iframe.html"))
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

	_, err = client.Call(ctx, sessionID, "Page.enable", nil)
	g.E(err)

	// The crash is asked for from another goroutine, which reports its result
	// through the channel and asserts nothing itself: a goroutine that
	// asserted after the test had returned was the "Fail in goroutine after
	// TestCrash has completed" panic seen in the Gate.
	crashed := make(chan error, 1)
	go func() {
		utils.Sleep(1)
		_, err := client.Call(ctx, sessionID, "Browser.crash", nil)
		crashed <- err
	}()

	_, err = client.Call(ctx, sessionID, "Runtime.evaluate", map[string]interface{}{
		"expression":   `new Promise(() => {})`,
		"awaitPromise": true,
	})
	g.Eq(err, io.EOF)

	g.Eq(<-crashed, io.EOF)

	_, err = client.Call(ctx, sessionID, "Runtime.evaluate", map[string]interface{}{
		"expression": `10`,
	})
	g.Has(err.Error(), "use of closed network connection")
}

func TestFormat(t *testing.T) {
	g := setup(t)

	g.Eq(cdp.Request{
		ID:        123,
		SessionID: "000000001234",
		Method:    "test",
		Params:    1,
	}.String(), `=> #123 @00000000 test 1`)

	g.Eq(cdp.Response{
		ID:     0,
		Result: []byte("11"),
	}.String(), "<= #0 11")

	g.Eq(cdp.Response{Error: &cdp.Error{}}.String(), `<= #0 error: {"code":0,"message":"","data":""}`)

	g.Eq(cdp.Event{
		Method: "event",
		Params: []byte("11"),
	}.String(), `<- @00000000 event 11`)
}

func TestSlowSend(t *testing.T) {
	g := setup(t)

	leakcheck.CheckLeak(g, 0)

	id := 0
	wait := make(chan int)

	ws := &MockWebSocket{
		send: func([]byte) error {
			close(wait)
			utils.Sleep(0.3)
			return nil
		},
		read: func() ([]byte, error) {
			if id > 0 {
				return nil, io.EOF
			}

			id++
			<-wait

			return json.Marshal(cdp.Response{
				ID:     id,
				Result: json.RawMessage("1"),
				Error:  nil,
			})
		},
	}

	c := cdp.New().Start(ws)
	_, err := c.Call(g.Context(), "1234567890", "method", 1)
	g.E(err)
}

func TestCancelCallLeak(t *testing.T) {
	g := setup(t)

	leakcheck.CheckLeak(g, 0)

	for i := 0; i < 30; i++ {
		id := 0
		wait := make(chan int)

		ws := &MockWebSocket{
			send: func([]byte) error {
				close(wait)
				utils.Sleep(0.01)
				return nil
			},
			read: func() ([]byte, error) {
				if id > 0 {
					return nil, io.EOF
				}

				id++
				<-wait

				return json.Marshal(cdp.Response{
					ID:     id,
					Result: json.RawMessage("1"),
					Error:  nil,
				})
			},
		}

		c := cdp.New().Start(ws)
		ctx := g.Context()
		ctx.Cancel()
		_, _ = c.Call(ctx, "1234567890", "method", 1)
	}
}

func TestConcurrentCall(t *testing.T) {
	g := setup(t)

	leakcheck.CheckLeak(g, 0)

	req := make(chan []byte, 30)
	t.Cleanup(func() { close(req) })

	ws := &MockWebSocket{
		send: func(data []byte) error {
			req <- data
			return nil
		},
		read: func() ([]byte, error) {
			data, ok := <-req
			if !ok {
				return nil, io.EOF
			}

			var req cdp.Request
			err := json.Unmarshal(data, &req)
			if err != nil {
				return nil, err
			}

			return json.Marshal(cdp.Response{
				ID:     req.ID,
				Result: json.RawMessage(lazyjson.New(req.Params).JSON("", "")),
				Error:  nil,
			})
		},
	}

	c := cdp.New().Start(ws)

	for i := 0; i < 1000; i++ {
		i := i
		t.Run(fmt.Sprintf("%v", i), func(t *testing.T) {
			g := setup(t)
			g.Parallel()

			res, err := c.Call(g.Context(), "1234567890", "method", i)
			g.E(err)
			g.Eq(lazyjson.New(res).Int(), i)
		})
	}
}

// TestCallSendErrWhileClosing: the connection drops as a request goes out, so
// Send fails while the message loop is already failing every pending call.
// The loop must not block on the call that is about to return on its own:
// it once did, on a send to a channel nobody read any more, and the client
// never closed its event channel (a goroutine leak the root suite's
// TestBrowserVersionMismatch reported in the Gate).
func TestCallSendErrWhileClosing(t *testing.T) {
	g := setup(t)

	dropped := make(chan struct{})
	var client *cdp.Client
	ws := &MockWebSocket{
		read: func() ([]byte, error) {
			<-dropped
			return nil, errors.New("connection dropped")
		},
		send: func([]byte) error {
			// The read side fails now, and the message loop is given time to
			// finish with the pending call before Send reports its failure.
			close(dropped)
			select {
			case <-client.Event():
			case <-time.After(5 * time.Second):
				g.Fatal("the message loop did not finish while a failed Send was pending")
			}
			return errors.New("send failed")
		},
	}
	client = cdp.New().Start(ws)

	_, err := client.Call(g.Context(), "", "method", nil)
	g.Has(err.Error(), "send failed")
}

type MockWebSocket struct {
	send func(data []byte) error
	read func() ([]byte, error)
}

func (c *MockWebSocket) Send(data []byte) error {
	return c.send(data)
}

func (c *MockWebSocket) Read() ([]byte, error) {
	return c.read()
}
