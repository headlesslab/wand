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

const (
	chromeForTesting = "https://googlechromelabs.github.io/chrome-for-testing"
	chromeBucket     = "https://storage.googleapis.com/chrome-for-testing-public"
	snapshotsBucket  = "https://storage.googleapis.com/chromium-browser-snapshots"
	snapshotsAPI     = "https://storage.googleapis.com/storage/v1/b/chromium-browser-snapshots/o"
	protocolRepo     = "https://github.com/ChromeDevTools/devtools-protocol.git"

	// companionWindow is how far below the branch position the Companion
	// Chromium is looked for. Every prefix gets a build within a few hundred
	// positions; the window is generous so a prefix that paused for weeks
	// still resolves.
	companionWindow = 20000
)

var (
	chromeBinaries  = []string{"chrome", "chrome-headless-shell"}
	chromePlatforms = []string{"linux64", "linux-arm64", "mac-x64", "mac-arm64", "win32", "win64"}

	// chromiumArchives maps each Chromium snapshots bucket prefix to the
	// archive the launcher downloads from it.
	chromiumArchives = map[string]string{
		"Linux_x64": "chrome-linux.zip",
		"Mac":       "chrome-mac.zip",
		"Mac_Arm":   "chrome-mac.zip",
		"Win":       "chrome-win.zip",
		"Win_x64":   "chrome-win.zip",
	}
)

// sources is where the Roll reads from: Google's endpoints in production,
// a local server in tests.
type sources struct {
	client       *http.Client
	cft          string // Chrome for Testing's version JSON
	cftBucket    string // Chrome for Testing's archive bucket
	snapshots    string // the Chromium snapshots bucket
	snapshotsAPI string // the JSON listing API of that bucket
	rolls        func(context.Context) ([]int, error)
	parallel     int
	log          *log.Logger
}

func newSources() *sources {
	return &sources{
		client:       &http.Client{},
		cft:          chromeForTesting,
		cftBucket:    chromeBucket,
		snapshots:    snapshotsBucket,
		snapshotsAPI: snapshotsAPI,
		rolls:        gitRolls,
		parallel:     4,
		log:          log.New(os.Stderr, "[pins] ", log.Ltime),
	}
}

// roll computes the pins for version, or for Chrome for Testing's Stable when
// version is empty. Archives Google does not serve come back in missing, by
// URL, with the pins holding everything that was verified.
func (s *sources) roll(ctx context.Context, version string) (Pins, []string, error) {
	var p Pins
	var err error
	if version == "" {
		p.ChromeVersion, p.ChromePosition, err = s.stable(ctx)
	} else {
		p.ChromeVersion = version
		p.ChromePosition, err = s.position(ctx, version)
	}
	if err != nil {
		return Pins{}, nil, err
	}
	s.log.Printf("Target Chrome %s, branch position %d", p.ChromeVersion, p.ChromePosition)

	rolls, err := s.rolls(ctx)
	if err != nil {
		return Pins{}, nil, fmt.Errorf("listing devtools-protocol tags: %w", err)
	}
	p.ProtocolRoll, err = protocolRoll(rolls, p.ChromePosition)
	if err != nil {
		return Pins{}, nil, err
	}
	s.log.Printf("Protocol roll r%d", p.ProtocolRoll)

	p.ChromiumPosition, err = s.companion(ctx, p.ChromePosition)
	if err != nil {
		return Pins{}, nil, err
	}
	s.log.Printf("Companion Chromium %d", p.ChromiumPosition)

	archives := s.archives(p)
	sums, missing, err := s.download(ctx, archives)
	if err != nil {
		return Pins{}, nil, err
	}

	p.ChromeSHA256 = map[string]map[string]string{}
	for _, binary := range chromeBinaries {
		p.ChromeSHA256[binary] = map[string]string{}
	}
	p.ChromiumSHA256 = map[string]string{}
	for _, a := range archives {
		sum, has := sums[a.url]
		if !has {
			continue
		}
		if a.prefix != "" {
			p.ChromiumSHA256[a.prefix] = sum
		} else {
			p.ChromeSHA256[a.binary][a.platform] = sum
		}
	}

	return p, missing, nil
}

