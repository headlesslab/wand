package devutil

// golangciLint is the one pinned Go tool of the generators and the lint tool.
// Its formatters (gofmt, gofumpt, goimports, gci) and its linters run at the
// versions its own go.mod pins, so moving this line moves them all together;
// the satellites' reusable workflow in headlesslab/.github pins the same
// version. go run builds it into the build cache on first use and installs
// nothing (spec #33, section 13: no @latest in a Gate).
const golangciLint = "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2"

// GolangciLint runs golangci-lint at the pinned version with args, such as
// "fmt ./lib/proto" to rewrite or "run ./..." to report, and returns what it
// printed.
func GolangciLint(args ...string) string {
	return Exec("go run "+golangciLint, args...)
}

// nodeTools is where the Node tools live: package.json there names each at
// an exact version and package-lock.json fixes their whole tree, so the
// setup tool installs them with npm ci and a Gate never resolves a version.
const nodeTools = "internal/tools"

// InstallNodeTools installs the pinned Node tools from the lockfile.
func InstallNodeTools() {
	Exec("npm --prefix " + nodeTools + " ci")
}

// NodeTool runs one of the pinned Node tools by its bin name from the module
// root, echoing its output as well as returning it.
func NodeTool(name string, args ...string) string {
	return nodeTool(true, name, args...)
}

// NodeToolOutput is NodeTool with the output captured rather than echoed,
// for a caller that parses it.
func NodeToolOutput(name string, args ...string) string {
	return nodeTool(false, name, args...)
}

// nodeTool runs a pinned Node tool through npm exec; --no refuses to fetch
// anything the lockfile did not install.
func nodeTool(std bool, name string, args ...string) string {
	return ExecLine(std, "npm --prefix "+nodeTools+" exec --no -- "+name, args...)
}
