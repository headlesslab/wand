// Package main prints the Chrome the oldest milestone in the Support window
// resolves to, which is the browser the Support-window Nightly runs the suite
// on (spec #33, section 13; ADR-0008; ticket #61):
//
//	go run ./internal/tools/support-window   # 149.0.7632.83
//
// The Support window is the Target Chrome plus the three stable milestones
// before it (CONTEXT.md), so the oldest is the Target Chrome's milestone minus
// three, and the build is Chrome for Testing's last known good one of that
// milestone. The version goes to stdout alone, so that the fetch command can
// take it; everything else goes to stderr.
//
// Nothing here reads or writes the pins: the milestone is arithmetic on the
// Target Chrome, and the version behind it moves whenever Chrome for Testing
// publishes a new patch of that milestone, which is what makes this a Nightly
// rather than a pin.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/headlesslab/wand/lib/launcher/pins"
)

const (
	// milestones is how many stable milestones below the Target Chrome the
	// Support window reaches: the oldest one wand claims to work with.
	milestones = 3

	// perMilestone is Chrome for Testing's last known good build of every
	// milestone it still publishes, the same site the Roll reads its Stable
	// channel from (lib/launcher/pins/generate).
	perMilestone = "https://googlechromelabs.github.io/chrome-for-testing/latest-versions-per-milestone.json"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	milestone, err := oldest(pins.ChromeVersion)
	if err != nil {
		fail(err)
	}

	version, err := lastKnownGood(ctx, http.DefaultClient, perMilestone, milestone)
	if err != nil {
		fail(err)
	}

	fmt.Fprintf(os.Stderr, "support-window: the Target Chrome is %s, so the oldest milestone in the window is %d, whose last known good build is %s\n",
		pins.ChromeVersion, milestone, version)
	fmt.Println(version)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "support-window:", err)
	os.Exit(1)
}

// oldest is the milestone at the bottom of the Support window: the milestone
// of the Target Chrome given, less the window's depth.
func oldest(target string) (int, error) {
	major, _, has := strings.Cut(target, ".")
	if !has {
		return 0, fmt.Errorf("no milestone in the Target Chrome %q", target)
	}

	milestone, err := strconv.Atoi(major)
	if err != nil {
		return 0, fmt.Errorf("no milestone in the Target Chrome %q: %w", target, err)
	}

	if milestone <= milestones {
		return 0, fmt.Errorf("the Target Chrome %q has no %dth milestone below it", target, milestones)
	}

	return milestone - milestones, nil
}

// lastKnownGood is Chrome for Testing's last known good build of one
// milestone, which is a version of that milestone or nothing at all: a
// milestone old enough drops off the list, and a milestone that never reached
// stable was never on it.
func lastKnownGood(ctx context.Context, client *http.Client, url string, milestone int) (string, error) {
	var data struct {
		Milestones map[string]struct {
			Version string `json:"version"`
		} `json:"milestones"`
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s answered %s", url, res.Status)
	}

	if err := json.NewDecoder(res.Body).Decode(&data); err != nil {
		return "", fmt.Errorf("%s: %w", url, err)
	}

	key := strconv.Itoa(milestone)
	build, has := data.Milestones[key]
	if !has {
		return "", fmt.Errorf("no build of milestone %d is published by Chrome for Testing, so the oldest milestone in the Support window cannot be tested", milestone)
	}

	// A version of another milestone under this key would send the suite to
	// the wrong browser and read as a green Support window; the prefix is
	// what says it did not.
	if !strings.HasPrefix(build.Version, key+".") {
		return "", fmt.Errorf("%q is what Chrome for Testing gives as the last known good build of milestone %d, which is not a version of it", build.Version, milestone)
	}

	return build.Version, nil
}
