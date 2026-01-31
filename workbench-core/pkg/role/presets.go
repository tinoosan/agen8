package role

import (
	"strings"
	"time"
)

var (
	General = Role{
		Name:        "General",
		Description: "General-purpose autonomous agent.",
		StandingGoals: []string{
			"Review inbox tasks and complete them.",
		},
		Triggers: []Trigger{
			{Type: "interval", Interval: 30 * time.Minute, Goal: "Check inbox and summarize progress so far."},
		},
	}.Normalize()

	StockResearcher = Role{
		Name:        "StockResearcher",
		Description: "Autonomous stock market research agent.",
		StandingGoals: []string{
			"Monitor market news and write summaries.",
			"Maintain a running research log of tickers mentioned in tasks.",
		},
		Triggers: []Trigger{
			{Type: "interval", Interval: 1 * time.Hour, Goal: "Scan top market news and write a short summary to /project."},
			{Type: "time_of_day", TimeOfDay: "09:30", Goal: "Market open analysis: summarize overnight developments and top movers."},
		},
	}.Normalize()

	SoftwareDeveloper = Role{
		Name:        "SoftwareDeveloper",
		Description: "Autonomous software engineering agent.",
		StandingGoals: []string{
			"Look for failing tests or linter errors and fix them.",
			"Keep documentation consistent with code changes.",
		},
		Triggers: []Trigger{
			{Type: "interval", Interval: 30 * time.Minute, Goal: "Scan repo for obvious issues (tests, build, lint) and fix what you can."},
		},
	}.Normalize()
)

func Get(name string) Role {
	if r, ok := getDefaultRole(name); ok {
		return r.Normalize()
	}
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "general", "default":
		return General
	case "stockresearcher", "stock_researcher", "stock-researcher", "researcher":
		return StockResearcher
	case "softwaredeveloper", "software_developer", "software-developer", "developer", "coder":
		return SoftwareDeveloper
	default:
		// Unknown roles fall back to General instead of failing startup.
		return General
	}
}
