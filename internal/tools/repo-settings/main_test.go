package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/ysmood/got"
)

var setup = got.Setup(nil)

func TestParseArgs(t *testing.T) {
	g := setup(t)

	_, err := parseArgs(nil)
	g.Err(err)
	g.Has(err.Error(), "at least one")

	for _, bad := range []string{"wand", "/wand", "headlesslab/", "a/b/c"} {
		_, err = parseArgs([]string{bad})
		g.Desc("%q", bad).Err(err)
		g.Desc("%q", bad).Has(err.Error(), "owner/name")
	}

	_, err = parseArgs([]string{"-bogus", "headlesslab/wand"})
	g.Err(err)

	opts, err := parseArgs([]string{
		"-check", "go / lint",
		"-check", "go / test (ubuntu-latest, floor)",
		"-app", "headlesslab-bot",
		"-dry-run",
		"headlesslab/wand", "headlesslab/fetch",
	})
	g.E(err)
	g.Eq(opts.repos, []string{"headlesslab/wand", "headlesslab/fetch"})
	g.Eq(opts.checks, []string{"go / lint", "go / test (ubuntu-latest, floor)"})
	g.Eq((*repeatable)(&opts.checks).String(), "go / lint, go / test (ubuntu-latest, floor)")
	g.Eq(opts.app, "headlesslab-bot")
	g.True(opts.dryRun)

	opts, err = parseArgs([]string{"headlesslab/wand"})
	g.E(err)
	g.Len(opts.checks, 0)
	g.Eq(opts.app, "")
	g.False(opts.dryRun)
}

func TestUsage(t *testing.T) {
	g := setup(t)

	out := &bytes.Buffer{}
	usage(out)
	g.Has(out.String(), "usage: go run ./internal/tools/repo-settings")
	g.Has(out.String(), "-check value")
	g.Has(out.String(), "-app string")
	g.Has(out.String(), "-dry-run")
}

func TestLookupApp(t *testing.T) {
	g := setup(t)

	f := newFakeGitHub("headlesslab/wand")
	f.apps["headlesslab-bot"] = 4242
	c := &client{api: f}

	id, err := lookupApp(c, "headlesslab-bot")
	g.E(err)
	g.Eq(id, 4242)

	_, err = lookupApp(c, "nobody")
	g.Err(err)
	g.Has(err.Error(), "apps/nobody")
	g.Has(err.Error(), "404")
	g.Has(err.Error(), "numeric App ID")

	// A numeric App ID is used as it is, without a lookup.
	id, err = lookupApp(&client{api: stubAPI{err: errors.New("no call expected")}}, "4242")
	g.E(err)
	g.Eq(id, 4242)

	// An App response without an id is an error, not App 0.
	_, err = lookupApp(&client{api: stubAPI{status: 200, body: `{"slug":"x"}`}}, "x")
	g.Err(err)
	g.Has(err.Error(), "no id")
}

