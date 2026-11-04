package api

import (
	"encoding/json"
	"net/http"
)

type ProblemDetails struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail"`
	Instance string `json:"instance,omitempty"`
}

func RespondError(w http.ResponseWriter, r *http.Request, status int, title, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)

	instance := ""
	if r != nil {
		instance = r.URL.Path
	}

	problem := ProblemDetails{
		Type:     "https://modulaengine.dev/errors",
		Title:    title,
		Status:   status,
		Detail:   detail,
		Instance: instance,
	}

	_ = json.NewEncoder(w).Encode(problem)
}

func RespondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
