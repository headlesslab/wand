package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/headlesslab/wand/lib/launcher/pins"
)

// Pins is everything the pins package holds.
type Pins struct {
	ChromeVersion    string
	ChromePosition   int
	ProtocolRoll     int
	ChromiumPosition int
	ChromeSHA256     map[string]map[string]string // binary -> platform -> hex SHA-256
	ChromiumSHA256   map[string]string            // bucket prefix -> hex SHA-256
}

// current is what the pins package holds at build time.
func current() Pins {
	return Pins{
		ChromeVersion:    pins.ChromeVersion,
		ChromePosition:   pins.ChromePosition,
		ProtocolRoll:     pins.ProtocolRoll,
		ChromiumPosition: pins.ChromiumPosition,
		ChromeSHA256:     pins.ChromeSHA256,
		ChromiumSHA256:   pins.ChromiumSHA256,
	}
}

const (
	chromeForTesting = "https://googlechromelabs.github.io/chrome-for-testing"
	chromeBucket     = "https://storage.googleapis.com/chrome-for-testing-public"
	chromiumBucket   = "https://storage.googleapis.com/chromium-browser-snapshots"
	chromiumListing  = "https://storage.googleapis.com/storage/v1/b/chromium-browser-snapshots/o"
	protocolRepo     = "https://github.com/ChromeDevTools/devtools-protocol.git"

	// companionWindow is how far below the branch position the first search
	// for the Companion Chromium reaches. Every prefix gets a Chromium trunk
	// build within a few hundred positions; the search doubles the window
	// until it finds one, so the number only sets the size of the first
	// listing.
	companionWindow = 20000
)

var (
	chromeBinaries  = []string{"chrome", "chrome-headless-shell"}
	chromePlatforms = []string{"linux64", "linux-arm64", "mac-x64", "mac-arm64", "win32", "win64"}

	// chromiumArchives maps each prefix of the Chromium trunk build bucket to
	// the archive the launcher downloads from it.
	chromiumArchives = map[string]string{
		"Linux_x64": "chrome-linux.zip",
		"Mac":       "chrome-mac.zip",
		"Mac_Arm":   "chrome-mac.zip",
		"Win":       "chrome-win.zip",
		"Win_x64":   "chrome-win.zip",
	}
)

// roller computes the pins. Its endpoints are Google's in production and a
// local server in tests.
type roller struct {
	client          *http.Client
	cft             string // Chrome for Testing's version JSON
	chromeBucket    string // Chrome for Testing's archive bucket
	chromiumBucket  string // the Chromium trunk build bucket
	chromiumListing string // the JSON listing API of that bucket
	rolls           func(context.Context) ([]int, error)
	parallel        int
	log             *log.Logger
}

func newRoller() *roller {
	return &roller{
		client:          &http.Client{},
		cft:             chromeForTesting,
		chromeBucket:    chromeBucket,
		chromiumBucket:  chromiumBucket,
		chromiumListing: chromiumListing,
		rolls:           gitRolls,
		parallel:        4,
		log:             log.New(os.Stderr, "[pins] ", log.Ltime),
	}
}

// roll computes the pins for version, or for Chrome for Testing's Stable when
// version is empty. Archives Google does not serve come back in missing, by
// URL, with the pins holding everything that was verified.
func (r *roller) roll(ctx context.Context, version string) (Pins, []string, error) {
	var p Pins
	var err error
	if version == "" {
		p.ChromeVersion, p.ChromePosition, err = r.stable(ctx)
	} else {
		p.ChromeVersion = version
		p.ChromePosition, err = r.position(ctx, version)
	}
	if err != nil {
		return Pins{}, nil, err
	}
	r.log.Printf("Target Chrome %s, branch position %d", p.ChromeVersion, p.ChromePosition)

	rolls, err := r.rolls(ctx)
	if err != nil {
		return Pins{}, nil, fmt.Errorf("listing devtools-protocol tags: %w", err)
	}
	p.ProtocolRoll, err = protocolRoll(rolls, p.ChromePosition)
	if err != nil {
		return Pins{}, nil, err
	}
	r.log.Printf("Protocol roll r%d", p.ProtocolRoll)

	p.ChromiumPosition, err = r.companion(ctx, p.ChromePosition)
	if err != nil {
		return Pins{}, nil, err
	}
	r.log.Printf("Companion Chromium %d", p.ChromiumPosition)

	p.ChromeSHA256 = map[string]map[string]string{}
	for _, binary := range chromeBinaries {
		p.ChromeSHA256[binary] = map[string]string{}
	}
	p.ChromiumSHA256 = map[string]string{}

	archives := r.archives(p)
	sums, missing, err := r.download(ctx, archives)
	if err != nil {
		return Pins{}, nil, err
	}
	for _, a := range archives {
		if sum, has := sums[a.url]; has {
			a.record(&p, sum)
		}
	}

	return p, missing, nil
}

