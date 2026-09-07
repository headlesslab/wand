package wand

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime/debug"
	"sync"
	"time"

	"github.com/headlesslab/wand/lib/cdp"
	"github.com/headlesslab/wand/lib/proto"
	"github.com/headlesslab/wand/lib/utils"
)

// CDPClient is usually used to make wand side-effect free. Such as proxy all IO of wand.
type CDPClient interface {
	Event() <-chan *cdp.Event
	Call(ctx context.Context, sessionID, method string, params interface{}) ([]byte, error)
}

// Message represents a cdp.Event.
type Message struct {
	SessionID proto.TargetSessionID
	Method    string

	lock  *sync.Mutex
	data  json.RawMessage
	event reflect.Value
}

// Load data into e, returns true if e matches the event type.
func (msg *Message) Load(e proto.Event) bool {
	if msg.Method != e.ProtoEvent() {
		return false
	}

	eVal := reflect.ValueOf(e)
	if eVal.Kind() != reflect.Pointer {
		return true
	}
	eVal = reflect.Indirect(eVal)

	msg.lock.Lock()
	defer msg.lock.Unlock()
	if msg.data == nil {
		eVal.Set(msg.event)
		return true
	}

	utils.E(json.Unmarshal(msg.data, e))
	msg.event = eVal
	msg.data = nil
	return true
}

// DefaultLogger for wand.
var DefaultLogger = log.New(os.Stdout, "[wand] ", log.LstdFlags)

// DefaultSleeper generates the default sleeper for retry, it uses backoff to grow the interval.
// The growth looks like:
//
//	A(0) = 100ms, A(n) = A(n-1) * random[1.9, 2.1), A(n) < 1s
//
// Why the default is not RequestAnimationFrame or DOM change events is because of if a retry never
// ends it can easily flood the program. But you can always easily config it into what you want.
var DefaultSleeper = func() utils.Sleeper {
	return utils.BackoffSleeper(100*time.Millisecond, time.Second, nil)
}

// NewPagePool instance.
func NewPagePool(limit int) Pool[Page] {
	return NewPool[Page](limit)
}

// NewBrowserPool instance.
func NewBrowserPool(limit int) Pool[Browser] {
	return NewPool[Browser](limit)
}

// Pool is used to thread-safely limit the number of elements at the same time: Get takes one
// of its slots, creating the element when the slot is empty, and Put gives the slot back with
// the element in it for the next Get. Use [NewPool], the zero value is not a pool.
//
// Cleanup ends the pool: from then on Get returns [ErrPoolCleanedUp] at once, and an
// element Put back later goes to Cleanup's function, so a caller that held one across
// Cleanup leaks nothing and hits no closed channel.
type Pool[T any] struct{ *pool[T] }

type pool[T any] struct {
	// slots holds one token per element the pool hands out at a time; a nil
	// token means Get creates the element.
	slots chan *T
	// done is closed by Cleanup, to wake every Get waiting on slots.
	done chan struct{}

	// mu guards the two fields below, keeps a Put out of the pool once
	// Cleanup drained it, and serializes the calls of cleanup.
	mu      sync.Mutex
	cleaned bool
	cleanup func(*T)
}

// NewPool instance.
func NewPool[T any](limit int) Pool[T] {
	p := &pool[T]{slots: make(chan *T, limit), done: make(chan struct{})}
	for i := 0; i < limit; i++ {
		p.slots <- nil
	}
	return Pool[T]{p}
}

// Get a elem from the pool, allow error. Use the [Pool[T].Put] to make it reusable later.
// A Get after Cleanup returns [ErrPoolCleanedUp]; one that took its slot before Cleanup
// ran completes, and its elem goes to Cleanup's function when it is Put back.
func (p Pool[T]) Get(create func() (*T, error)) (elem *T, err error) {
	select {
	case elem = <-p.slots:
	case <-p.done:
		return nil, ErrPoolCleanedUp
	}
	if elem == nil {
		elem, err = create()
	}
	return
}

// Put an elem back to the pool. After Cleanup the elem goes to Cleanup's function instead.
// A Put with every slot in the pool already, a Put without a Get, panics.
func (p Pool[T]) Put(elem *T) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cleaned {
		if elem != nil {
			p.cleanup(elem)
		}
		return
	}

	select {
	case p.slots <- elem:
	default:
		panic("wand: Pool.Put without a Get, every slot is in the pool already")
	}
}

