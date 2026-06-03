package domain

import (
	"fmt"
	"strings"

	pkgtypes "github.com/tinoosan/agen8-mcp-server/pkg/types"
)

type Category = pkgtypes.Category

type Urgency = pkgtypes.Urgency

const (
	CategoryFinancial      = pkgtypes.CategoryFinancial
	CategoryLegal          = pkgtypes.CategoryLegal
	CategoryContent        = pkgtypes.CategoryContent
	CategoryCode           = pkgtypes.CategoryCode
	CategoryGeneral        = pkgtypes.CategoryGeneral
	CategoryPhysical       = pkgtypes.CategoryPhysical
	CategoryCommunication  = pkgtypes.CategoryCommunication
	CategoryAdministrative = pkgtypes.CategoryAdministrative

	UrgencyLow      = pkgtypes.UrgencyLow
	UrgencyMedium   = pkgtypes.UrgencyMedium
	UrgencyHigh     = pkgtypes.UrgencyHigh
	UrgencyCritical = pkgtypes.UrgencyCritical
)

func ValidateCategory(category Category) error {
	return pkgtypes.ValidateCategory(pkgtypes.Category(category))
}

func ValidateUrgency(urgency Urgency) error {
	return pkgtypes.ValidateUrgency(pkgtypes.Urgency(urgency))
}

type Source string

const (
	SourceMember   Source = "member"
	SourceOperator Source = "operator"
)

func ValidateSource(source Source) error {
	switch strings.TrimSpace(string(source)) {
	case string(SourceMember), string(SourceOperator):
		return nil
	default:
		return fmt.Errorf("invalid source %q", source)
	}
}
