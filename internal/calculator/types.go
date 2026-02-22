package calculator

// CalcRequest is the JSON body for binary operations (add, subtract, multiply, divide).
type CalcRequest struct {
	A float64 `json:"a"`
	B float64 `json:"b"`
}

// CalcResponse is the JSON response for all calculator endpoints.
type CalcResponse struct {
	Operation string  `json:"operation"`
	A         float64 `json:"a"`
	B         float64 `json:"b"`
	Result    float64 `json:"result"`
	RequestID string  `json:"request_id"`
}

// ChainStep describes a single step in a chained calculation.
type ChainStep struct {
	Op    string  `json:"op"`    // "add", "subtract", "multiply", "divide"
	Value float64 `json:"value"` // the operand applied with the running total
}

// ChainRequest is the JSON body for POST /calculator/chain.
type ChainRequest struct {
	Initial float64     `json:"initial"` // starting value
	Steps   []ChainStep `json:"steps"`
}

// ChainResponse is the JSON response for POST /calculator/chain.
type ChainResponse struct {
	Initial   float64       `json:"initial"`
	Steps     []ChainResult `json:"steps"`
	Result    float64       `json:"result"`
	RequestID string        `json:"request_id"`
}

// ChainResult records one executed step.
type ChainResult struct {
	Op     string  `json:"op"`
	Value  float64 `json:"value"`
	Result float64 `json:"result"`
}
