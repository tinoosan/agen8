package types

import "fmt"

// Urgency represents the priority level for escalations and operator actions.
// Shared across both bounded contexts per PRD decision D32.
type Urgency string

const (
	UrgencyLow      Urgency = "low"
	UrgencyMedium   Urgency = "medium"
	UrgencyHigh     Urgency = "high"
	UrgencyCritical Urgency = "critical"
)

// ValidUrgencies is the exhaustive set of valid urgency values.
var ValidUrgencies = []Urgency{
	UrgencyLow,
	UrgencyMedium,
	UrgencyHigh,
	UrgencyCritical,
}

// ValidateUrgency returns an error if the urgency value is not one of the known values.
func ValidateUrgency(u Urgency) error {
	switch u {
	case UrgencyLow, UrgencyMedium, UrgencyHigh, UrgencyCritical:
		return nil
	default:
		return fmt.Errorf("invalid urgency %q: must be one of low, medium, high, critical", u)
	}
}

// Category classifies the subject area of an escalation or operator action.
// 8 categories per PRD decision D28: original 5 + 3 new (physical, communication, administrative).
type Category string

const (
	CategoryFinancial      Category = "financial"
	CategoryLegal          Category = "legal"
	CategoryContent        Category = "content"
	CategoryCode           Category = "code"
	CategoryGeneral        Category = "general"
	CategoryPhysical       Category = "physical"
	CategoryCommunication  Category = "communication"
	CategoryAdministrative Category = "administrative"
)

// ValidCategories is the exhaustive set of valid category values.
var ValidCategories = []Category{
	CategoryFinancial,
	CategoryLegal,
	CategoryContent,
	CategoryCode,
	CategoryGeneral,
	CategoryPhysical,
	CategoryCommunication,
	CategoryAdministrative,
}

// ValidateCategory returns an error if the category value is not one of the known values.
func ValidateCategory(c Category) error {
	switch c {
	case CategoryFinancial, CategoryLegal, CategoryContent, CategoryCode,
		CategoryGeneral, CategoryPhysical, CategoryCommunication, CategoryAdministrative:
		return nil
	default:
		return fmt.Errorf("invalid category %q: must be one of financial, legal, content, code, general, physical, communication, administrative", c)
	}
}
