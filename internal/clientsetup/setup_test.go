package clientsetup

import "testing"

func TestSetupScopeSelectsCredentialBoundary(t *testing.T) {
	tests := []struct {
		name      string
		requested string
		token     string
		want      string
		wantError bool
	}{
		{name: "account key defaults user", token: "ak_account", want: ScopeUser},
		{name: "project token defaults local", token: "wlt_project", want: ScopeLocal},
		{name: "explicit local account key", requested: ScopeLocal, token: "ak_account", want: ScopeLocal},
		{name: "project token cannot be global", requested: ScopeUser, token: "wlt_project", wantError: true},
		{name: "unknown scope", requested: "workspace", token: "ak_account", wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := setupScope(test.requested, test.token)
			if test.wantError {
				if err == nil {
					t.Fatalf("setupScope()=%q want error", got)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("setupScope()=(%q,%v) want (%q,nil)", got, err, test.want)
			}
		})
	}
}
