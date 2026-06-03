package domain

type Question struct {
	ID             string   `json:"id"`
	Text           string   `json:"text"`
	Type           string   `json:"type"`
	Options        []string `json:"options,omitempty"`
	AllowFreeForm  bool     `json:"allowFreeForm,omitempty"`
	Recommendation string   `json:"recommendation,omitempty"`
	Blocking       bool     `json:"blocking,omitempty"`
}

type QuestionsPayload struct {
	Title     string     `json:"title,omitempty"`
	Context   string     `json:"context,omitempty"`
	Questions []Question `json:"questions"`
}

// Answer is the operator's response to a single Question. Exactly one
// of SelectedOption / SelectedOptions / FreeFormText is expected to be
// populated, depending on Question.Type:
//
//   - multiple_choice → SelectedOption (single string, required)
//   - multi_select    → SelectedOptions (array of strings, required, len>=1)
//   - free_form       → FreeFormText (string, required)
//
// FreeFormText may also accompany SelectedOption / SelectedOptions when
// the question allows a custom "Other" answer alongside the listed
// options. The wire format keeps single-select and multi-select in
// distinct fields so the cardinality is unambiguous to every consumer.
type Answer struct {
	QuestionID      string   `json:"questionId"`
	SelectedOption  *string  `json:"selectedOption,omitempty"`
	SelectedOptions []string `json:"selectedOptions,omitempty"`
	FreeFormText    *string  `json:"freeFormText,omitempty"`
}

type QuestionsResult struct {
	Cancelled bool     `json:"cancelled,omitempty"`
	Answers   []Answer `json:"answers,omitempty"`
}
