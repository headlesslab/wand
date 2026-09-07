package wand_test

import (
	"go/ast"
	godoc "go/doc"
	"go/parser"
	"go/token"
	"net"
	"net/url"
	"strconv"
	"testing"

	"github.com/ysmood/got"
)

// exampleFiles is every file of this module that holds Go examples.
var exampleFiles = []string{
	"examples_test.go",
	"lib/cdp/example_test.go",
	"lib/launcher/example_test.go",
}

// TestExamplesOffline fails on an example that both runs and names a public
// host. An example with an output comment is executed by "go test -run
// Example ./...", and no example may reach the public internet (spec #33,
// section 12; ticket #52); one without an output comment is documentation the
// run never executes, so a host it names is only read, never requested. The
// examples drive the pages under fixtures/examples instead, on a loopback
// port the OS picks.
func TestExamplesOffline(t *testing.T) {
	g := got.T(t)

	for _, name := range exampleFiles {
		file, err := parser.ParseFile(token.NewFileSet(), slash(name), nil, parser.ParseComments)
		g.E(err)

		examples := godoc.Examples(file)
		if len(examples) == 0 {
			g.Logf("%s: no example found, so this test proves nothing about it", name)
			g.Fail()
		}

		for _, example := range examples {
			if example.Output == "" {
				continue // compiled, never run
			}

			for _, host := range urlHosts(example.Code) {
				if !isLoopback(host) {
					g.Logf("%s: Example%s runs and names the public host %q", name, example.Name, host)
					g.Fail()
				}
			}
		}
	}
}

// urlHosts is the host of every string literal of node that parses as a URL
// with one.
func urlHosts(node ast.Node) []string {
	hosts := []string{}

	ast.Inspect(node, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}

		val, err := strconv.Unquote(lit.Value)
		if err != nil {
			return true
		}

		parsed, err := url.Parse(val)
		if err == nil && parsed.Hostname() != "" {
			hosts = append(hosts, parsed.Hostname())
		}

		return true
	})

	return hosts
}

// isLoopback reports whether host is the machine the run is on.
func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}

	ip := net.ParseIP(host)

	return ip != nil && ip.IsLoopback()
}
