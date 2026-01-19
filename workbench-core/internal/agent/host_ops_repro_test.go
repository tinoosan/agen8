package agent

import (
	"testing"
)

func TestApplyUnifiedDiffStrict_Reproduction(t *testing.T) {
	// Case 1: Standard header (should pass)
	oldText := "line1\nline2\nline3\n"
	patchStandard := "@@ -2,1 +2,1 @@\n-line2\n+new2\n"
	if _, err := applyUnifiedDiffStrict(oldText, patchStandard); err != nil {
		t.Errorf("Standard patch failed: %v", err)
	}

	// Case 2: Missing trailing @@ (should be allowed)
	patchNoTrailing := "@@ -2,1 +2,1\n-line2\n+new2\n"
	if _, err := applyUnifiedDiffStrict(oldText, patchNoTrailing); err != nil {
		t.Errorf("Missing trailing @@ patch should be allowed, got error: %v", err)
	}

	// Case 3: Naked @@ should still fail
	patchNaked := "@@\n-line2\n+new2\n"
	if _, err := applyUnifiedDiffStrict(oldText, patchNaked); err == nil {
		t.Errorf("Expected error for naked @@ header")
	}
}
