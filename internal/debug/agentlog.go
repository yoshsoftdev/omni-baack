package debug

import (
	"encoding/json"
	"os"
	"time"
)

const debugLogPath = "/home/me/stag/omni-chan-main/.cursor/debug-3953ae.log"

// AgentLog appends one NDJSON debug line for this session.
func AgentLog(hypothesisID, location, message string, data map[string]any) {
	payload := map[string]any{
		"sessionId":    "3953ae",
		"hypothesisId": hypothesisID,
		"location":     location,
		"message":      message,
		"data":         data,
		"timestamp":    time.Now().UnixMilli(),
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return
	}
	f, err := os.OpenFile(debugLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	_, _ = f.Write(append(b, '\n'))
	_ = f.Close()
}
