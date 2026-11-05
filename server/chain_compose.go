package server

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/afterglow-labs/afterglow-cybersecurity/agent"
	"github.com/afterglow-labs/afterglow-cybersecurity/db"
)

type chainComposeState struct {
	Running    bool      `json:"running"`
	Added      int       `json:"added"`
	Error      string    `json:"error,omitempty"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
}

func (s *Server) taskChainComposeStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := canonicalTaskID(r.PathValue("id"))
	if !ok {
		writeErr(w, 400, "Invalid task ID")
		return
	}
	if _, exists := s.m.Task(id); !exists {
		writeErr(w, 404, "Task not found")
		return
	}
	s.chainComposeMu.Lock()
	state := s.chainCompose[id]
	s.chainComposeMu.Unlock()
	writeJSON(w, 200, state)
}

func (s *Server) composeTaskChains(w http.ResponseWriter, r *http.Request) {
	id, ok := canonicalTaskID(r.PathValue("id"))
	if !ok {
		writeErr(w, 400, "Invalid task ID")
		return
	}
	task, exists := s.m.Task(id)
	if !exists {
		writeErr(w, 404, "Task not found")
		return
	}
	taskID, _ := strconv.ParseInt(id, 10, 64)
	findings, err := s.m.pg.ConfirmedChainFindings(taskID)
	if err != nil {
		writeErr(w, 500, "Could not inspect confirmed findings")
		return
	}
	if len(findings) < 2 {
		writeErr(w, 400, "At least two confirmed findings are needed to compose a chain")
		return
	}
	s.chainComposeMu.Lock()
	if s.chainCompose == nil {
		s.chainCompose = make(map[string]chainComposeState)
	}
	if s.chainCompose[id].Running {
		s.chainComposeMu.Unlock()
		writeErr(w, 409, "Chain composition is already running")
		return
	}
	s.chainCompose[id] = chainComposeState{Running: true}
	s.chainComposeMu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(s.engine.execContextFor(s.ctx, id), 2*time.Minute)
		defer cancel()
		provider := &taskLLMRuntime{s: s, taskID: id, agentKey: "planner"}
		added, runErr := agent.RunChainSmith(ctx, provider, task.Store, taskID, func(a db.Activity) { s.engine.emitActivity(task, a) })
		state := chainComposeState{Added: added, FinishedAt: time.Now().UTC()}
		if runErr != nil {
			state.Error = "Chain composition failed; see task activity"
		}
		s.chainComposeMu.Lock()
		s.chainCompose[id] = state
		s.chainComposeMu.Unlock()
	}()
	writeJSON(w, http.StatusAccepted, chainComposeState{Running: true})
}
