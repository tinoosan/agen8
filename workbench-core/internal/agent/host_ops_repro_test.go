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

	// Case 2: Missing trailing @@ (should fail currently, but we want to allow it)
	patchNoTrailing := "@@ -2,1 +2,1\n-line2\n+new2\n"
	if _, err := applyUnifiedDiffStrict(oldText, patchNoTrailing); err != nil {
		t.Logf("Confirmed: Missing trailing @@ causes error: %v", err)
	} else {
		t.Errorf("Unexpected success with missing trailing @@")
	}

	// Case 3: Naked @@ (matches screenshot error?)
	patchNaked := "@@\n-line2\n+new2\n"
	if _, err := applyUnifiedDiffStrict(oldText, patchNaked); err != nil {
		t.Logf("Confirmed: Naked @@ causes error: %v", err)
	}
}
