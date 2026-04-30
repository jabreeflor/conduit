package localmodel

// Recommendation is static model metadata suitable for first-launch UI choices.
type Recommendation struct {
	Name          string
	Runtime       RuntimeKind
	MachineClass  string
	EstimatedSize string
	Description   string
}

// RecommendedModels returns the built-in model choices from PRD 7.3. The
// installer still requires a concrete artifact URL and checksum before download.
func RecommendedModels() []Recommendation {
	return []Recommendation{
		{
			Name:          "llama3.1-8b-instruct-q4",
			Runtime:       RuntimeOllama,
			MachineClass:  "entry",
			EstimatedSize: "5GB",
			Description:   "general-purpose local chat model for 8-16GB machines",
		},
		{
			Name:          "mistral-7b-instruct-q6",
			Runtime:       RuntimeLlamaCPP,
			MachineClass:  "mid-range",
			EstimatedSize: "6GB",
			Description:   "balanced quality and speed for 16-32GB machines",
		},
		{
			Name:          "qwen2.5-coder-32b-q4",
			Runtime:       RuntimeOllama,
			MachineClass:  "high-end",
			EstimatedSize: "20GB",
			Description:   "code-focused local model for high-memory machines",
		},
	}
}
