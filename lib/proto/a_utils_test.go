package proto_test

import (
	"regexp"
	"testing"

	"github.com/headlesslab/wand/lib/proto"
	"github.com/ysmood/got"
)

type T struct {
	got.G
}

func Test(t *testing.T) {
	got.Each(t, T{})
}

func (t T) PatternToReg() {
	t.Eq(``, proto.PatternToReg(""))
	t.Eq(`\A.*\z`, proto.PatternToReg("*"))
	t.Eq(`\A.?\z`, proto.PatternToReg("?"))
	t.Eq(`\Aa\z`, proto.PatternToReg("a"))
	t.Eq(`\Aa\.com/.*/test\z`, proto.PatternToReg("a.com/*/test"))
	t.Eq(`\A\?\*\z`, proto.PatternToReg(`\?\*`))
	t.Eq(`\Aa\.com\?a=10&b=\*\z`, proto.PatternToReg(`a.com\?a=10&b=\*`))
}

// PatternToRegLiterals: every character but the wildcards matches itself, a
// run of wildcards is one match, a "?" matches no character as well as one,
// and no pattern makes a regexp Go refuses (rod #982): the browser's own
// reading of the pattern, checked against Chrome 152's Fetch domain.
func (t T) PatternToRegLiterals() {
	t.Eq(`\A.*\.example\.com/.*\z`, proto.PatternToReg("**.example.com/**"))
	t.Eq(`\Aa\.com/\(1\)\z`, proto.PatternToReg("a.com/(1)"))
	t.Eq(`\A.?.?\z`, proto.PatternToReg("??"))
	t.Eq(`\Aa.*b\z`, proto.PatternToReg("a?*b"))
	t.Eq(`\Ad\z`, proto.PatternToReg(`\d`))
	t.Eq(`\A\\.*\z`, proto.PatternToReg(`\\*`))
	t.Eq(`\Aa\z`, proto.PatternToReg(`a\`))

	match := func(pattern, s string) bool {
		return regexp.MustCompile(proto.PatternToReg(pattern)).MatchString(s)
	}
	t.True(match("**.example.com/**", "https://sub.example.com/path?a=1"))
	t.False(match("a.com", "a-com"))
	t.True(match("a.com/(1)", "a.com/(1)"))
	t.True(match(`a\\*`, `a\anything`))
	t.True(match("a?*b", "ab"))
	t.True(match(`*://x.com/?`, `https://x.com/`))
	t.True(match(`*://x.com/?`, `https://x.com/1`))
	t.False(match(`*://x.com/?`, `https://x.com/12`))
	t.True(match(`*://x.com/??`, `https://x.com/é!`))
}
