package pins_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/headlesslab/wand/lib/launcher/pins"
	"github.com/ysmood/got"
)

var setup = got.Setup(nil)

// hexSHA256 is what every archive hash must look like: lower-case hex of
// 32 bytes, the form the launcher compares a download against.
var hexSHA256 = regexp.MustCompile(`^[0-9a-f]{64}$`)

var (
	chromeBinaries   = []string{"chrome", "chrome-headless-shell"}
	chromePlatforms  = []string{"linux64", "linux-arm64", "mac-x64", "mac-arm64", "win32", "win64"}
	chromiumPrefixes = []string{"Linux_x64", "Mac", "Mac_Arm", "Win", "Win_x64"}
)

func TestTargetChrome(t *testing.T) {
	g := setup(t)

	parts := strings.Split(pins.ChromeVersion, ".")
	g.Len(parts, 4)
	for _, part := range parts {
		_, err := strconv.Atoi(part)
		g.Desc("%q", pins.ChromeVersion).E(err)
	}

	g.Gt(pins.ChromePosition, 0)

	// The Protocol roll is the largest devtools-protocol roll not above the
	// branch position, and the Companion Chromium the newest trunk build at or
	// below it, so neither may sit above the position.
	g.Gt(pins.ProtocolRoll, 0)
	g.Lte(pins.ProtocolRoll, pins.ChromePosition)
	g.Gt(pins.ChromiumPosition, 0)
	g.Lte(pins.ChromiumPosition, pins.ChromePosition)
}

func TestChromeArchives(t *testing.T) {
	g := setup(t)

	g.Len(pins.ChromeSHA256, len(chromeBinaries))

	for _, binary := range chromeBinaries {
		table, has := pins.ChromeSHA256[binary]
		g.Desc(binary).True(has)

		expected := 0
		for _, platform := range chromePlatforms {
			sum, has := table[platform]
			if !has && platformGap(platform) {
				continue
			}
			expected++
			g.Desc("%s/%s", binary, platform).True(has)
			g.Desc("%s/%s", binary, platform).Regex(hexSHA256.String(), sum)
		}

		// No platform outside the six.
		g.Desc(binary).Len(table, expected)
	}
}

// platformGap reports whether the Target Chrome may lack an archive for the
// platform. Chrome for Testing published its first linux-arm64 build with
// 153.0.8001.0, so a Target Chrome below 153 has none (ADR-0005). The first
// Roll, to 153 or later (spec #33, order of work, step 9), makes this always
// false; delete it with that Roll.
func platformGap(platform string) bool {
	major, err := strconv.Atoi(strings.SplitN(pins.ChromeVersion, ".", 2)[0])
	if err != nil {
		panic(err)
	}
	return platform == "linux-arm64" && major < 153
}

func TestChromiumArchives(t *testing.T) {
	g := setup(t)

	g.Len(pins.ChromiumSHA256, len(chromiumPrefixes))

	for _, prefix := range chromiumPrefixes {
		sum, has := pins.ChromiumSHA256[prefix]
		g.Desc(prefix).True(has)
		g.Desc(prefix).Regex(hexSHA256.String(), sum)
	}
}

// TestZeroImports pins the layout fact ADR-0009 lets CI assert: the package
// imports nothing, so the launcher and the protocol generator can both read
// it without either pulling in the other.
func TestZeroImports(t *testing.T) {
	g := setup(t)

	notTest := func(info fs.FileInfo) bool { return !strings.HasSuffix(info.Name(), "_test.go") }
	packages, err := parser.ParseDir(token.NewFileSet(), ".", notTest, parser.ImportsOnly)
	g.E(err)
	g.Len(packages, 1)

	for _, pkg := range packages {
		g.Gt(len(pkg.Files), 0)
		for name, file := range pkg.Files {
			g.Desc(name).Len(file.Imports, 0)
		}
	}
}
