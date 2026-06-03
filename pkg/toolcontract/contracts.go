package toolcontract

import (
	"sort"
	"strings"
)

// Group identifies a reusable contract grouping for member-type/mode projections.
type Group string

const (
	GroupSystemAlways           Group = "system_always"
	GroupCoordinatorBase        Group = "coordinator_base"
	GroupCoordinatorWithWorkers Group = "coordinator_with_workers"
)

// MemberType is the normalized member type identifier used by tool contracts.
type MemberType string

const (
	MemberCoordinator     MemberType = "coordinator"
	MemberLoneCoordinator MemberType = "lone_coordinator"
	MemberWorker          MemberType = "worker"
	MemberReviewer        MemberType = "reviewer"
)

// MemberTypeVisibility declares which runtime member types may use a tool.
type MemberTypeVisibility struct {
	Coordinator     bool
	LoneCoordinator bool
	Worker          bool
	Reviewer        bool
}

func (v MemberTypeVisibility) Allows(memberType MemberType) bool {
	switch memberType {
	case MemberCoordinator:
		return v.Coordinator
	case MemberLoneCoordinator:
		return v.LoneCoordinator
	case MemberWorker:
		return v.Worker
	case MemberReviewer:
		return v.Reviewer
	default:
		return false
	}
}

// ModeVisibility declares mode-specific exposure information.
type ModeVisibility struct {
	Normal bool
}

// DiscoveryMetadata stores tool-discovery hints for docs/prompt/catalog layers.
type DiscoveryMetadata struct {
	Description string
	Tags        []string
}

// ToolContract is the single source-of-truth exposure contract for a tool.
type ToolContract struct {
	Name        string
	Aliases     []string
	System      bool
	MemberTypes MemberTypeVisibility
	Modes       ModeVisibility
	Groups      []Group
	Discovery   DiscoveryMetadata
}

// ContractProvider is a composable contract source.
type ContractProvider interface {
	Contracts() []ToolContract
}

// Selector allows custom contract filtering strategies.
type Selector interface {
	Select(contracts []ToolContract, criteria Criteria) []ToolContract
}

// CatalogRenderer allows pluggable catalog projections from selected contracts.
type CatalogRenderer interface {
	Render(contracts []ToolContract) []string
}

// Criteria controls contract selection.
type Criteria struct {
	MemberType     MemberType
	Group          Group
	SystemOnly     bool
	IncludeAliases bool
}

// StaticProvider is the default immutable provider.
type StaticProvider struct {
	contracts []ToolContract
}

func NewStaticProvider(contracts []ToolContract) StaticProvider {
	copied := make([]ToolContract, len(contracts))
	copy(copied, contracts)
	return StaticProvider{contracts: copied}
}

func (p StaticProvider) Contracts() []ToolContract {
	out := make([]ToolContract, len(p.contracts))
	copy(out, p.contracts)
	return out
}

// DefaultSelector performs deterministic contract filtering.
type DefaultSelector struct{}

func (DefaultSelector) Select(contracts []ToolContract, criteria Criteria) []ToolContract {
	out := make([]ToolContract, 0, len(contracts))
	for _, contract := range contracts {
		if criteria.Group != "" && !hasGroup(contract, criteria.Group) {
			continue
		}
		if criteria.SystemOnly && !contract.System {
			continue
		}
		if criteria.MemberType != "" && !contract.MemberTypes.Allows(criteria.MemberType) {
			continue
		}
		if !contract.Modes.Normal {
			continue
		}
		out = append(out, contract)
	}
	return out
}

var defaultProvider = NewStaticProvider(defaultContracts)
var defaultSelector DefaultSelector

// Contracts returns the full default contract set.
func Contracts() []ToolContract {
	return defaultProvider.Contracts()
}

// Select returns contracts matching criteria using the default provider/selector.
func Select(criteria Criteria) []ToolContract {
	return defaultSelector.Select(defaultProvider.Contracts(), criteria)
}

