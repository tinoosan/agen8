package app

import "testing"

func TestShouldEnableProtocolStdio(t *testing.T) {
	tests := []struct {
		name     string
		explicit bool
		inTTY    bool
		outTTY   bool
		want     bool
	}{
		{name: "explicit always enabled", explicit: true, inTTY: true, outTTY: true, want: true},
		{name: "piped stdin and stdout", explicit: false, inTTY: false, outTTY: false, want: true},
		{name: "interactive tty", explicit: false, inTTY: true, outTTY: true, want: false},
		{name: "mixed tty state", explicit: false, inTTY: true, outTTY: false, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldEnableProtocolStdio(tc.explicit, tc.inTTY, tc.outTTY)
			if got != tc.want {
				t.Fatalf("shouldEnableProtocolStdio()=%v want %v", got, tc.want)
			}
		})
	}
}
