package orchestrator

type ChatRequest struct {
	Query     string `json:"query"`
	ImagePath string `json:"image_path,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	Uuid string `json:"Uuid,omitempty"`
}

type ChatResponse struct {
	Answer    string `json:"answer"`
	SessionID string `json:"session_id"`
}