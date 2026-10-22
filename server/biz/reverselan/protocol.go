package reverselan

const (
	APIVersion         = 1
	CapabilityHTTPJSON = "http-json-v1"
	LaneInteractive    = "interactive"
	LaneBackground     = "background"

	HeaderServerID   = "X-Orange-Server-Id"
	HeaderGeneration = "X-Orange-Generation"
)

type Capabilities struct {
	APIVersion   int      `json:"apiVersion"`
	Capabilities []string `json:"capabilities"`
	Lanes        []string `json:"lanes"`
}

type Request struct {
	APIVersion int               `json:"apiVersion"`
	RequestID  string            `json:"requestId"`
	Lane       string            `json:"lane"`
	Method     string            `json:"method"`
	Path       string            `json:"path"`
	Headers    map[string]string `json:"headers,omitempty"`
	Body       string            `json:"body,omitempty"`
}

type Result struct {
	APIVersion int               `json:"apiVersion"`
	RequestID  string            `json:"requestId"`
	Status     int               `json:"status"`
	Headers    map[string]string `json:"headers,omitempty"`
	Body       string            `json:"body,omitempty"`
}
