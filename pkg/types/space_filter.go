package types

// SpaceFilter specifies filtering and pagination for space queries.
type SpaceFilter struct {
	// Filtering
	TitleContains string // case-insensitive substring match against title/current goal
	ProjectID     string // exact match on space project_id (project-scoped spaces)
	IncludeSystem bool   // include daemon/system spaces in results

	// Pagination
	Limit  int // max results (default: 50, 0 = use default)
	Offset int // skip N spaces

	// Sorting
	SortBy   string // "updated_at" (default), "created_at", "title"
	SortDesc bool   // true = DESC (default), false = ASC
}
