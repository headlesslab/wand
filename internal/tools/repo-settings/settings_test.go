package main

import (
	"encoding/json"
	"testing"
)

func TestSubset(t *testing.T) {
	g := setup(t)

	// Objects: got may carry extra keys.
	g.True(subset(j(`{"a":1}`), j(`{"a":1,"b":2}`)))
	g.False(subset(j(`{"a":1,"b":2}`), j(`{"a":1}`)))
	g.False(subset(j(`{"a":1}`), j(`{"a":2}`)))

	// Arrays: same length, any order, elements matched as subsets.
	g.True(subset(j(`[{"t":"x"},{"t":"y"}]`), j(`[{"t":"y","p":1},{"t":"x"}]`)))
	g.False(subset(j(`[{"t":"x"}]`), j(`[{"t":"x"},{"t":"y"}]`)))
	g.False(subset(j(`[{"t":"x"},{"t":"x"}]`), j(`[{"t":"x"},{"t":"y"}]`)))
	g.True(subset(j(`[]`), j(`[]`)))

	// Scalars and type mismatches.
	g.True(subset(j(`true`), j(`true`)))
	g.False(subset(j(`1`), j(`"1"`)))
	g.False(subset(j(`{"a":1}`), j(`[1]`)))
	g.True(subset(j(`null`), j(`null`)))
	g.False(subset(j(`null`), j(`1`)))
}

func TestBypassActors(t *testing.T) {
	g := setup(t)

	g.Eq(bypassActors(0), []actor{{ID: 5, Type: "RepositoryRole", Mode: "always"}})
	g.Eq(bypassActors(123), []actor{
		{ID: 5, Type: "RepositoryRole", Mode: "always"},
		{ID: 123, Type: "Integration", Mode: "always"},
	})
}

func TestMainRuleset(t *testing.T) {
	g := setup(t)

	rs := normalize(mainRuleset(nil, bypassActors(0)))
	g.Eq(rs.(map[string]any)["name"], "main")
	g.Eq(rs.(map[string]any)["target"], "branch")
	g.Eq(rs.(map[string]any)["enforcement"], "active")
	g.Eq(ruleTypes(rs), []string{"deletion", "non_fast_forward", "pull_request"})
	g.True(subset(j(`{"conditions":{"ref_name":{"include":["~DEFAULT_BRANCH"],"exclude":[]}}}`), rs))
	g.True(subset(j(`{"bypass_actors":[{"actor_id":5,"actor_type":"RepositoryRole","bypass_mode":"always"}]}`), rs))
	g.True(subset(j(`{"rules":[{"type":"deletion"},{"type":"non_fast_forward"},
		{"type":"pull_request","parameters":{"required_approving_review_count":0}}]}`), rs))

	rs = normalize(mainRuleset([]string{"go / lint", "go / test (ubuntu-latest, floor)"}, bypassActors(7)))
	g.Eq(ruleTypes(rs), []string{"deletion", "non_fast_forward", "pull_request", "required_status_checks"})
	g.True(subset(j(`{"rules":[{"type":"deletion"},{"type":"non_fast_forward"},{"type":"pull_request"},
		{"type":"required_status_checks","parameters":{
			"strict_required_status_checks_policy":false,
			"required_status_checks":[{"context":"go / lint"},{"context":"go / test (ubuntu-latest, floor)"}]}}]}`), rs))
	g.True(subset(j(`{"bypass_actors":[
		{"actor_id":5,"actor_type":"RepositoryRole","bypass_mode":"always"},
		{"actor_id":7,"actor_type":"Integration","bypass_mode":"always"}]}`), rs))
}

func TestTagRuleset(t *testing.T) {
	g := setup(t)

	rs := normalize(tagRuleset(bypassActors(7)))
	g.Eq(rs.(map[string]any)["name"], "v*")
	g.Eq(rs.(map[string]any)["target"], "tag")
	g.Eq(rs.(map[string]any)["enforcement"], "active")
	g.Eq(ruleTypes(rs), []string{"creation", "update", "deletion"})
	g.True(subset(j(`{"conditions":{"ref_name":{"include":["refs/tags/v*"],"exclude":[]}}}`), rs))
	g.True(subset(j(`{"bypass_actors":[
		{"actor_id":5,"actor_type":"RepositoryRole","bypass_mode":"always"},
		{"actor_id":7,"actor_type":"Integration","bypass_mode":"always"}]}`), rs))
}

func TestSecurityAnalysisAbsent(t *testing.T) {
	g := setup(t)

	// A repository whose plan has no secret scanning reports the field absent.
	c := &client{api: stubAPI{status: 200, body: `{"security_and_analysis":{}}`}}
	current, ok, err := bundle(nil, 0)[0].read(c, "o/r")
	g.E(err)
	g.False(ok)
	g.Eq(current, "absent")
}

// j decodes a JSON literal the way responses are decoded.
func j(s string) any {
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		panic(err)
	}
	return v
}

// ruleTypes lists the rule types of a normalized ruleset in order.
func ruleTypes(rs any) []string {
	var types []string
	for _, r := range rs.(map[string]any)["rules"].([]any) {
		types = append(types, r.(map[string]any)["type"].(string))
	}
	return types
}
