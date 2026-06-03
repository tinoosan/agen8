package domain

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	humaninput "github.com/tinoosan/agen8-mcp-server/internal/services/humaninput/domain"
)

func newValidLog() Decision {
	return Decision{
		Title:      "Use PostgreSQL for analytics",
		ProjectID:  "proj-1",
		Source:     DecisionSourceAgent,
		Confidence: 0.85,
		CreatedAt:  time.Now().UTC(),
		Log:        &LogPayload{Rationale: "Better query performance for OLAP workloads"},
	}
}

func TestDecision_Validate(t *testing.T) {
	t.Run("valid log decision passes", func(t *testing.T) {
		d := newValidLog()
		if err := d.Validate(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("missing title", func(t *testing.T) {
		d := newValidLog()
		d.Title = "  "
		if err := d.Validate(); err == nil {
			t.Fatal("expected error for missing title")
		}
	})

	t.Run("missing rationale on log payload", func(t *testing.T) {
		d := newValidLog()
		d.Log = &LogPayload{Rationale: ""}
		if err := d.Validate(); err == nil {
			t.Fatal("expected error for missing rationale")
		}
	})

	t.Run("missing projectId", func(t *testing.T) {
		d := newValidLog()
		d.ProjectID = "   "
		if err := d.Validate(); err == nil {
			t.Fatal("expected error for missing projectId")
		}
	})

	t.Run("invalid source", func(t *testing.T) {
		d := newValidLog()
		d.Source = "unknown"
		if err := d.Validate(); err == nil {
			t.Fatal("expected error for invalid source")
		}
	})

	t.Run("policy source no longer valid", func(t *testing.T) {
		d := newValidLog()
		d.Source = "policy"
		if err := d.Validate(); err == nil {
			t.Fatal("expected error: policy source was removed")
		}
	})

	t.Run("source agent accepted", func(t *testing.T) {
		d := newValidLog()
		d.Source = DecisionSourceAgent
		if err := d.Validate(); err != nil {
			t.Fatalf("agent source should be valid: %v", err)
		}
	})

	t.Run("source operator with log accepted", func(t *testing.T) {
		d := newValidLog()
		d.Source = DecisionSourceOperator
		if err := d.Validate(); err != nil {
			t.Fatalf("operator + log should be valid: %v", err)
		}
	})

	t.Run("ask_user with no questions", func(t *testing.T) {
		d := Decision{
			Title:     "Need clarification",
			ProjectID: "proj-1",
			Source:    DecisionSourceAgent,
			CreatedAt: time.Now().UTC(),
			AskUser:   &AskUserPayload{},
		}
		if err := d.Validate(); err == nil {
			t.Fatal("expected error for ask_user with no questions")
		}
	})

	t.Run("both payloads set is rejected", func(t *testing.T) {
		d := newValidLog()
		d.AskUser = &AskUserPayload{Questions: []humaninput.Question{{ID: "q1", Text: "?"}}}
		if err := d.Validate(); err == nil {
			t.Fatal("expected error when both Log and AskUser are set")
		}
	})

	t.Run("no payload is rejected", func(t *testing.T) {
		d := newValidLog()
		d.Log = nil
		if err := d.Validate(); err == nil {
			t.Fatal("expected error when no payload is set")
		}
	})

	t.Run("confidence range", func(t *testing.T) {
		for _, c := range []float64{-0.1, 1.5, 99.0} {
			d := newValidLog()
			d.Confidence = c
			if err := d.Validate(); err == nil {
				t.Errorf("confidence %f should be invalid", c)
			}
		}
	})

	t.Run("confidence boundaries valid", func(t *testing.T) {
		for _, c := range []float64{0, 0.5, 1.0} {
			d := newValidLog()
			d.Confidence = c
			if err := d.Validate(); err != nil {
				t.Errorf("confidence %f should be valid: %v", c, err)
			}
		}
	})
}

func TestIsValidDecisionSource(t *testing.T) {
	tests := []struct {
		source DecisionSource
		valid  bool
	}{
		{DecisionSourceAgent, true},
		{DecisionSourceOperator, true},
		{"policy", false},
		{"unknown", false},
		{"", false},
		{"Agent", false},
		{"OPERATOR", false},
	}

	for _, tc := range tests {
		t.Run(string(tc.source), func(t *testing.T) {
			if got := IsValidDecisionSource(tc.source); got != tc.valid {
				t.Errorf("IsValidDecisionSource(%q) = %v, want %v", tc.source, got, tc.valid)
			}
		})
	}
}

func TestValidDecisionSources(t *testing.T) {
	if got := ValidDecisionSources(); len(got) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(got))
	}
}

func TestDecisionSource_Constants(t *testing.T) {
	if DecisionSourceAgent != "agent" {
		t.Errorf("DecisionSourceAgent = %q, want 'agent'", DecisionSourceAgent)
	}
	if DecisionSourceOperator != "operator" {
		t.Errorf("DecisionSourceOperator = %q, want 'operator'", DecisionSourceOperator)
	}
}

func TestDecision_Kind_EmptyOnNoPayload(t *testing.T) {
	d := Decision{}
	if k := d.Kind(); k != "" {
		t.Errorf("Kind() = %q, want empty", k)
	}
}

func TestDecision_JSONRoundTrip_Log(t *testing.T) {
	original := Decision{
		ID:         "dec-log-1",
		ProjectID:  "proj-1",
		SpaceID:    "space-1",
		Source:     DecisionSourceAgent,
		Title:      "Use PostgreSQL",
		Confidence: 0.9,
		CreatedAt:  time.Now().UTC().Truncate(time.Microsecond),
		Log: &LogPayload{
			Rationale:              "Faster OLAP",
			AlternativesRejected:   "ClickHouse",
			InvalidationConditions: []string{"latency > 1s"},
			Outcome:                "Deployed",
		},
	}
	bytes, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(bytes), `"log":{`) {
		t.Errorf("marshaled JSON missing nested log object: %s", bytes)
	}
	if !strings.Contains(string(bytes), `"rationale":"Faster OLAP"`) {
		t.Errorf("marshaled JSON missing rationale: %s", bytes)
	}
	var roundtrip Decision
	if err := json.Unmarshal(bytes, &roundtrip); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if roundtrip.Kind() != DecisionKindLog {
		t.Errorf("kind = %q, want log", roundtrip.Kind())
	}
	if roundtrip.Log == nil {
		t.Fatal("Log payload is nil after roundtrip")
	}
	if roundtrip.Log.Rationale != "Faster OLAP" || roundtrip.Log.AlternativesRejected != "ClickHouse" {
		t.Errorf("payload = %+v, fields not preserved", roundtrip.Log)
	}
	if roundtrip.AskUser != nil {
		t.Errorf("AskUser should be nil for log decision")
	}
}

func TestDecision_JSONRoundTrip_AskUser(t *testing.T) {
	original := Decision{
		ID:        "dec-ask-1",
		ProjectID: "proj-1",
		Source:    DecisionSourceAgent,
		Title:     "Need clarification",
		CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
		AskUser: &AskUserPayload{
			Context:   "About vendor selection",
			Questions: []humaninput.Question{{ID: "q1", Text: "Which vendor?"}},
		},
	}
	bytes, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(bytes), `"askUser":{`) {
		t.Errorf("marshaled JSON missing nested askUser object: %s", bytes)
	}
	var roundtrip Decision
	if err := json.Unmarshal(bytes, &roundtrip); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if roundtrip.Kind() != DecisionKindAskUser {
		t.Errorf("kind = %q, want ask_user", roundtrip.Kind())
	}
	if roundtrip.AskUser == nil {
		t.Fatal("AskUser payload is nil after roundtrip")
	}
	if len(roundtrip.AskUser.Questions) != 1 || roundtrip.AskUser.Questions[0].ID != "q1" {
		t.Errorf("questions not preserved: %+v", roundtrip.AskUser.Questions)
	}
}
