package dto

// ListSmartTargetingTagsRequest carries normalized list filters from either a
// campaign-scoped or bundle-scoped Smart Targeting endpoint.
type ListSmartTargetingTagsRequest struct {
	CustomerID    uint
	BundleID      uint
	CampaignUUID  string
	Search        string
	SortBy        string
	SortDirection string
	Page          int
	PageSize      int
}

// SmartTargetingTagItem is one selectable tag row. Evaluated bundles expose
// the tag metadata and score explanation captured by their latest completed
// evaluation. Unevaluated bundles expose live tag metadata and nil evaluation
// fields. Nullable CTR values mean that those metrics are not yet available.
type SmartTargetingTagItem struct {
	TagID                 uint     `json:"tag_id"`
	// TagName               string   `json:"tag_name"`
	TagDisplayTitle       *string  `json:"tag_display_title"`
	// TagAudiencePersona    *string  `json:"tag_audience_persona"`
	TagCapacity           *int64   `json:"tag_capacity"`
	BundlePersonaFitScore *float64 `json:"bundle_persona_fit_score"`
	EvaluationRunID       *int64   `json:"evaluation_run_id"`
	FitLevel              *string  `json:"fit_level"`
	RelationType          *string  `json:"relation_type"`
	// Reason                *string  `json:"reason"`
	TestPhaseAvgCTR       *float64 `json:"test_phase_avg_ctr"`
	OverallAvgCTR         *float64 `json:"overall_avg_ctr"`
	Selected              bool     `json:"selected"`
}

// SmartTargetingSelectionSummary describes the complete selection, not only
// the tags visible on the current page.
type SmartTargetingSelectionSummary struct {
	SelectedTagCount    int64 `json:"selected_tag_count"`
	SelectedRawCapacity int64 `json:"selected_raw_capacity"`
}

// ListSmartTargetingTagsResponse combines the requested page with the complete
// selected-ID set so pagination never loses the customer's selection state.
type ListSmartTargetingTagsResponse struct {
	Items                  []SmartTargetingTagItem        `json:"items"`
	Pagination             PaginationInfo                 `json:"pagination"`
	SelectedTagIDs         []uint                         `json:"selected_tag_ids"`
	Summary                SmartTargetingSelectionSummary `json:"summary"`
	EvaluationAvailable    bool                           `json:"evaluation_available"`
	EffectiveSortBy        string                         `json:"effective_sort_by"`
	EffectiveSortDirection string                         `json:"effective_sort_direction"`
}

// ReplaceSmartTargetingSelectionRequest replaces the entire campaign
// selection. TagIDs must be unique and available in the bundle's effective
// source: its current score snapshot, or active live tags when no scores exist.
type ReplaceSmartTargetingSelectionRequest struct {
	CustomerID   uint   `json:"-"`
	CampaignUUID string `json:"-"`
	TagIDs       []uint `json:"tag_ids" validate:"required,min=1,max=10000,dive,min=1"`
}

// AutoSelectSmartTargetingTagsRequest selects the first Count tags from the
// complete normalized filter and sort order.
type AutoSelectSmartTargetingTagsRequest struct {
	CustomerID    uint   `json:"-"`
	CampaignUUID  string `json:"-"`
	Count         int    `json:"count" validate:"required,min=1,max=10000"`
	Search        string `json:"search,omitempty" validate:"omitempty,max=200"`
	SortBy        string `json:"sort_by,omitempty"`
	SortDirection string `json:"sort_direction,omitempty"`
}

// SmartTargetingSelectionResponse returns the complete persisted selection.
type SmartTargetingSelectionResponse struct {
	SelectedTagIDs []uint                         `json:"selected_tag_ids"`
	Summary        SmartTargetingSelectionSummary `json:"summary"`
}
