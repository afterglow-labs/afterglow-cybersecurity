package server

import (
	"net/http"
	"strconv"

	"github.com/afterglow-labs/afterglow-cybersecurity/db"
)

type c2HandoffSummary struct {
	ID           int64  `json:"id,string"`
	Vector       string `json:"vector"`
	TargetHost   string `json:"target_host"`
	TargetPath   string `json:"target_path"`
	BinaryArch   string `json:"binary_arch"`
	BinarySHA256 string `json:"binary_sha256"`
	Status       string `json:"status"`
}

func summarizeC2Handoffs(rows []db.C2Handoff) []c2HandoffSummary {
	summaries := make([]c2HandoffSummary, 0, len(rows))
	for _, row := range rows {
		summaries = append(summaries, c2HandoffSummary{
			ID: row.ID, Vector: row.Vector, TargetHost: row.TargetHost,
			TargetPath: row.TargetPath, BinaryArch: row.BinaryArch,
			BinarySHA256: row.BinarySHA256, Status: row.Status,
		})
	}
	return summaries
}

func (s *Server) c2TaskID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, ok := canonicalTaskID(r.PathValue("id"))
	if !ok {
		writeErr(w, 400, "Invalid task ID")
		return 0, false
	}
	if _, exists := s.m.Task(id); !exists {
		writeErr(w, 404, "Task not found")
		return 0, false
	}
	n, _ := strconv.ParseInt(id, 10, 64)
	return n, true
}

func (s *Server) taskC2Handoffs(w http.ResponseWriter, r *http.Request) {
	taskID, ok := s.c2TaskID(w, r)
	if !ok {
		return
	}
	rows, err := s.m.pg.ListC2Handoffs(taskID)
	if err != nil {
		writeErr(w, 500, "Could not list C2 handoffs")
		return
	}
	writeJSON(w, 200, map[string]any{"handoffs": rows})
}

// taskC2HandoffsSummary is the read-only view exposed to scoped APTEX clients.
// The full route above remains an administrator-only console feature.
func (s *Server) taskC2HandoffsSummary(w http.ResponseWriter, r *http.Request) {
	taskID, ok := s.c2TaskID(w, r)
	if !ok {
		return
	}
	rows, err := s.m.pg.ListC2Handoffs(taskID)
	if err != nil {
		writeErr(w, 500, "Could not list C2 handoffs")
		return
	}
	writeJSON(w, 200, map[string]any{"handoffs": summarizeC2Handoffs(rows)})
}

func (s *Server) c2HandoffStatus(w http.ResponseWriter, r *http.Request) {
	taskID, ok := s.c2TaskID(w, r)
	if !ok {
		return
	}
	handoffID, err := strconv.ParseInt(r.PathValue("handoff"), 10, 64)
	if err != nil || handoffID <= 0 {
		writeErr(w, 400, "Invalid handoff ID")
		return
	}
	var input struct {
		Status string `json:"status"`
	}
	if err := decode(r, &input); err != nil || (input.Status != "acknowledged" && input.Status != "cancelled") {
		writeErr(w, 400, "Status must be acknowledged or cancelled")
		return
	}
	changed, err := s.m.pg.SetC2HandoffStatus(taskID, handoffID, input.Status)
	if err != nil {
		writeErr(w, 500, "Could not update C2 handoff")
		return
	}
	if !changed {
		writeErr(w, 409, "Handoff is missing or already closed")
		return
	}
	row, err := s.m.pg.GetC2Handoff(taskID, handoffID)
	if err != nil {
		writeErr(w, 500, "Could not read C2 handoff")
		return
	}
	writeJSON(w, 200, row)
}
