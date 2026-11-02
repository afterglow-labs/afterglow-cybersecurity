package server

import (
	"net/http"
	"strconv"
)

func (s *Server) huntClassTask(w http.ResponseWriter, r *http.Request) (*Task, int64, bool) {
	id, ok := canonicalTaskID(r.PathValue("id"))
	if !ok {
		writeErr(w, 400, "Invalid task ID")
		return nil, 0, false
	}
	task, exists := s.m.Task(id)
	if !exists {
		writeErr(w, 404, "Task not found")
		return nil, 0, false
	}
	taskID, _ := strconv.ParseInt(id, 10, 64)
	return task, taskID, true
}

func (s *Server) taskHuntClasses(w http.ResponseWriter, r *http.Request) {
	_, taskID, ok := s.huntClassTask(w, r)
	if !ok {
		return
	}
	rows, err := s.m.pg.ListHuntClasses(taskID)
	if err != nil {
		writeErr(w, 500, "Could not list hunt classes")
		return
	}
	writeJSON(w, 200, map[string]any{"hunt_classes": rows})
}

func (s *Server) enableTaskHuntClasses(w http.ResponseWriter, r *http.Request) {
	task, taskID, ok := s.huntClassTask(w, r)
	if !ok {
		return
	}
	if isTerminalStatus(task.lifecycleSnapshot().Status) {
		writeErr(w, 409, "Reopen the task before enabling hunt coverage")
		return
	}
	if err := s.m.pg.SeedHuntClasses(taskID); err != nil {
		writeErr(w, 500, "Could not enable hunt coverage")
		return
	}
	task.Notify()
	s.taskHuntClasses(w, r)
}

func (s *Server) updateTaskHuntClass(w http.ResponseWriter, r *http.Request) {
	task, taskID, ok := s.huntClassTask(w, r)
	if !ok {
		return
	}
	var input struct {
		Status    string `json:"status"`
		Reason    string `json:"reason"`
		MissTag   string `json:"miss_tag"`
		WidenNext string `json:"widen_next"`
	}
	if err := decode(r, &input); err != nil {
		writeErr(w, 400, "Invalid class outcome")
		return
	}
	class := r.PathValue("class")
	if err := s.m.pg.SetHuntClassStatus(taskID, class, input.Status, input.Reason, "", input.MissTag, input.WidenNext); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	rows, err := s.m.pg.ListHuntClasses(taskID)
	if err != nil {
		writeErr(w, 500, "Could not list hunt classes")
		return
	}
	task.Notify()
	writeJSON(w, 200, map[string]any{"hunt_classes": rows})
}
