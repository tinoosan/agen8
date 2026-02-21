package protocol

import "testing"

func TestTurnStatusFromTaskStatus(t *testing.T) {
	cases := []struct {
		in     string
		want   TurnStatus
		wantOK bool
	}{
		{in: "pending", want: TurnStatusPending, wantOK: true},
		{in: " active ", want: TurnStatusInProgress, wantOK: true},
		{in: "succeeded", want: TurnStatusCompleted, wantOK: true},
		{in: "success", want: TurnStatusCompleted, wantOK: true},
		{in: "completed", want: TurnStatusCompleted, wantOK: true},
		{in: "failed", want: TurnStatusFailed, wantOK: true},
		{in: "failure", want: TurnStatusFailed, wantOK: true},
		{in: "error", want: TurnStatusFailed, wantOK: true},
		{in: "canceled", want: TurnStatusCanceled, wantOK: true},
		{in: "cancelled", want: TurnStatusCanceled, wantOK: true},
		{in: "", want: "", wantOK: false},
		{in: "other", want: "", wantOK: false},
	}
	for _, tc := range cases {
		got, ok := turnStatusFromTaskStatus(tc.in)
		if ok != tc.wantOK {
			t.Fatalf("turnStatusFromTaskStatus(%q) ok=%v want %v", tc.in, ok, tc.wantOK)
		}
		if got != tc.want {
			t.Fatalf("turnStatusFromTaskStatus(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}