func TestRunAppliesThenIdle(t *testing.T) {
	g := setup(t)

	f := newFakeGitHub("headlesslab/wand", "headlesslab/fetch")
	// fetch starts with immutable releases already on, like the real satellites.
	f.repos["headlesslab/fetch"].immutable = true
	c := &client{api: f}
	checks := []string{"go / lint", "go / govulncheck"}
	settings := bundle(checks, 7)
	g.Len(settings, 9)

	out := &bytes.Buffer{}
	changes, err := run(c, []string{"headlesslab/wand", "headlesslab/fetch"}, settings, false, out)
	g.E(err)
	g.Eq(changes, 17)
	g.Has(out.String(), "headlesslab/wand\n")
	g.Has(out.String(), "secret scanning                  disabled -> enabled")
	g.Has(out.String(), "push protection                  disabled -> enabled")
	g.Has(out.String(), "Dependabot alerts                disabled -> enabled")
	g.Has(out.String(), "Dependabot security updates      disabled -> enabled")
	g.Has(out.String(), "private vulnerability reporting  disabled -> enabled")
	g.Has(out.String(), "immutable releases               disabled -> enabled")
	g.Has(out.String(), "immutable releases               enabled\n")
	g.Has(out.String(), "Actions SHA pinning              optional -> required")
	g.Has(out.String(), "ruleset main                     missing -> up to date")
	g.Has(out.String(), "ruleset v*                       missing -> up to date")
	g.Has(out.String(), "headlesslab/wand: 9 changes\n")
	g.Has(out.String(), "headlesslab/fetch: 8 changes\n")
	g.Has(out.String(), "2 repositories, 17 changes\n")

	// The fake mirrors GitHub: every write landed.
	for _, name := range []string{"headlesslab/wand", "headlesslab/fetch"} {
		r := f.repos[name]
		g.Desc("%s", name).Eq(r.secretScanning, "enabled")
		g.Desc("%s", name).Eq(r.pushProtection, "enabled")
		g.Desc("%s", name).True(r.alerts)
		g.Desc("%s", name).True(r.securityUpdates)
		g.Desc("%s", name).True(r.reporting)
		g.Desc("%s", name).True(r.immutable)
		g.Desc("%s", name).True(r.shaPinning)
		g.Desc("%s", name).Eq(r.allowedActions, "all")
		g.Desc("%s", name).Len(r.rulesets, 2)
		g.Desc("%s", name).True(subset(normalize(mainRuleset(checks, bypassActors(7))), r.rulesets[0]))
		g.Desc("%s", name).True(subset(normalize(tagRuleset(bypassActors(7))), r.rulesets[1]))
	}

	// A second run finds everything in place and writes nothing.
	writes := f.writes
	out.Reset()
	changes, err = run(c, []string{"headlesslab/wand", "headlesslab/fetch"}, settings, false, out)
	g.E(err)
	g.Eq(changes, 0)
	g.Eq(f.writes, writes)
	g.Has(out.String(), "ruleset main                     up to date\n")
	g.Has(out.String(), "headlesslab/wand: no changes\n")
	g.Has(out.String(), "2 repositories, no changes\n")

	// Extending the required checks updates the main ruleset and nothing else.
	out.Reset()
	changes, err = run(c, []string{"headlesslab/wand"}, bundle(append(checks, "go / test"), 7), false, out)
	g.E(err)
	g.Eq(changes, 1)
	g.Has(out.String(), "ruleset main                     differs -> up to date")
	g.Has(out.String(), "headlesslab/wand: 1 change\n")
	g.Len(f.repos["headlesslab/wand"].rulesets, 2)
	g.True(subset(normalize(mainRuleset(append(checks, "go / test"), bypassActors(7))), f.repos["headlesslab/wand"].rulesets[0]))
}

func TestRunDryRun(t *testing.T) {
	g := setup(t)

	f := newFakeGitHub("headlesslab/wand")
	c := &client{api: f}

	out := &bytes.Buffer{}
	changes, err := run(c, []string{"headlesslab/wand"}, bundle(nil, 0), true, out)
	g.E(err)
	g.Eq(changes, 9)
	g.Eq(f.writes, 0)
	g.Has(out.String(), "secret scanning                  disabled -> enabled (dry run)")
	g.Has(out.String(), "ruleset main                     missing -> up to date (dry run)")
	g.Has(out.String(), "headlesslab/wand: 9 changes pending\n")
	g.Has(out.String(), "1 repository, 9 changes pending (dry run)\n")
	g.Eq(f.repos["headlesslab/wand"].secretScanning, "disabled")
}

func TestRunVerifiesWrites(t *testing.T) {
	g := setup(t)

	f := newFakeGitHub("headlesslab/wand")
	f.ignoreWrites = true
	c := &client{api: f}

	_, err := run(c, []string{"headlesslab/wand"}, bundle(nil, 0), false, &bytes.Buffer{})
	g.Err(err)
	g.Has(err.Error(), "headlesslab/wand")
	g.Has(err.Error(), "secret scanning")
	g.Has(err.Error(), "still \"disabled\"")
}

