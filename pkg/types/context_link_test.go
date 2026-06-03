package types

import (
	"testing"
)

func TestContextLink_Validate_RequiredFields(t *testing.T) {
	tests := []struct {
		name    string
		link    ContextLink
		wantErr bool
	}{
		{
			name:    "empty link fails",
			link:    ContextLink{},
			wantErr: true,
		},
		{
			name: "missing sourceType",
			link: ContextLink{
				SourceID: "t-1", TargetType: "key_result", TargetID: "kr-1", EdgeType: EdgeTypeServes,
			},
			wantErr: true,
		},
		{
			name: "missing sourceId",
			link: ContextLink{
				SourceType: "task", TargetType: "key_result", TargetID: "kr-1", EdgeType: EdgeTypeServes,
			},
			wantErr: true,
		},
		{
			name: "missing targetType",
			link: ContextLink{
				SourceType: "task", SourceID: "t-1", TargetID: "kr-1", EdgeType: EdgeTypeServes,
			},
			wantErr: true,
		},
		{
			name: "missing targetId",
			link: ContextLink{
				SourceType: "task", SourceID: "t-1", TargetType: "key_result", EdgeType: EdgeTypeServes,
			},
			wantErr: true,
		},
		{
			name: "missing edgeType",
			link: ContextLink{
				SourceType: "task", SourceID: "t-1", TargetType: "key_result", TargetID: "kr-1",
			},
			wantErr: true,
		},
		{
			name: "all required fields present",
			link: ContextLink{
				SourceType: "task", SourceID: "t-1",
				TargetType: "key_result", TargetID: "kr-1",
				EdgeType: EdgeTypeServes, Confidence: 0.8,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.link.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestContextLink_Validate_ConfidenceRange(t *testing.T) {
	base := ContextLink{
		SourceType: "task", SourceID: "t-1",
		TargetType: "key_result", TargetID: "kr-1",
		EdgeType: EdgeTypeServes,
	}

	t.Run("negative confidence returns error", func(t *testing.T) {
		link := base
		link.Confidence = -0.5
		if err := link.Validate(); err == nil {
			t.Fatal("expected error for negative confidence")
		}
	})

	t.Run("above 1 confidence returns error", func(t *testing.T) {
		link := base
		link.Confidence = 1.5
		if err := link.Validate(); err == nil {
			t.Fatal("expected error for confidence > 1")
		}
	})

	t.Run("zero confidence is valid", func(t *testing.T) {
		link := base
		link.Confidence = 0
		if err := link.Validate(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("one confidence is valid", func(t *testing.T) {
		link := base
		link.Confidence = 1.0
		if err := link.Validate(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("mid-range confidence is valid", func(t *testing.T) {
		link := base
		link.Confidence = 0.75
		if err := link.Validate(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestContextLink_Validate_EdgeType(t *testing.T) {
	base := ContextLink{
		SourceType: "task", SourceID: "t-1",
		TargetType: "key_result", TargetID: "kr-1",
		Confidence: 0.8,
	}

	t.Run("unknown edge type returns error", func(t *testing.T) {
		link := base
		link.EdgeType = "invented_edge"
		if err := link.Validate(); err == nil {
			t.Fatal("expected error for unknown edge type")
		}
	})

	t.Run("empty edge type returns error", func(t *testing.T) {
		link := base
		link.EdgeType = ""
		if err := link.Validate(); err == nil {
			t.Fatal("expected error for empty edge type")
		}
	})

	// Verify all known edge types are accepted.
	knownTypes := []string{
		EdgeTypeBlockedBy,
		EdgeTypeResolvedBy,
		EdgeTypeCompletedBy,
		EdgeTypeServes,
		EdgeTypeInformedBy,
		EdgeTypeProduced,
		EdgeTypeMadeDuring,
		EdgeTypeSpawned,
		EdgeTypeRelatesTo,
	}
	for _, et := range knownTypes {
		t.Run("valid edge type: "+et, func(t *testing.T) {
			link := base
			link.EdgeType = et
			if err := link.Validate(); err != nil {
				t.Fatalf("unexpected error for known edge type %q: %v", et, err)
			}
		})
	}
}

func TestValidEdgeType(t *testing.T) {
	t.Run("returns true for known types", func(t *testing.T) {
		if !ValidEdgeType(EdgeTypeServes) {
			t.Error("expected serves to be valid")
		}
		if !ValidEdgeType(EdgeTypeBlockedBy) {
			t.Error("expected blocked_by to be valid")
		}
	})

	t.Run("returns false for unknown types", func(t *testing.T) {
		if ValidEdgeType("garbage") {
			t.Error("expected garbage to be invalid")
		}
		if ValidEdgeType("") {
			t.Error("expected empty string to be invalid")
		}
	})
}

func TestContextLink_Validate_AllFieldsCombined(t *testing.T) {
	t.Run("fully valid link with metadata", func(t *testing.T) {
		link := ContextLink{
			SourceType: "task",
			SourceID:   "t-1",
			TargetType: "key_result",
			TargetID:   "kr-1",
			EdgeType:   EdgeTypeServes,
			Confidence: 0.9,
			Metadata:   map[string]string{"reason": "direct contribution"},
			CreatedBy:  "member:planner",
		}
		if err := link.Validate(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("valid link with nil metadata", func(t *testing.T) {
		link := ContextLink{
			SourceType: "task",
			SourceID:   "t-1",
			TargetType: "key_result",
			TargetID:   "kr-1",
			EdgeType:   EdgeTypeServes,
			Confidence: 0.5,
		}
		if err := link.Validate(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