// check verifies the committed pins without writing: the Protocol roll must
// still be what the branch position derives, and file must be byte for byte
// what render gives for p, so that the next Roll's diff is only its values.
func (r *roller) check(ctx context.Context, p Pins, file []byte) error {
	rolls, err := r.rolls(ctx)
	if err != nil {
		return fmt.Errorf("listing devtools-protocol tags: %w", err)
	}
	roll, err := protocolRoll(rolls, p.ChromePosition)
	if err != nil {
		return err
	}
	if roll != p.ProtocolRoll {
		return fmt.Errorf("ProtocolRoll is r%d but branch position %d derives r%d", p.ProtocolRoll, p.ChromePosition, roll)
	}

	want, err := render(p)
	if err != nil {
		return err
	}
	if !bytes.Equal(normalize(file), want) {
		return fmt.Errorf("%s is not what the Roll writes for its own values; run the Roll again", pinsFile)
	}

	return nil
}

// normalize undoes the CRLF a Windows checkout with autocrlf applies, so the
// bytes compare to what the Roll writes.
func normalize(file []byte) []byte {
	return bytes.ReplaceAll(file, []byte("\r\n"), []byte("\n"))
}

// stable reads Chrome for Testing's last-known-good Stable version and its
// branch position, which the JSON calls the revision.
func (r *roller) stable(ctx context.Context) (string, int, error) {
	var data struct {
		Channels map[string]struct {
			Version  string `json:"version"`
			Revision string `json:"revision"`
		} `json:"channels"`
	}
	if err := r.getJSON(ctx, r.cft+"/last-known-good-versions.json", &data); err != nil {
		return "", 0, err
	}

	stable, has := data.Channels["Stable"]
	if !has {
		return "", 0, errors.New("no Stable channel in Chrome for Testing's last-known-good versions")
	}
	position, err := strconv.Atoi(stable.Revision)
	if err != nil {
		return "", 0, fmt.Errorf("bad branch position %q for Chrome for Testing Stable %s", stable.Revision, stable.Version)
	}

	return stable.Version, position, nil
}

// position reads the branch position of a Chrome for Testing version.
func (r *roller) position(ctx context.Context, version string) (int, error) {
	var data struct {
		Versions []struct {
			Version  string `json:"version"`
			Revision string `json:"revision"`
		} `json:"versions"`
	}
	if err := r.getJSON(ctx, r.cft+"/known-good-versions.json", &data); err != nil {
		return 0, err
	}

	for _, v := range data.Versions {
		if v.Version != version {
			continue
		}
		position, err := strconv.Atoi(v.Revision)
		if err != nil {
			return 0, fmt.Errorf("bad branch position %q for Chrome for Testing %s", v.Revision, version)
		}
		return position, nil
	}

	return 0, fmt.Errorf("%s is not a Chrome for Testing version", version)
}

func (r *roller) getJSON(ctx context.Context, u string, v interface{}) error {
	res, missing, err := r.do(ctx, http.MethodGet, u)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if missing {
		return fmt.Errorf("GET %s: %s", u, res.Status)
	}

	if err := json.NewDecoder(res.Body).Decode(v); err != nil {
		return fmt.Errorf("GET %s: %w", u, err)
	}
	return nil
}

// do sends one request and sorts the answer into found, missing (404) or an
// error for anything else. The caller closes the body of a found or missing
// response.
func (r *roller) do(ctx context.Context, method, u string) (*http.Response, bool, error) {
	req, err := http.NewRequestWithContext(ctx, method, u, nil)
	if err != nil {
		return nil, false, err
	}
	res, err := r.client.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("%s %s: %w", method, u, err)
	}

	switch res.StatusCode {
	case http.StatusOK:
		return res, false, nil
	case http.StatusNotFound:
		return res, true, nil
	default:
		_ = res.Body.Close()
		return nil, false, fmt.Errorf("%s %s: %s", method, u, res.Status)
	}
}

