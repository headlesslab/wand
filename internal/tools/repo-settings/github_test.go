package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

func TestParseResponse(t *testing.T) {
	g := setup(t)

	status, body, err := parseResponse([]byte("HTTP/2.0 200 OK\r\nContent-Type: application/json\r\n\r\n{\"a\":1}\n"))
	g.E(err)
	g.Eq(status, 200)
	g.Eq(string(body), "{\"a\":1}\n")

	status, body, err = parseResponse([]byte("HTTP/2.0 204 No Content\nX-Fake: 1\n\n"))
	g.E(err)
	g.Eq(status, 204)
	g.Len(body, 0)

	status, body, err = parseResponse([]byte("HTTP/2.0 204 No Content\nX-Fake: 1"))
	g.E(err)
	g.Eq(status, 204)
	g.Len(body, 0)

	status, body, err = parseResponse([]byte("HTTP/1.1 404 Not Found\r\n\r\n{\"message\":\"Not Found\"}"))
	g.E(err)
	g.Eq(status, 404)
	g.Eq(string(body), "{\"message\":\"Not Found\"}")

	_, _, err = parseResponse([]byte("gh: not logged in"))
	g.Err(err)
	g.Has(err.Error(), "gh: not logged in")

	_, _, err = parseResponse([]byte("HTTP/2.0 teapot\r\n\r\n"))
	g.Err(err)

	_, _, err = parseResponse(nil)
	g.Err(err)
}

func TestClient(t *testing.T) {
	g := setup(t)

	// A body that is not JSON is reported as such, and used verbatim in errors.
	c := &client{api: stubAPI{status: 200, body: "<html>"}}
	var out map[string]any
	err := c.do("GET", "repos/o/r", nil, &out)
	g.Err(err)
	g.Has(err.Error(), "decoding response")

	c = &client{api: stubAPI{status: 502, body: "<html>"}}
	err = c.do("GET", "repos/o/r", nil, nil)
	g.Eq(err.Error(), "GET repos/o/r: 502 <html>")

	// exists treats anything but a 2xx or a 404 as an error.
	c = &client{api: stubAPI{status: 403, body: `{"message":"no"}`}}
	_, err = c.exists("repos/o/r/vulnerability-alerts")
	g.Eq(err.Error(), "GET repos/o/r/vulnerability-alerts: 403 no")

	c = &client{api: stubAPI{err: errors.New("boom")}}
	_, err = c.exists("repos/o/r/vulnerability-alerts")
	g.Eq(err.Error(), "boom")
}

func TestGhAPICall(t *testing.T) {
	g := setup(t)

	// The binary itself is missing.
	_, _, err := ghAPI{bin: "definitely-not-gh-" + g.RandStr(8)}.call("GET", "repos/o/r", nil)
	g.Err(err)

	// From here on gh is this test binary running TestHelperProcess.
	prev := command
	command = func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, os.Args[0], append([]string{"-test.run=^TestHelperProcess$", "--"}, args...)...)
	}
	defer func() { command = prev }()
	t.Setenv("REPO_SETTINGS_FAKE_GH", "1")
	gh := ghAPI{bin: "gh"}

	// A 2xx with a body: gh exits 0.
	t.Setenv("REPO_SETTINGS_FAKE_STATUS", "200 OK")
	t.Setenv("REPO_SETTINGS_FAKE_EXIT", "0")
	status, body, err := gh.call("PUT", "repos/o/r/thing", map[string]any{"k": "v"})
	g.E(err)
	g.Eq(status, 200)
	g.Has(string(body), "args=api -i --method PUT repos/o/r/thing --input -")
	g.Has(string(body), "stdin={\"k\":\"v\"}")

	// A 404: gh exits 1 but still prints the response, which is what we parse.
	t.Setenv("REPO_SETTINGS_FAKE_STATUS", "404 Not Found")
	t.Setenv("REPO_SETTINGS_FAKE_EXIT", "1")
	status, body, err = gh.call("GET", "repos/o/r/missing", nil)
	g.E(err)
	g.Eq(status, 404)
	g.Has(string(body), "args=api -i --method GET repos/o/r/missing\n")
	g.Has(string(body), "stdin=\n")

	// No response at all (gh itself failed): the error carries gh's output.
	t.Setenv("REPO_SETTINGS_FAKE_STATUS", "")
	t.Setenv("REPO_SETTINGS_FAKE_EXIT", "4")
	_, _, err = gh.call("GET", "repos/o/r", nil)
	g.Err(err)
	g.Has(err.Error(), "not logged in")

	// A body that cannot be encoded.
	_, _, err = gh.call("PUT", "repos/o/r", map[string]any{"f": func() {}})
	g.Err(err)
	g.Has(err.Error(), "encoding body")
}

// TestHelperProcess stands in for gh when TestGhAPICall re-executes the test binary.
func TestHelperProcess(*testing.T) {
	if os.Getenv("REPO_SETTINGS_FAKE_GH") == "" {
		return
	}
	args := os.Args
	for i, a := range args {
		if a == "--" {
			args = args[i+1:]
			break
		}
	}
	stdin, _ := io.ReadAll(os.Stdin)
	code, _ := strconv.Atoi(os.Getenv("REPO_SETTINGS_FAKE_EXIT"))
	if status := os.Getenv("REPO_SETTINGS_FAKE_STATUS"); status != "" {
		_, _ = fmt.Fprintf(os.Stdout, "HTTP/2.0 %s\r\nX-Fake: yes\r\n\r\nargs=%s\nstdin=%s\n",
			status, strings.Join(args, " "), stdin)
	} else {
		_, _ = fmt.Fprintln(os.Stderr, "gh: not logged in")
	}
	os.Exit(code)
}

// stubAPI answers every call the same way.
type stubAPI struct {
	status int
	body   string
	err    error
}

func (s stubAPI) call(string, string, any) (int, []byte, error) {
	return s.status, []byte(s.body), s.err
}
