package sparkdash

// Unit is a SparkDash registry entry as returned by GET /api/sparks.
type Unit struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type sparksListResponse struct {
	Sparks []Unit `json:"sparks"`
}

// Snapshot is the one-shot metrics payload from GET /api/sparks/:id/metrics.
type Snapshot struct {
	ID       string      `json:"id"`
	Name     string      `json:"name"`
	Online   bool        `json:"online"`
	LLMPort  int         `json:"llmPort"`
	LLMPorts []int       `json:"llmPorts"`
	Metrics  SnapMetrics `json:"metrics"`
}

// SnapMetrics is the metrics object inside a snapshot.
type SnapMetrics struct {
	LLM []LlmMetrics `json:"llm"`
}

// LlmMetrics is one LLM port's live probe result.
type LlmMetrics struct {
	Available          bool     `json:"available"`
	Backend            string   `json:"backend"`
	ModelID            *string  `json:"modelId"`
	SlotsActive        int      `json:"slotsActive"`
	SlotsTotal         int      `json:"slotsTotal"`
	GenerationTps      float64  `json:"generationTps"`
	PrefillTps         float64  `json:"prefillTps"`
	CachedPrefillTps   *float64 `json:"cachedPrefillTps"`
	UncachedPrefillTps *float64 `json:"uncachedPrefillTps"`
	KVCacheUsage       *float64 `json:"kvCacheUsage"`
	RequestsRunning    *int     `json:"requestsRunning"`
	RequestsWaiting    *int     `json:"requestsWaiting"`
	TTFTP95Seconds     *float64 `json:"ttftP95Seconds"`
	Error              string   `json:"error"`
}
