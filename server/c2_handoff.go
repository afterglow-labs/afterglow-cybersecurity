package server

import (
	"net/http"
	"strconv"
)

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