// Cleanup runs iteratee on every element the pool holds and ends the pool: an element
// out of the pool at the time goes to iteratee when it is Put back, and Get returns
// [ErrPoolCleanedUp]. The calls of iteratee never overlap, whichever goroutine Puts;
// iteratee must not use the pool itself. A second Cleanup does nothing.
func (p Pool[T]) Cleanup(iteratee func(*T)) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cleaned {
		return
	}
	p.cleaned, p.cleanup = true, iteratee

	// Only what is in the pool right now: nothing comes back to it after
	// this, so the pool is empty for good once the lock is released.
	for i := 0; i < cap(p.slots); i++ {
		select {
		case elem := <-p.slots:
			if elem != nil {
				iteratee(elem)
			}
		default:
		}
	}
	close(p.done)
}

var _ io.ReadCloser = &StreamReader{}

// StreamReader for browser data stream.
type StreamReader struct {
	Offset *int

	c      proto.Client
	handle proto.IOStreamHandle
	buf    *bytes.Buffer
}

// NewStreamReader instance.
func NewStreamReader(c proto.Client, h proto.IOStreamHandle) *StreamReader {
	return &StreamReader{
		c:      c,
		handle: h,
		buf:    &bytes.Buffer{},
	}
}

func (sr *StreamReader) Read(p []byte) (n int, err error) {
	res, err := proto.IORead{
		Handle: sr.handle,
		Offset: sr.Offset,
	}.Call(sr.c)
	if err != nil {
		return 0, err
	}

	if !res.EOF {
		var bin []byte
		if res.Base64Encoded {
			bin, err = base64.StdEncoding.DecodeString(res.Data)
			if err != nil {
				return 0, err
			}
		} else {
			bin = []byte(res.Data)
		}

		_, _ = sr.buf.Write(bin)
	}

	return sr.buf.Read(p)
}

// Close the stream, discard any temporary backing storage.
func (sr *StreamReader) Close() error {
	return proto.IOClose{Handle: sr.handle}.Call(sr.c)
}

// Try try fn with recover, return the panic as wand.ErrTry.
func Try(fn func()) (err error) {
	defer func() {
		if val := recover(); val != nil {
			err = &TryError{val, string(debug.Stack())}
		}
	}()

	fn()

	return err
}

func genRegMatcher(includes, excludes []string) func(string) bool {
	regIncludes := make([]*regexp.Regexp, len(includes))
	for i, p := range includes {
		regIncludes[i] = regexp.MustCompile(p)
	}

	regExcludes := make([]*regexp.Regexp, len(excludes))
	for i, p := range excludes {
		regExcludes[i] = regexp.MustCompile(p)
	}

	return func(s string) bool {
		for _, include := range regIncludes {
			if include.MatchString(s) {
				for _, exclude := range regExcludes {
					if exclude.MatchString(s) {
						goto end
					}
				}
				return true
			}
		}
	end:
		return false
	}
}

type saveFileType int

const (
	saveFileTypeScreenshot saveFileType = iota
	saveFileTypePDF
)

func saveFile(fileType saveFileType, bin []byte, toFile []string) error {
	if len(toFile) == 0 {
		return nil
	}
	if toFile[0] == "" {
		stamp := fmt.Sprintf("%d", time.Now().UnixNano())
		switch fileType {
		case saveFileTypeScreenshot:
			toFile = []string{"tmp", "screenshots", stamp + ".png"}
		case saveFileTypePDF:
			toFile = []string{"tmp", "pdf", stamp + ".pdf"}
		}
	}
	return utils.OutputFile(filepath.Join(toFile...), bin)
}

func httHTML(w http.ResponseWriter, body string) {
	w.Header().Add("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(body))
}

func mustToJSONForDev(value interface{}) string {
	buf := new(bytes.Buffer)
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)

	utils.E(enc.Encode(value))

	return buf.String()
}

// https://developer.mozilla.org/en-US/docs/Web/HTTP/Basics_of_HTTP/Data_URIs
var regDataURI = regexp.MustCompile(`\Adata:(.+?)?(;base64)?,`)

func parseDataURI(uri string) (string, []byte) {
	matches := regDataURI.FindStringSubmatch(uri)
	l := len(matches[0])
	contentType := matches[1]

	bin, _ := base64.StdEncoding.DecodeString(uri[l:])
	return contentType, bin
}
