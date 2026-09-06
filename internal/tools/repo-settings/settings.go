package main

import (
	"encoding/json"
	"fmt"
	"net/http"
)

const (
	enabled  = "enabled"
	disabled = "disabled"
)

// setting is one item of the bundle: how to read a repository's current value,
// which value the bundle wants, and how to write it.
type setting struct {
	name string
	want string
	// read returns the current value for the report and whether it already matches.
	read func(c *client, repo string) (current string, ok bool, err error)
	// write moves the repository to the wanted value.
	write func(c *client, repo string) error
}

// bundle is the settings every headlesslab repository carries, in an order
// GitHub accepts: push protection needs secret scanning first, and Dependabot
// security updates need the alerts first. appID is the GitHub App made a
// bypass actor of both rulesets, 0 while the App does not exist yet.
func bundle(checks []string, appID int) []setting {
	actors := bypassActors(appID)
	return []setting{
		securityAnalysis("secret scanning", "secret_scanning"),
		securityAnalysis("push protection", "secret_scanning_push_protection"),
		dependabotAlerts(),
		enabledFlag("Dependabot security updates", "automated-security-fixes"),
		enabledFlag("private vulnerability reporting", "private-vulnerability-reporting"),
		enabledFlag("immutable releases", "immutable-releases"),
		shaPinning(),
		ruleset(mainRuleset(checks, actors)),
		ruleset(tagRuleset(actors)),
	}
}

// securityAnalysis is one status field of the repository's
// security_and_analysis object, read from the repository and set with a PATCH.
func securityAnalysis(name, field string) setting {
	return setting{
		name: name,
		want: enabled,
		read: func(c *client, repo string) (string, bool, error) {
			var r struct {
				SecurityAndAnalysis map[string]struct {
					Status string `json:"status"`
				} `json:"security_and_analysis"`
			}
			if err := c.do(http.MethodGet, "repos/"+repo, nil, &r); err != nil {
				return "", false, err
			}
			status, ok := r.SecurityAndAnalysis[field]
			if !ok {
				return "absent", false, nil
			}
			return status.Status, status.Status == enabled, nil
		},
		write: func(c *client, repo string) error {
			body := map[string]any{"security_and_analysis": map[string]any{field: map[string]any{"status": enabled}}}
			return c.do(http.MethodPatch, "repos/"+repo, body, nil)
		},
	}
}

// dependabotAlerts is the one endpoint that answers 204 or 404 instead of a body.
func dependabotAlerts() setting {
	return setting{
		name: "Dependabot alerts",
		want: enabled,
		read: func(c *client, repo string) (string, bool, error) {
			on, err := c.exists("repos/" + repo + "/vulnerability-alerts")
			if err != nil {
				return "", false, err
			}
			return onOff(on), on, nil
		},
		write: func(c *client, repo string) error {
			return c.do(http.MethodPut, "repos/"+repo+"/vulnerability-alerts", nil, nil)
		},
	}
}

// enabledFlag is a repository endpoint that reports {"enabled": bool} on GET
// and switches on with an empty PUT.
func enabledFlag(name, endpoint string) setting {
	path := func(repo string) string { return "repos/" + repo + "/" + endpoint }
	return setting{
		name: name,
		want: enabled,
		read: func(c *client, repo string) (string, bool, error) {
			var r struct {
				Enabled bool `json:"enabled"`
			}
			if err := c.do(http.MethodGet, path(repo), nil, &r); err != nil {
				return "", false, err
			}
			return onOff(r.Enabled), r.Enabled, nil
		},
		write: func(c *client, repo string) error {
			return c.do(http.MethodPut, path(repo), nil, nil)
		},
	}
}

// shaPinning is the Actions policy that requires every action reference to be
// a full-length commit SHA. The repository's other Actions permissions are
// sent back as they are.
func shaPinning() setting {
	type permissions struct {
		Enabled            bool   `json:"enabled"`
		AllowedActions     string `json:"allowed_actions,omitempty"`
		SHAPinningRequired bool   `json:"sha_pinning_required"`
	}
	path := func(repo string) string { return "repos/" + repo + "/actions/permissions" }
	return setting{
		name: "Actions SHA pinning",
		want: "required",
		read: func(c *client, repo string) (string, bool, error) {
			var p permissions
			if err := c.do(http.MethodGet, path(repo), nil, &p); err != nil {
				return "", false, err
			}
			if p.SHAPinningRequired {
				return "required", true, nil
			}
			return "optional", false, nil
		},
		write: func(c *client, repo string) error {
			var p permissions
			if err := c.do(http.MethodGet, path(repo), nil, &p); err != nil {
				return err
			}
			p.SHAPinningRequired = true
			return c.do(http.MethodPut, path(repo), p, nil)
		},
	}
}

// ruleset creates or updates the repository ruleset named in want. The
// current ruleset matches when every field of want is present in it with the
// same value; the fields GitHub adds on its own (id, source, default rule
// parameters) do not count.
func ruleset(want map[string]any) setting {
	name, _ := want["name"].(string)
	desired := normalize(want)
	return setting{
		name: "ruleset " + name,
		want: "up to date",
		read: func(c *client, repo string) (string, bool, error) {
			id, current, err := findRuleset(c, repo, name)
			if err != nil {
				return "", false, err
			}
			switch {
			case id == 0:
				return "missing", false, nil
			case subset(desired, current):
				return "up to date", true, nil
			}
			return "differs", false, nil
		},
		write: func(c *client, repo string) error {
			id, _, err := findRuleset(c, repo, name)
			if err != nil {
				return err
			}
			if id == 0 {
				return c.do(http.MethodPost, "repos/"+repo+"/rulesets", want, nil)
			}
			return c.do(http.MethodPut, fmt.Sprintf("repos/%s/rulesets/%d", repo, id), want, nil)
		},
	}
}

