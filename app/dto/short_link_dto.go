package dto

// AdminDownloadShortLinksByScenarioNameRegexRequest defines input for downloading Excel filtered by scenario_name regex
// Example: scenario_name_regex: ".*sahel_11.*"
type AdminDownloadShortLinksByScenarioNameRegexRequest struct {
	ScenarioNameRegex string `json:"scenario_name_regex" validate:"required"`
}
