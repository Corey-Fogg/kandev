package models

// CoordinatorMetrics summarises a coordinator's delegated outcomes over a
// window. Rates and times are nil when there is nothing to measure.
type CoordinatorMetrics struct {
	Days                 int      `json:"days"`
	Since                string   `json:"since"`
	Delegated            int      `json:"delegated"`
	Completed            int      `json:"completed"`
	Failed               int      `json:"failed"`
	Truncated            bool     `json:"truncated,omitempty"`
	SuccessRate          *float64 `json:"success_rate"`
	MergedPullRequests   *int     `json:"merged_prs"`
	CycleTimeSamples     int      `json:"cycle_time_samples"`
	CycleTimeMedianHours *float64 `json:"cycle_time_median_hours"`
	CycleTimeP90Hours    *float64 `json:"cycle_time_p90_hours"`
	CostUSD              float64  `json:"cost_usd"`
	CostPerMergedPRUSD   *float64 `json:"cost_per_merged_pr_usd"`
	UnpricedEvents       int      `json:"unpriced_event_count"`
}
