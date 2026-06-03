package toolcontract

import "testing"

func TestResolveCanonicalName_UnknownAlias(t *testing.T) {
	if _, ok := ResolveCanonicalName("bash"); ok {
		t.Fatal("expected bash alias to be unresolved after shell_exec removal")
	}
}

func TestIsSystemTool_UnknownAlias(t *testing.T) {
	if IsSystemTool("bash") {
		t.Fatal("expected bash alias to not be treated as system tool")
	}
}
