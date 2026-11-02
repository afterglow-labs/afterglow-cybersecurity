package server

import (
	"net/http"
	"strconv"
)

func (s *Server) taskArtifacts(w http.ResponseWriter, r *http.Request) {
	id, ok := canonicalTaskID(r.PathValue("id"))
	if !ok {
		writeErr(w, 400, "Invalid task ID")
		return
	}
	if _, exists := s.m.Task(id); !exists {
		writeErr(w, 404, "Task not found")
		return
	}
	taskID, _ := strconv.ParseInt(id, 10, 64)
	rows, err := s.m.pg.ListTaskArtifacts(taskID)
	if err != nil {
		writeErr(w, 500, "Could not list task artifacts")
		return
	}
	writeJSON(w, 200, map[string]any{"artifacts": rows})
}
