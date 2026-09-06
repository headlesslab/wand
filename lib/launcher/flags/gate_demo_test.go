package flags_test

import "testing"

// TestGateDemoDeliberatelyRed exists only to show that a red Gate blocks the
// merge of pull request #71 (#36's acceptance criteria). Reverted before the
// pull request merges.
func TestGateDemoDeliberatelyRed(t *testing.T) {
	t.Fatal("deliberately red for the #36 Gate demonstration")
}