// gitRolls lists the v0.0.<rev> tags of ChromeDevTools/devtools-protocol
// through git, which needs no token and is not rate limited.
func gitRolls(ctx context.Context) ([]int, error) {
	cmd := exec.CommandContext(ctx, "git", "ls-remote", "--tags", "--refs", protocolRepo)
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-remote %s: %w\n%s", protocolRepo, err, stderr.String())
	}
	return parseRolls(string(out)), nil
}

// parseRolls extracts the roll numbers from git ls-remote output, one
// "<sha>\trefs/tags/<tag>" per line; tags of any other shape are ignored.
func parseRolls(out string) []int {
	rolls := []int{}
	for _, line := range strings.Split(out, "\n") {
		_, rev, found := strings.Cut(line, "\trefs/tags/v0.0.")
		if !found {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(rev))
		if err != nil {
			continue
		}
		rolls = append(rolls, n)
	}
	return rolls
}

// protocolRoll is the largest roll not above the branch position: a roll
// below the branch point is in that release, one above may carry unreleased
// changes (ADR-0004).
func protocolRoll(rolls []int, position int) (int, error) {
	best := 0
	for _, roll := range rolls {
		if roll <= position && roll > best {
			best = roll
		}
	}
	if best == 0 {
		return 0, fmt.Errorf("no devtools-protocol roll at or below branch position %d", position)
	}
	return best, nil
}

// companion is the newest Chromium trunk build at or below the branch
// position whose archive exists under every bucket prefix (ADR-0005). It
// searches a window below the position and doubles the window until a build
// is found or the window reaches position zero. A build listed for every
// prefix but lacking an archive under one of them is skipped, so a
// half-uploaded build never becomes the pin.
func (r *roller) companion(ctx context.Context, position int) (int, error) {
	prefixes := sortedKeys(chromiumArchives)

	for window := companionWindow; ; window *= 2 {
		lo := position - window
		if lo < 0 {
			lo = 0
		}

		lists := make([][]int, 0, len(prefixes))
		for _, prefix := range prefixes {
			list, err := r.listPositions(ctx, prefix, lo, position)
			if err != nil {
				return 0, err
			}
			lists = append(lists, list)
		}

		for _, candidate := range commonPositions(lists) {
			present, err := r.present(ctx, candidate)
			if err != nil {
				return 0, err
			}
			if present {
				return candidate, nil
			}
		}

		if lo == 0 {
			return 0, fmt.Errorf("no Chromium trunk build at or below position %d has its archive under all of %s",
				position, strings.Join(prefixes, ", "))
		}
	}
}

// listPositions lists the trunk positions under a bucket prefix between lo
// and hi inclusive. The listing API's offsets are lexicographic, so shorter
// numbers from years ago fall inside the window and are dropped here.
func (r *roller) listPositions(ctx context.Context, prefix string, lo, hi int) ([]int, error) {
	q := url.Values{}
	q.Set("prefix", prefix+"/")
	q.Set("delimiter", "/")
	q.Set("startOffset", fmt.Sprintf("%s/%d", prefix, lo))
	q.Set("endOffset", fmt.Sprintf("%s/%d", prefix, hi+1))
	q.Set("fields", "prefixes,nextPageToken")

	positions := []int{}
	for {
		var page struct {
			Prefixes      []string `json:"prefixes"`
			NextPageToken string   `json:"nextPageToken"`
		}
		if err := r.getJSON(ctx, r.chromiumListing+"?"+q.Encode(), &page); err != nil {
			return nil, err
		}

		for _, name := range page.Prefixes {
			n, err := strconv.Atoi(strings.Trim(strings.TrimPrefix(name, prefix+"/"), "/"))
			if err != nil || n < lo || n > hi {
				continue
			}
			positions = append(positions, n)
		}

		if page.NextPageToken == "" {
			return positions, nil
		}
		q.Set("pageToken", page.NextPageToken)
	}
}