// Names returns deterministic tool names for selected contracts.
func Names(contracts []ToolContract) []string {
	names := make([]string, 0, len(contracts))
	seen := map[string]struct{}{}
	for _, contract := range contracts {
		name := strings.TrimSpace(contract.Name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// NamesFor returns deterministic names for the default contract set filtered by criteria.
func NamesFor(criteria Criteria) []string {
	return Names(Select(criteria))
}

// ResolveCanonicalName resolves a tool name/alias to its canonical contract name.
func ResolveCanonicalName(name string) (string, bool) {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return "", false
	}
	for _, contract := range defaultProvider.Contracts() {
		canonical := strings.TrimSpace(strings.ToLower(contract.Name))
		if canonical == "" {
			continue
		}
		if canonical == name {
			return contract.Name, true
		}
		for _, alias := range contract.Aliases {
			if strings.TrimSpace(strings.ToLower(alias)) == name {
				return contract.Name, true
			}
		}
	}
	return "", false
}

// IsSystemTool returns true when the name maps to a system tool contract.
func IsSystemTool(name string) bool {
	canonical, ok := ResolveCanonicalName(name)
	if !ok {
		return false
	}
	for _, contract := range defaultProvider.Contracts() {
		if contract.Name == canonical {
			return contract.System
		}
	}
	return false
}

func hasGroup(contract ToolContract, group Group) bool {
	for _, g := range contract.Groups {
		if g == group {
			return true
		}
	}
	return false
}

var allMemberTypes = MemberTypeVisibility{Coordinator: true, LoneCoordinator: true, Worker: true, Reviewer: true}
var coordinatorMemberTypes = MemberTypeVisibility{Coordinator: true, LoneCoordinator: true}

var defaultContracts = []ToolContract{
	{
		Name:        "operator",
		System:      true,
		MemberTypes: allMemberTypes,
		Modes:       ModeVisibility{Normal: true},
		Groups:      []Group{GroupSystemAlways},
		Discovery: DiscoveryMetadata{
			Description: "Operator-domain gateway for real-world human action requests.",
			Tags:        []string{"coordination"},
		},
	},
	{
		Name:        "decision",
		System:      true,
		MemberTypes: allMemberTypes,
		Modes:       ModeVisibility{Normal: true},
		Groups:      []Group{GroupSystemAlways},
		Discovery: DiscoveryMetadata{
			Description: "Decision-domain gateway for logging decisions and asking structured human questions.",
			Tags:        []string{"coordination", "audit", "human-input"},
		},
	},
	{
		Name:        "task",
		System:      true,
		MemberTypes: allMemberTypes,
		Modes:       ModeVisibility{Normal: true},
		Groups:      []Group{GroupSystemAlways},
		Discovery: DiscoveryMetadata{
			Description: "Task-domain gateway.",
			Tags:        []string{"tasks", "coordination"},
		},
	},
	{
		Name:        "space",
		System:      true,
		MemberTypes: allMemberTypes,
		Modes:       ModeVisibility{Normal: true},
		Groups:      []Group{GroupSystemAlways},
		Discovery: DiscoveryMetadata{
			Description: "Space coordination gateway.",
			Tags:        []string{"spaces", "coordination"},
		},
	},
	{
		Name:        "mission",
		System:      true,
		MemberTypes: allMemberTypes,
		Modes:       ModeVisibility{Normal: true},
		Groups:      []Group{GroupSystemAlways},
		Discovery: DiscoveryMetadata{
			Description: "Mission and key result gateway.",
			Tags:        []string{"missions", "okr"},
		},
	},
	{
		Name:        "plan",
		System:      true,
		MemberTypes: allMemberTypes,
		Modes:       ModeVisibility{Normal: true},
		Groups:      []Group{GroupSystemAlways},
		Discovery: DiscoveryMetadata{
			Description: "Structured plan gateway.",
			Tags:        []string{"planning"},
		},
	},
	{
		Name:        "tool",
		System:      true,
		MemberTypes: allMemberTypes,
		Modes:       ModeVisibility{Normal: true},
		Groups:      []Group{GroupSystemAlways},
		Discovery: DiscoveryMetadata{
			Description: "List or search callable tools.",
			Tags:        []string{"discovery"},
		},
	},
	{
		Name:        "heartbeat",
		System:      true,
		MemberTypes: coordinatorMemberTypes,
		Modes:       ModeVisibility{Normal: true},
		Groups:      []Group{GroupCoordinatorBase},
		Discovery: DiscoveryMetadata{
			Description: "Manage heartbeat schedules and trigger jobs.",
			Tags:        []string{"heartbeat"},
		},
	},
	{
		Name:        "metrics",
		System:      true,
		MemberTypes: coordinatorMemberTypes,
		Modes:       ModeVisibility{Normal: true},
		Groups:      []Group{GroupCoordinatorBase},
		Discovery: DiscoveryMetadata{
			Description: "Inspect runtime metrics.",
			Tags:        []string{"metrics"},
		},
	},
}