func TestRunReportsAPIErrors(t *testing.T) {
	g := setup(t)

	f := newFakeGitHub("headlesslab/wand")
	f.fail["repos/headlesslab/wand/immutable-releases"] = 403
	c := &client{api: f}

	_, err := run(c, []string{"headlesslab/wand"}, bundle(nil, 0), false, &bytes.Buffer{})
	g.Err(err)
	g.Has(err.Error(), "immutable releases")
	g.Has(err.Error(), "GET repos/headlesslab/wand/immutable-releases: 403")
	g.Has(err.Error(), "Forbidden by the fake")

	// A ruleset that lists but cannot be read.
	delete(f.fail, "repos/headlesslab/wand/immutable-releases")
	_, err = run(c, []string{"headlesslab/wand"}, bundle(nil, 0), false, &bytes.Buffer{})
	g.E(err)
	f.fail["repos/headlesslab/wand/rulesets/100"] = 500
	_, err = run(c, []string{"headlesslab/wand"}, bundle(nil, 0), false, &bytes.Buffer{})
	g.Err(err)
	g.Has(err.Error(), "ruleset main: GET repos/headlesslab/wand/rulesets/100: 500")

	// A transport failure surfaces as is.
	f.fail["repos/headlesslab/wand"] = -1
	_, err = run(c, []string{"headlesslab/wand"}, bundle(nil, 0), false, &bytes.Buffer{})
	g.Err(err)
	g.Has(err.Error(), "transport down")
}

// fakeGitHub is an in-memory stand-in for the endpoints the tool touches. It
// mirrors GitHub where the details matter: push protection needs secret
// scanning first, security updates need alerts first, Dependabot alerts answer
// 204 or 404, and a stored ruleset grows the fields GitHub adds on its own.
type fakeGitHub struct {
	repos        map[string]*fakeRepo
	apps         map[string]int
	fail         map[string]int // path -> status to answer with; -1 fails the transport
	writes       int
	ignoreWrites bool
}

type fakeRepo struct {
	secretScanning  string
	pushProtection  string
	alerts          bool
	securityUpdates bool
	reporting       bool
	immutable       bool
	allowedActions  string
	shaPinning      bool
	rulesets        []map[string]any
	nextRulesetID   int
}

func newFakeGitHub(repos ...string) *fakeGitHub {
	f := &fakeGitHub{repos: map[string]*fakeRepo{}, apps: map[string]int{}, fail: map[string]int{}}
	for _, r := range repos {
		f.repos[r] = &fakeRepo{secretScanning: "disabled", pushProtection: "disabled", allowedActions: "all", nextRulesetID: 100}
	}
	return f
}

func (f *fakeGitHub) call(method, path string, body any) (int, []byte, error) {
	if status, ok := f.fail[path]; ok {
		if status < 0 {
			return 0, nil, errors.New("transport down")
		}
		return status, []byte(`{"message":"Forbidden by the fake"}`), nil
	}
	if method != "GET" {
		f.writes++
		if f.ignoreWrites {
			return 204, nil, nil
		}
	}

	if slug, ok := strings.CutPrefix(path, "apps/"); ok {
		if id, ok := f.apps[slug]; ok {
			return jsonResponse(200, map[string]any{"id": id, "slug": slug})
		}
		return notFound()
	}

	parts := strings.SplitN(strings.TrimPrefix(path, "repos/"), "/", 3)
	if len(parts) < 2 {
		return notFound()
	}
	r, ok := f.repos[parts[0]+"/"+parts[1]]
	if !ok {
		return notFound()
	}
	rest := ""
	if len(parts) == 3 {
		rest = parts[2]
	}
	in := normalize(body)

	switch {
	case rest == "":
		return r.security(method, in)
	case strings.HasPrefix(rest, "rulesets"):
		return r.ruleset(method, rest, in, parts[0]+"/"+parts[1])
	}
	return r.toggle(method, rest, in)
}

func (r *fakeRepo) security(method string, in any) (int, []byte, error) {
	if method == "GET" {
		return jsonResponse(200, map[string]any{"security_and_analysis": map[string]any{
			"secret_scanning":                 map[string]any{"status": r.secretScanning},
			"secret_scanning_push_protection": map[string]any{"status": r.pushProtection},
		}})
	}
	sa, _ := in.(map[string]any)["security_and_analysis"].(map[string]any)
	if s, ok := sa["secret_scanning"].(map[string]any); ok {
		r.secretScanning = s["status"].(string)
	}
	if s, ok := sa["secret_scanning_push_protection"].(map[string]any); ok {
		if r.secretScanning != "enabled" {
			return 422, []byte(`{"message":"Secret scanning must be enabled first"}`), nil
		}
		r.pushProtection = s["status"].(string)
	}
	return 200, []byte(`{}`), nil
}

