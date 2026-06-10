package infra

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestNoGenericRemoteExecExported is a CI-enforced guard for the security
// model's "capability-scoped API, not generic exec" rule: the only exported
// Transport method that runs a remote command must be GitShowBaseline. If
// someone adds a generic Exec/Run/Shell method on Transport, this fails.
func TestNoGenericRemoteExecExported(t *testing.T) {
	src, err := os.ReadFile("local_filesystem.go")
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	// Match exported methods on Transport: `func (t Transport) Name(`.
	re := regexp.MustCompile(`func \(t Transport\) ([A-Z]\w*)\(`)
	banned := []string{"Exec", "Run", "Shell", "Command", "RunCommand", "SSH"}
	for _, m := range re.FindAllStringSubmatch(string(src), -1) {
		name := m[1]
		for _, b := range banned {
			if name == b || strings.HasPrefix(name, b+"C") || strings.HasSuffix(name, "Exec") {
				t.Fatalf("Transport exposes a generic-exec-shaped method %q; remote command execution must stay capability-scoped to GitShowBaseline", name)
			}
		}
	}
}
