package types

import "testing"

func TestValidateUrgency(t *testing.T) {
	t.Run("valid urgencies", func(t *testing.T) {
		for _, u := range ValidUrgencies {
			if err := ValidateUrgency(u); err != nil {
				t.Errorf("ValidateUrgency(%q) returned error: %v", u, err)
			}
		}
	})
	t.Run("invalid urgency returns error", func(t *testing.T) {
		invalids := []Urgency{"", "urgent", "LOW", "Medium", "unknown"}
		for _, u := range invalids {
			if err := ValidateUrgency(u); err == nil {
				t.Errorf("ValidateUrgency(%q) returned nil, want error", u)
			}
		}
	})
}

func TestValidateCategory(t *testing.T) {
	t.Run("valid categories", func(t *testing.T) {
		for _, c := range ValidCategories {
			if err := ValidateCategory(c); err != nil {
				t.Errorf("ValidateCategory(%q) returned error: %v", c, err)
			}
		}
	})
	t.Run("all 8 categories present", func(t *testing.T) {
		if len(ValidCategories) != 8 {
			t.Errorf("expected 8 valid categories, got %d", len(ValidCategories))
		}
	})
	t.Run("invalid category returns error", func(t *testing.T) {
		invalids := []Category{"", "Finance", "LEGAL", "tech", "other"}
		for _, c := range invalids {
			if err := ValidateCategory(c); err == nil {
				t.Errorf("ValidateCategory(%q) returned nil, want error", c)
			}
		}
	})
}