func (r *fakeRepo) toggle(method, rest string, in any) (int, []byte, error) {
	put := method == "PUT"
	switch rest {
	case "vulnerability-alerts":
		if put {
			r.alerts = true
		}
		if r.alerts {
			return 204, nil, nil
		}
		return 404, []byte(`{"message":"Vulnerability alerts are disabled"}`), nil
	case "automated-security-fixes":
		if put && !r.alerts {
			return 422, []byte(`{"message":"Dependabot alerts must be enabled first"}`), nil
		}
		return onSwitch(&r.securityUpdates, put, map[string]any{"paused": false})
	case "private-vulnerability-reporting":
		return onSwitch(&r.reporting, put, nil)
	case "immutable-releases":
		return onSwitch(&r.immutable, put, map[string]any{"enforced_by_owner": false})
	case "actions/permissions":
		if put {
			m := in.(map[string]any)
			r.allowedActions = m["allowed_actions"].(string)
			r.shaPinning = m["sha_pinning_required"].(bool)
			return 204, nil, nil
		}
		return jsonResponse(200, map[string]any{
			"enabled": true, "allowed_actions": r.allowedActions, "sha_pinning_required": r.shaPinning,
		})
	}
	return notFound()
}

// onSwitch is a GET {"enabled": bool} / PUT-to-enable endpoint with extra GET fields.
func onSwitch(on *bool, put bool, extra map[string]any) (int, []byte, error) {
	if put {
		*on = true
		return 204, nil, nil
	}
	body := map[string]any{"enabled": *on}
	for k, v := range extra {
		body[k] = v
	}
	return jsonResponse(200, body)
}

func (r *fakeRepo) ruleset(method, rest string, in any, source string) (int, []byte, error) {
	if rest == "rulesets" {
		if method == "POST" {
			rs := storeRuleset(in, r.nextRulesetID, source)
			r.nextRulesetID++
			r.rulesets = append(r.rulesets, rs)
			return jsonResponse(201, rs)
		}
		list := []any{}
		for _, rs := range r.rulesets {
			list = append(list, map[string]any{
				"id": rs["id"], "name": rs["name"], "target": rs["target"],
				"enforcement": rs["enforcement"], "source_type": rs["source_type"],
			})
		}
		return jsonResponse(200, list)
	}

	id, _ := strconv.Atoi(strings.TrimPrefix(rest, "rulesets/"))
	for i, rs := range r.rulesets {
		if int(rs["id"].(float64)) != id {
			continue
		}
		if method == "PUT" {
			r.rulesets[i] = storeRuleset(in, id, source)
		}
		return jsonResponse(200, r.rulesets[i])
	}
	return notFound()
}

// storeRuleset keeps a ruleset the way GitHub returns it: with an id, source
// fields, and default parameters filled in on rules that were sent without any.
func storeRuleset(in any, id int, source string) map[string]any {
	rs := normalize(in).(map[string]any)
	rs["id"] = float64(id)
	rs["source"] = source
	rs["source_type"] = "Repository"
	rs["current_user_can_bypass"] = "always"
	for _, rule := range rs["rules"].([]any) {
		rule := rule.(map[string]any)
		switch rule["type"] {
		case "update":
			if rule["parameters"] == nil {
				rule["parameters"] = map[string]any{"update_allows_fetch_and_merge": false}
			}
		case "pull_request":
			rule["parameters"].(map[string]any)["allowed_merge_methods"] = []any{"merge", "squash", "rebase"}
		case "required_status_checks":
			for _, c := range rule["parameters"].(map[string]any)["required_status_checks"].([]any) {
				c.(map[string]any)["integration_id"] = nil
			}
		}
	}
	return rs
}

func jsonResponse(status int, v any) (int, []byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return status, b, nil
}

func notFound() (int, []byte, error) {
	return 404, []byte(`{"message":"Not Found"}`), nil
}