// findRuleset returns the id and the full body of the repository's own
// ruleset of that name; id is 0 when there is none. Rulesets inherited from
// the organisation are listed by the same endpoint and skipped here.
func findRuleset(c *client, repo, name string) (int, map[string]any, error) {
	var summaries []struct {
		ID         int    `json:"id"`
		Name       string `json:"name"`
		SourceType string `json:"source_type"`
	}
	if err := c.do(http.MethodGet, "repos/"+repo+"/rulesets", nil, &summaries); err != nil {
		return 0, nil, err
	}
	for _, rs := range summaries {
		if rs.Name != name || rs.SourceType != "Repository" {
			continue
		}
		var full map[string]any
		if err := c.do(http.MethodGet, fmt.Sprintf("repos/%s/rulesets/%d", repo, rs.ID), nil, &full); err != nil {
			return 0, nil, err
		}
		return rs.ID, full, nil
	}
	return 0, nil, nil
}

// actor is one entry of a ruleset's bypass list.
type actor struct {
	ID   int    `json:"actor_id"`
	Type string `json:"actor_type"`
	Mode string `json:"bypass_mode"`
}

// adminRole is the id GitHub gives the repository admin role in bypass lists.
const adminRole = 5

// bypassActors lists who may bypass a ruleset: repository admins always, and
// the GitHub App once it exists.
func bypassActors(appID int) []actor {
	actors := []actor{{ID: adminRole, Type: "RepositoryRole", Mode: "always"}}
	if appID != 0 {
		actors = append(actors, actor{ID: appID, Type: "Integration", Mode: "always"})
	}
	return actors
}

// mainRuleset protects the default branch: no deletion, no force push, every
// change through a pull request, and the given status checks green before a
// merge. With no checks the status-check rule is left out, since GitHub
// rejects an empty list.
func mainRuleset(checks []string, actors []actor) map[string]any {
	rules := []any{
		rule("deletion", nil),
		rule("non_fast_forward", nil),
		rule("pull_request", map[string]any{
			"required_approving_review_count":   0,
			"dismiss_stale_reviews_on_push":     false,
			"require_code_owner_review":         false,
			"require_last_push_approval":        false,
			"required_review_thread_resolution": false,
		}),
	}
	if len(checks) > 0 {
		contexts := make([]any, 0, len(checks))
		for _, check := range checks {
			contexts = append(contexts, map[string]any{"context": check})
		}
		rules = append(rules, rule("required_status_checks", map[string]any{
			"strict_required_status_checks_policy": false,
			"do_not_enforce_on_create":             false,
			"required_status_checks":               contexts,
		}))
	}
	return map[string]any{
		"name":          "main",
		"target":        "branch",
		"enforcement":   "active",
		"bypass_actors": actors,
		"conditions":    refNames("~DEFAULT_BRANCH"),
		"rules":         rules,
	}
}

// tagRuleset restricts creating, moving and deleting release tags to the
// bypass actors, so a published tag never changes.
func tagRuleset(actors []actor) map[string]any {
	return map[string]any{
		"name":          "v*",
		"target":        "tag",
		"enforcement":   "active",
		"bypass_actors": actors,
		"conditions":    refNames("refs/tags/v*"),
		"rules": []any{
			rule("creation", nil),
			rule("update", nil),
			rule("deletion", nil),
		},
	}
}

func rule(typ string, parameters map[string]any) map[string]any {
	r := map[string]any{"type": typ}
	if parameters != nil {
		r["parameters"] = parameters
	}
	return r
}

func refNames(include ...string) map[string]any {
	return map[string]any{"ref_name": map[string]any{"include": include, "exclude": []string{}}}
}

// normalize round-trips v through JSON so that structs, ints and typed slices
// compare like a decoded response: objects as map[string]any, arrays as
// []any, numbers as float64.
func normalize(v any) any {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		panic(err)
	}
	return out
}

// subset reports whether everything in want is present in got: an object in
// got may carry extra keys, an array must have the same length with each
// element of want matching a distinct element of got in any order, and
// scalars must be equal.
func subset(want, got any) bool {
	switch w := want.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			return false
		}
		for k, wv := range w {
			gv, ok := g[k]
			if !ok || !subset(wv, gv) {
				return false
			}
		}
		return true
	case []any:
		g, ok := got.([]any)
		if !ok || len(g) != len(w) {
			return false
		}
		used := make([]bool, len(g))
		for _, wv := range w {
			if !claim(wv, g, used) {
				return false
			}
		}
		return true
	default:
		return want == got
	}
}

// claim marks the first unused element of got that want matches.
func claim(want any, got []any, used []bool) bool {
	for i, gv := range got {
		if !used[i] && subset(want, gv) {
			used[i] = true
			return true
		}
	}
	return false
}

func onOff(on bool) string {
	if on {
		return enabled
	}
	return disabled
}