// commonPositions is the positions present in every list, newest first.
func commonPositions(lists [][]int) []int {
	common := []int{}
	if len(lists) == 0 {
		return common
	}

	count := map[int]int{}
	for _, list := range lists {
		seen := map[int]bool{}
		for _, n := range list {
			if seen[n] {
				continue
			}
			seen[n] = true
			count[n]++
		}
	}
	for n, c := range count {
		if c == len(lists) {
			common = append(common, n)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(common)))

	return common
}

// present reports whether the archive of a Chromium trunk build exists under
// every bucket prefix.
func (r *roller) present(ctx context.Context, position int) (bool, error) {
	for _, prefix := range sortedKeys(chromiumArchives) {
		u := r.chromiumURL(prefix, position)
		res, missing, err := r.do(ctx, http.MethodHead, u)
		if err != nil {
			return false, err
		}
		_ = res.Body.Close()
		if missing {
			r.log.Printf("skipping Chromium %d: %s is missing", position, u)
			return false, nil
		}
	}
	return true, nil
}

func (r *roller) chromiumURL(prefix string, position int) string {
	return fmt.Sprintf("%s/%s/%d/%s", r.chromiumBucket, prefix, position, chromiumArchives[prefix])
}

// archive is one Managed browser archive the Roll downloads; record files its
// hash under the table and key it belongs to.
type archive struct {
	name   string
	url    string
	record func(p *Pins, sum string)
}

func (r *roller) archives(p Pins) []archive {
	list := []archive{}
	for _, binary := range chromeBinaries {
		for _, platform := range chromePlatforms {
			binary, platform := binary, platform
			list = append(list, archive{
				name:   binary + "/" + platform,
				url:    fmt.Sprintf("%s/%s/%s/%s-%s.zip", r.chromeBucket, p.ChromeVersion, platform, binary, platform),
				record: func(p *Pins, sum string) { p.ChromeSHA256[binary][platform] = sum },
			})
		}
	}
	for _, prefix := range sortedKeys(chromiumArchives) {
		prefix := prefix
		list = append(list, archive{
			name:   "chromium/" + prefix,
			url:    r.chromiumURL(prefix, p.ChromiumPosition),
			record: func(p *Pins, sum string) { p.ChromiumSHA256[prefix] = sum },
		})
	}
	return list
}

type fetched struct {
	sum     string
	missing bool
	err     error
}

// download hashes every archive, r.parallel at a time, and returns the hashes
// by URL and the URLs Google does not serve. The first failure that is not a
// consequence of cancelling the others is the error.
func (r *roller) download(ctx context.Context, archives []archive) (map[string]string, []string, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	results := make([]fetched, len(archives))
	slots := make(chan struct{}, r.parallel)
	var wg sync.WaitGroup
	for i := range archives {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			select {
			case slots <- struct{}{}:
			case <-ctx.Done():
				results[i].err = ctx.Err()
				return
			}
			defer func() { <-slots }()

			results[i] = r.fetch(ctx, archives[i])
			if results[i].err != nil {
				cancel()
			}
		}(i)
	}
	wg.Wait()

	sums := map[string]string{}
	missing := []string{}
	var firstErr error
	for i, f := range results {
		switch {
		case f.err != nil:
			if firstErr == nil || (errors.Is(firstErr, context.Canceled) && !errors.Is(f.err, context.Canceled)) {
				firstErr = f.err
			}
		case f.missing:
			missing = append(missing, archives[i].url)
		default:
			sums[archives[i].url] = f.sum
		}
	}
	if firstErr != nil {
		return nil, nil, firstErr
	}

	return sums, missing, nil
}

// fetch streams one archive through SHA-256 without keeping it.
func (r *roller) fetch(ctx context.Context, a archive) fetched {
	start := time.Now()

	res, missing, err := r.do(ctx, http.MethodGet, a.url)
	if err != nil {
		return fetched{err: err}
	}
	defer func() { _ = res.Body.Close() }()
	if missing {
		r.log.Printf("%s: missing, %s", a.name, a.url)
		return fetched{missing: true}
	}

	h := sha256.New()
	n, err := io.Copy(h, res.Body)
	if err != nil {
		return fetched{err: fmt.Errorf("GET %s: %w", a.url, err)}
	}
	sum := hex.EncodeToString(h.Sum(nil))
	r.log.Printf("%s: %s (%d MB in %s)", a.name, sum, n>>20, time.Since(start).Round(time.Second))

	return fetched{sum: sum}
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
