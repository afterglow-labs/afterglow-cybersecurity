package server

import (
	"net/http"
	"strconv"
)

func (s *Server) taskBusinessMap(w http.ResponseWriter, r *http.Request) {
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
	rows, err := s.m.pg.ListBusinessMap(taskID)
	if err != nil {
		writeErr(w, 500, "Could not list business model")
		return
	}
	writeJSON(w, 200, map[string]any{"business_map": rows})
}

func (s *Server) taskAttackChains(w http.ResponseWriter, r *http.Request) {
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
	rows, err := s.m.pg.ListChains(taskID)
	if err != nil {
		writeErr(w, 500, "Could not list attack chains")
		return
	}
	writeJSON(w, 200, map[string]any{"chains": rows})
}

func (s *Server) taskCampaignBranches(w http.ResponseWriter, r *http.Request) {
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
	rows, err := s.m.pg.ListCampaignBranches(taskID)
	if err != nil {
		writeErr(w, 500, "Could not list campaign branches")
		return
	}
	writeJSON(w, 200, map[string]any{"branches": rows})
}