// check verifies the committed pins without writing: the Protocol roll must
// still be what the branch position derives, and file must be what render
// gives for p.
func (s *sources) check(ctx context.Context, p Pins, file []byte) error {
	rolls, err := s.rolls(ctx)
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

// stable reads Chrome for Testing's last-known-good Stable version and its
// branch position.
func (s *sources) stable(ctx context.Context) (string, int, error) {
	var data struct {
		Channels map[string]struct {
			Version  string `json:"version"`
			Revision string `json:"revision"`
		} `json:"channels"`
	}
	if err := s.getJSON(ctx, s.cft+"/last-known-good-versions.json", &data); err != nil {
		return "", 0, err
	}

	stable, has := data.Channels["Stable"]
	if !has {
		return "", 0, errors.New("no Stable channel in Chrome for Testing's last-known-good versions")
	}
	position, err := strconv.Atoi(stable.Revision)
	if err != nil {
		return "", 0, fmt.Errorf("bad revision %q for Chrome for Testing Stable %s", stable.Revision, stable.Version)
	}

	return stable.Version, position, nil
}

// position reads the branch position of a Chrome for Testing version.
func (s *sources) position(ctx context.Context, version string) (int, error) {
	var data struct {
		Versions []struct {
			Version  string `json:"version"`
			Revision string `json:"revision"`
		} `json:"versions"`
	}
	if err := s.getJSON(ctx, s.cft+"/known-good-versions.json", &data); err != nil {
		return 0, err
	}

	for _, v := range data.Versions {
		if v.Version != version {
			continue
		}
		position, err := strconv.Atoi(v.Revision)
		if err != nil {
			return 0, fmt.Errorf("bad revision %q for Chrome for Testing %s", v.Revision, version)
		}
		return position, nil
	}

	return 0, fmt.Errorf("%s is not a Chrome for Testing version", version)
}

func (s *sources) getJSON(ctx context.Context, u string, v interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	res, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", u, res.Status)
	}
	if err := json.NewDecoder(res.Body).Decode(v); err != nil {
		return fmt.Errorf("GET %s: %w", u, err)
	}
	return nil
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

// companion is the newest Chromium trunk position at or below the branch
// position whose archive exists under every bucket prefix (ADR-0005). A
// position listed for every prefix but lacking an archive under one of them
// is skipped, so a half-uploaded build never becomes the pin.
func (s *sources) companion(ctx context.Context, position int) (int, error) {
	lo := position - companionWindow
	prefixes := sortedKeys(chromiumArchives)

	lists := make([][]int, 0, len(prefixes))
	for _, prefix := range prefixes {
		list, err := s.listPositions(ctx, prefix, lo, position)
		if err != nil {
			return 0, err
		}
		if len(list) == 0 {
			return 0, fmt.Errorf("no Chromium trunk build under %s between positions %d and %d", prefix, lo, position)
		}
		lists = append(lists, list)
	}

	candidates := commonPositions(lists...)
	if len(candidates) == 0 {
		return 0, fmt.Errorf("no Chromium trunk position between %d and %d is present for all of %s",
			lo, position, strings.Join(prefixes, ", "))
	}

	for _, candidate := range candidates {
		present, err := s.present(ctx, candidate)
		if err != nil {
			return 0, err
		}
		if present {
			return candidate, nil
		}
	}

	return 0, fmt.Errorf("no Chromium trunk position between %d and %d has its archive under all of %s",
		lo, position, strings.Join(prefixes, ", "))
}

// listPositions lists the trunk positions under a bucket prefix between lo
// and hi inclusive. The listing API's offsets are lexicographic, so shorter
// numbers from years ago fall inside the window and are dropped here.
func (s *sources) listPositions(ctx context.Context, prefix string, lo, hi int) ([]int, error) {
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
		if err := s.getJSON(ctx, s.snapshotsAPI+"?"+q.Encode(), &page); err != nil {
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
func commonPositions(lists ...[]int) []int {
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

// present reports whether the archive of a trunk position exists under every
// bucket prefix.
func (s *sources) present(ctx context.Context, position int) (bool, error) {
	for _, prefix := range sortedKeys(chromiumArchives) {
		u := s.snapshotURL(prefix, position)
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, u, nil)
		if err != nil {
			return false, err
		}
		res, err := s.client.Do(req)
		if err != nil {
			return false, err
		}
		_ = res.Body.Close()

		switch res.StatusCode {
		case http.StatusOK:
		case http.StatusNotFound:
			s.log.Printf("skipping Chromium %d: %s is missing", position, u)
			return false, nil
		default:
			return false, fmt.Errorf("HEAD %s: %s", u, res.Status)
		}
	}
	return true, nil
}

func (s *sources) snapshotURL(prefix string, position int) string {
	return fmt.Sprintf("%s/%s/%d/%s", s.snapshots, prefix, position, chromiumArchives[prefix])
}

// archive is one Managed browser archive the Roll downloads: a Chrome for
// Testing one (binary and platform) or a Companion Chromium one (prefix).
type archive struct {
	name     string
	binary   string
	platform string
	prefix   string
	url      string
}

func (s *sources) archives(p Pins) []archive {
	list := []archive{}
	for _, binary := range chromeBinaries {
		for _, platform := range chromePlatforms {
			list = append(list, archive{
				name:     binary + "/" + platform,
				binary:   binary,
				platform: platform,
				url:      fmt.Sprintf("%s/%s/%s/%s-%s.zip", s.cftBucket, p.ChromeVersion, platform, binary, platform),
			})
		}
	}
	for _, prefix := range sortedKeys(chromiumArchives) {
		list = append(list, archive{
			name:   "chromium/" + prefix,
			prefix: prefix,
			url:    s.snapshotURL(prefix, p.ChromiumPosition),
		})
	}
	return list
}

type fetched struct {
	sum     string
	missing bool
	err     error
}

// download hashes every archive, s.parallel at a time, and returns the hashes
// by URL and the URLs Google does not serve. The first failure that is not a
// consequence of cancelling the others is the error.
func (s *sources) download(ctx context.Context, archives []archive) (map[string]string, []string, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	results := make([]fetched, len(archives))
	slots := make(chan struct{}, s.parallel)
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

			results[i] = s.fetch(ctx, archives[i])
			if results[i].err != nil {
				cancel()
			}
		}(i)
	}
	wg.Wait()

	sums := map[string]string{}
	missing := []string{}
	var firstErr error
	for i, r := range results {
		switch {
		case r.err != nil:
			if firstErr == nil || (errors.Is(firstErr, context.Canceled) && !errors.Is(r.err, context.Canceled)) {
				firstErr = r.err
			}
		case r.missing:
			missing = append(missing, archives[i].url)
		default:
			sums[archives[i].url] = r.sum
		}
	}
	if firstErr != nil {
		return nil, nil, firstErr
	}

	return sums, missing, nil
}

// fetch streams one archive through SHA-256 without keeping it.
func (s *sources) fetch(ctx context.Context, a archive) fetched {
	start := time.Now()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.url, nil)
	if err != nil {
		return fetched{err: err}
	}
	res, err := s.client.Do(req)
	if err != nil {
		return fetched{err: fmt.Errorf("GET %s: %w", a.url, err)}
	}
	defer func() { _ = res.Body.Close() }()

	switch res.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		s.log.Printf("%s: missing, %s", a.name, a.url)
		return fetched{missing: true}
	default:
		return fetched{err: fmt.Errorf("GET %s: %s", a.url, res.Status)}
	}

	h := sha256.New()
	n, err := io.Copy(h, res.Body)
	if err != nil {
		return fetched{err: fmt.Errorf("GET %s: %w", a.url, err)}
	}
	sum := hex.EncodeToString(h.Sum(nil))
	s.log.Printf("%s: %s (%d MB in %s)", a.name, sum, n>>20, time.Since(start).Round(time.Second))

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
