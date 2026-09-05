// Package devutil holds helpers for wand's generators and developer tools.
// It is not part of the public API: nothing under lib/ or the root package
// imports it except the generators that sit beside the packages they generate.
package devutil

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"reflect"
	"regexp"
	"strings"
	"text/template"

	"github.com/headlesslab/wand/lib/utils"
)

// TestEnvs for testing.
var TestEnvs = map[string]string{
	"GODEBUG": "tracebackancestors=100",
}

// S Template render, the params is key-value pairs.
func S(tpl string, params ...interface{}) string {
	var out bytes.Buffer

	dict := map[string]interface{}{}
	fnDict := template.FuncMap{}

	l := len(params)
	for i := 0; i < l-1; i += 2 {
		k := params[i].(string) //nolint: forcetypeassert
		v := params[i+1]
		if reflect.TypeOf(v).Kind() == reflect.Func {
			fnDict[k] = v
		} else {
			dict[k] = v
		}
	}

	t := template.Must(template.New("").Funcs(fnDict).Parse(tpl))
	utils.E(t.Execute(&out, dict))

	return out.String()
}

// ReadString reads file as string.
func ReadString(p string) (string, error) {
	bin, err := os.ReadFile(p)
	return string(bin), err
}

var regSpace = regexp.MustCompile(`\s`)

// Exec command.
func Exec(line string, rest ...string) string {
	return ExecLine(true, line, rest...)
}

var execLogger = log.New(os.Stdout, "[exec] ", 0)

// ExecLine of command.
func ExecLine(std bool, line string, rest ...string) string {
	args := rest
	if line != "" {
		args = append(regSpace.Split(line, -1), rest...)
	}

	execLogger.Println(utils.FormatCLIArgs(args))

	buf := bytes.NewBuffer(nil)

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stderr = buf
	cmd.Stdout = buf

	if std {
		cmd.Stdin = os.Stdin
		cmd.Stderr = io.MultiWriter(buf, os.Stderr)
		cmd.Stdout = io.MultiWriter(buf, os.Stdout)
	}

	if err := cmd.Run(); err != nil {
		if std {
			panic(err)
		}
		panic(fmt.Sprintf("%v\n%v", err, buf.String()))
	}

	return buf.String()
}

// UseNode installs Node.js and set the bin path to PATH env var.
func UseNode(std bool) {
	binPath := strings.TrimSpace(ExecLine(std, "go run github.com/ysmood/use-node@latest -p v20"))
	utils.E(os.Setenv("PATH", binPath+string(os.PathListSeparator)+os.Getenv("PATH")))
}

// EscapeGoString not using encoding like base64 or gzip because of they will
// make git diff every large for small change.
func EscapeGoString(s string) string {
	return "`" + strings.ReplaceAll(s, "`", "` + \"`\" + `") + "`"
}
