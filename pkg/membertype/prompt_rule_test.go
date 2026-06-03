package membertype

import (
	"strings"
	"testing"
)

func TestJoinRuleLines_CombinesLines(t *testing.T) {
	t.Parallel()

	out := JoinRuleLines([]string{"- first", "- second"})

	if !strings.Contains(out, "- first") || !strings.Contains(out, "- second") {
		t.Fatalf("combined lines = %q", out)
	}
}
