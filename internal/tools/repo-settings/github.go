package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
)

// api performs one REST call against GitHub and returns the HTTP status with
// the response body. A non-2xx status is a value, not an error: the caller
// decides what it means (a 404 on the Dependabot alerts endpoint says "off").
type api interface {
	call(method, path string, body any) (status int, out []byte, err error)
}

// ghAPI is the api over `gh api -i`. With -i gh prints the status line and the
// headers before the body, so the status is readable even though gh exits 1
// on a non-2xx response.
type ghAPI struct {
	bin string
}

// command starts gh; the tests swap it to re-execute the test binary as a fake gh.
var command = exec.CommandContext

func (g ghAPI) call(method, path string, body any) (int, []byte, error) {
	args := []string{"api", "-i", "--method", method, path}

	var stdin io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, nil, fmt.Errorf("%s %s: encoding body: %w", method, path, err)
		}
		args = append(args, "--input", "-")
		stdin = bytes.NewReader(b)
	}

	cmd := command(context.Background(), g.bin, args...)
	cmd.Stdin = stdin
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	out, runErr := cmd.Output()

	status, resp, err := parseResponse(out)
	if err != nil {
		if runErr != nil {
			return 0, nil, fmt.Errorf("%s %s: %w: %s", method, path, runErr, strings.TrimSpace(stderr.String()))
		}
		return 0, nil, fmt.Errorf("%s %s: %w", method, path, err)
	}
	return status, resp, nil
}

// parseResponse splits the output of `gh api -i` into the HTTP status and the
// body that follows the headers.
func parseResponse(raw []byte) (int, []byte, error) {
	line, rest, _ := bytes.Cut(raw, []byte("\n"))
	fields := strings.Fields(string(line))
	if len(fields) < 2 || !strings.HasPrefix(fields[0], "HTTP/") {
		return 0, nil, fmt.Errorf("no HTTP status line in gh output: %q", strings.TrimSpace(string(raw)))
	}
	status, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0, nil, fmt.Errorf("bad HTTP status line in gh output: %q", string(line))
	}

	body := rest
	for {
		header, after, found := bytes.Cut(body, []byte("\n"))
		if !found {
			return status, nil, nil
		}
		body = after
		if len(bytes.TrimRight(header, "\r")) == 0 {
			return status, body, nil
		}
	}
}

// client adds the two request shapes the settings need on top of an api.
type client struct {
	api api
}

// apiError is a response outside the 2xx range.
type apiError struct {
	method  string
	path    string
	status  int
	message string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("%s %s: %d %s", e.method, e.path, e.status, e.message)
}

// do sends one request and decodes a 2xx body into out when out is not nil.
// Any other status comes back as an *apiError.
func (c *client) do(method, path string, body, out any) error {
	status, resp, err := c.api.call(method, path, body)
	if err != nil {
		return err
	}
	if !success(status) {
		return &apiError{method: method, path: path, status: status, message: message(resp)}
	}
	if out != nil && len(bytes.TrimSpace(resp)) > 0 {
		if err := json.Unmarshal(resp, out); err != nil {
			return fmt.Errorf("%s %s: decoding response: %w", method, path, err)
		}
	}
	return nil
}

// exists reports whether a GET answers with a 2xx (true) or a 404 (false),
// the way GitHub's on/off endpoints without a body answer.
func (c *client) exists(path string) (bool, error) {
	status, resp, err := c.api.call(http.MethodGet, path, nil)
	if err != nil {
		return false, err
	}
	switch {
	case status == 404:
		return false, nil
	case success(status):
		return true, nil
	}
	return false, &apiError{method: http.MethodGet, path: path, status: status, message: message(resp)}
}

func success(status int) bool {
	return status >= 200 && status <= 299
}

// message extracts GitHub's error message from a body, or returns the body.
func message(resp []byte) string {
	var m struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(resp, &m) == nil && m.Message != "" {
		return m.Message
	}
	return strings.TrimSpace(string(resp))
}
