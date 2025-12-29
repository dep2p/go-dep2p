// Package debuglog 提供调试日志功能
package debuglog

import (
	"encoding/json"
	"os"
	"time"
)

// #region agent log

// Log writes a single NDJSON line to the debug log file provisioned by the Cursor runtime.
// NOTE: keep this tiny; ignore all errors (debug-only).
func Log(runId, hypothesisId, location, message string, data any) {
	type payload struct {
		SessionID    string `json:"sessionId"`
		RunID        string `json:"runId"`
		HypothesisID string `json:"hypothesisId"`
		Location     string `json:"location"`
		Message      string `json:"message"`
		Data         any    `json:"data,omitempty"`
		Timestamp    int64  `json:"timestamp"`
	}

	b, err := json.Marshal(payload{
		SessionID:    "debug-session",
		RunID:        runId,
		HypothesisID: hypothesisId,
		Location:     location,
		Message:      message,
		Data:         data,
		Timestamp:    time.Now().UnixMilli(),
	})
	if err != nil {
		return
	}

	// Debug log path - uses environment variable or falls back to local file.
	// This is debug-only and will be removed during instrumentation cleanup.
	logPath := os.Getenv("DEP2P_DEBUG_LOG")
	if logPath == "" {
		logPath = ".cursor/debug.log"
	}
	const mirrorPath = "debug.agent.ndjson"

	write := func(p string) {
		f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return
		}
		_, _ = f.Write(append(b, '\n'))
		_ = f.Close()
	}

	write(logPath)
	write(mirrorPath)
}
// #endregion agent log


