package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // schedules keep IANA zones in the minimal runtime image

	"github.com/Autumn-27/artex/db"
)

type workflowReq struct {
	Name         string          `json:"name"`
	Description  *string         `json:"description"`
	AgentKey     string          `json:"agent_key"`
	Enabled      *bool           `json:"enabled"`
	ScheduleKind string          `json:"schedule_kind"`
	IntervalSec  *int            `json:"interval_sec"`
	DailyHour    *int            `json:"daily_hour"`
	DailyMinute  *int            `json:"daily_minute"`
	Timezone     string          `json:"timezone"`
	Prompt       *string         `json:"prompt"`
	GraphJSON    json.RawMessage `json:"graph_json"`
}

func defaultWorkflowGraph(name, scheduleKind, prompt, agentKey string, hour, minute, intervalSec int) string {
	schedLabel := "Manual"
	switch scheduleKind {
	case "daily":
		schedLabel = fmt.Sprintf("Daily %02d:%02d", hour, minute)
	case "interval":
		if intervalSec >= 3600 {
			schedLabel = fmt.Sprintf("Every %dh", intervalSec/3600)
		} else if intervalSec >= 60 {
			schedLabel = fmt.Sprintf("Every %dm", intervalSec/60)
		} else {
			schedLabel = fmt.Sprintf("Every %ds", intervalSec)
		}
	}
	type node struct {
		ID       string         `json:"id"`
		Type     string         `json:"type"`
		Position map[string]int `json:"position"`
		Data     map[string]any `json:"data"`
	}
	type edge struct {
		ID     string `json:"id"`
		Source string `json:"source"`
		Target string `json:"target"`
	}
	g := map[string]any{
		"nodes": []node{
			{ID: "trigger", Type: "workflow", Position: map[string]int{"x": 80, "y": 160}, Data: map[string]any{
				"kind": "trigger", "label": "Trigger", "subtitle": schedLabel,
			}},
			{ID: "agent", Type: "workflow", Position: map[string]int{"x": 360, "y": 160}, Data: map[string]any{
				"kind": "agent", "label": agentKey, "subtitle": "Run agent",
			}},
			{ID: "action", Type: "workflow", Position: map[string]int{"x": 640, "y": 160}, Data: map[string]any{
				"kind": "action", "label": "Do work", "subtitle": truncateRunes(prompt, 48),
			}},
		},
		"edges": []edge{
			{ID: "e1", Source: "trigger", Target: "agent"},
			{ID: "e2", Source: "agent", Target: "action"},
		},
		"name": name,
	}
	b, _ := json.Marshal(g)
	return string(b)
}

func truncateRunes(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func validateWorkflowFields(name, kind string, intervalSec, hour, minute int) string {
	if strings.TrimSpace(name) == "" {
		return "name is required"
	}
	if len(name) > 160 {
		return "name exceeds 160 bytes"
	}
	if kind == "" {
		kind = "manual"
	}
	switch kind {
	case "manual", "interval", "daily":
	default:
		return "schedule_kind must be manual, interval, or daily"
	}
	if kind == "interval" && intervalSec < 60 {
		return "interval_sec must be >= 60"
	}
	if kind == "daily" && (hour < 0 || hour > 23 || minute < 0 || minute > 59) {
		return "invalid daily_hour / daily_minute"
	}
	return ""
}

func validateWorkflowContent(pg *db.DB, w *db.Workflow) error {
	if len(w.Prompt) > 16000 || len(w.Description) > 4000 {
		return errors.New("workflow text exceeds size limit")
	}
	if w.AgentKey == "" {
		return errors.New("agent is required")
	}
	if a, err := pg.GetAgentByKey(w.AgentKey); err != nil || a == nil {
		return errors.New("selected agent does not exist")
	}
	if w.Timezone == "" {
		w.Timezone = "Local"
	}
	if _, err := time.LoadLocation(w.Timezone); err != nil {
		return errors.New("invalid IANA timezone")
	}
	if len(w.GraphJSON) > 128*1024 {
		return errors.New("workflow graph exceeds 128 KiB")
	}
	var graph struct {
		Nodes []json.RawMessage `json:"nodes"`
		Edges []json.RawMessage `json:"edges"`
	}
	if err := json.Unmarshal([]byte(w.GraphJSON), &graph); err != nil {
		return errors.New("invalid workflow graph JSON")
	}
	if len(graph.Nodes) > 128 || len(graph.Edges) > 256 {
		return errors.New("workflow graph has too many nodes or edges")
	}
	return nil
}

func (s *Server) pgListWorkflows(w http.ResponseWriter, r *http.Request) {
	pg := s.m.pg
	if pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}
	items, err := pg.ListWorkflows()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if items == nil {
		items = []*db.Workflow{}
	}
	writeJSON(w, 200, map[string]any{"workflows": items})
}

func (s *Server) pgGetWorkflow(w http.ResponseWriter, r *http.Request) {
	pg := s.m.pg
	if pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	item, err := pg.GetWorkflow(id)
	if err != nil {
		writeErr(w, 404, "workflow not found")
		return
	}
	writeJSON(w, 200, item)
}

func (s *Server) pgCreateWorkflow(w http.ResponseWriter, r *http.Request) {
	pg := s.m.pg
	if pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 192*1024)
	var req workflowReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, "bad json")
		return
	}
	kind := req.ScheduleKind
	if kind == "" {
		kind = "manual"
	}
	interval := 0
	if req.IntervalSec != nil {
		interval = *req.IntervalSec
	}
	hour, minute := 8, 0
	if req.DailyHour != nil {
		hour = *req.DailyHour
	}
	if req.DailyMinute != nil {
		minute = *req.DailyMinute
	}
	if msg := validateWorkflowFields(req.Name, kind, interval, hour, minute); msg != "" {
		writeErr(w, 400, msg)
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	agent := strings.TrimSpace(req.AgentKey)
	if agent == "" {
		agent = "auto"
	}
	prompt := ""
	if req.Prompt != nil {
		prompt = *req.Prompt
	}
	desc := ""
	if req.Description != nil {
		desc = *req.Description
	}
	graph := normalizeGraphJSON(req.GraphJSON)
	if len(req.GraphJSON) > 0 && graph == "" {
		writeErr(w, 400, "invalid workflow graph JSON")
		return
	}
	if graph == "" || graph == "{}" {
		graph = defaultWorkflowGraph(req.Name, kind, prompt, agent, hour, minute, interval)
	}
	input := &db.Workflow{
		Name:         strings.TrimSpace(req.Name),
		Description:  desc,
		AgentKey:     agent,
		Enabled:      enabled,
		ScheduleKind: kind,
		IntervalSec:  interval,
		DailyHour:    hour,
		DailyMinute:  minute,
		Timezone:     orDefault(req.Timezone, "Local"),
		Prompt:       prompt,
		GraphJSON:    graph,
	}
	if err := validateWorkflowContent(pg, input); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	item, err := pg.CreateWorkflow(input)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, item)
}

func (s *Server) pgUpdateWorkflow(w http.ResponseWriter, r *http.Request) {
	pg := s.m.pg
	if pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	cur, err := pg.GetWorkflow(id)
	if err != nil {
		writeErr(w, 404, "workflow not found")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 192*1024)
	var req workflowReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, "bad json")
		return
	}
	if strings.TrimSpace(req.Name) != "" {
		cur.Name = strings.TrimSpace(req.Name)
	}
	if req.Description != nil {
		cur.Description = *req.Description
	}
	if strings.TrimSpace(req.AgentKey) != "" {
		cur.AgentKey = strings.TrimSpace(req.AgentKey)
	}
	if req.Enabled != nil {
		cur.Enabled = *req.Enabled
	}
	if req.ScheduleKind != "" {
		cur.ScheduleKind = req.ScheduleKind
	}
	if req.IntervalSec != nil {
		cur.IntervalSec = *req.IntervalSec
	}
	if req.DailyHour != nil {
		cur.DailyHour = *req.DailyHour
	}
	if req.DailyMinute != nil {
		cur.DailyMinute = *req.DailyMinute
	}
	if req.Timezone != "" {
		cur.Timezone = req.Timezone
	}
	if req.Prompt != nil {
		cur.Prompt = *req.Prompt
	}
	if len(req.GraphJSON) > 0 && normalizeGraphJSON(req.GraphJSON) == "" {
		writeErr(w, 400, "invalid workflow graph JSON")
		return
	}
	if g := normalizeGraphJSON(req.GraphJSON); g != "" {
		cur.GraphJSON = g
	}
	if msg := validateWorkflowFields(cur.Name, cur.ScheduleKind, cur.IntervalSec, cur.DailyHour, cur.DailyMinute); msg != "" {
		writeErr(w, 400, msg)
		return
	}
	if err := validateWorkflowContent(pg, cur); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := pg.UpdateWorkflow(cur); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	updated, err := pg.GetWorkflow(id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, updated)
}

func (s *Server) pgDeleteWorkflow(w http.ResponseWriter, r *http.Request) {
	pg := s.m.pg
	if pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err := pg.DeleteWorkflow(id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"deleted": id})
}

func (s *Server) pgRunWorkflow(w http.ResponseWriter, r *http.Request) {
	pg := s.m.pg
	if pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	item, err := pg.GetWorkflow(id)
	if err != nil {
		writeErr(w, 404, "workflow not found")
		return
	}
	s.fireWorkflow(item, true)
	writeJSON(w, 200, map[string]any{"ok": true, "id": id})
}

func orDefault(s, d string) string {
	if strings.TrimSpace(s) == "" {
		return d
	}
	return s
}

// normalizeGraphJSON accepts either a JSON object or a JSON-encoded string blob.
func normalizeGraphJSON(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	if len(raw) > 128*1024 {
		return ""
	}
	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			if json.Valid([]byte(s)) && strings.HasPrefix(strings.TrimSpace(s), "{") {
				return strings.TrimSpace(s)
			}
			return ""
		}
	}
	if json.Valid(raw) && raw[0] == '{' {
		return strings.TrimSpace(string(raw))
	}
	return ""
}

func (s *Server) fireWorkflow(w *db.Workflow, manual bool) {
	if w == nil || s.m.pg == nil {
		return
	}
	msg := strings.TrimSpace(w.Prompt)
	if msg == "" {
		msg = fmt.Sprintf("Run workflow %q now.", w.Name)
	}
	header := fmt.Sprintf("[workflow:%d %s]", w.ID, w.Name)
	if manual {
		header += " (manual run)"
	}
	full := header + "\n\n" + msg
	title := "Workflow: " + w.Name
	s.StartTriggeredRun(w.AgentKey, title, full, 0, false, "", "")
	_ = s.m.pg.TouchWorkflowFire(w.ID)
}

func (s *Server) fireDueWorkflows() {
	pg := s.m.pg
	if pg == nil {
		return
	}
	items, err := pg.ListEnabledWorkflows()
	if err != nil {
		return
	}
	now := time.Now()
	for _, w := range items {
		loc, err := time.LoadLocation(w.Timezone)
		if err != nil {
			continue
		}
		localNow := now.In(loc)
		var threshold time.Time
		var due bool
		switch w.ScheduleKind {
		case "interval":
			if w.IntervalSec < 60 {
				continue
			}
			threshold = now.Add(-time.Duration(w.IntervalSec) * time.Second)
			due = w.LastFire == nil || w.LastFire.Before(threshold)
		case "daily":
			if localNow.Hour() != w.DailyHour || localNow.Minute() != w.DailyMinute {
				continue
			}
			threshold = time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, loc)
			due = w.LastFire == nil || w.LastFire.Before(threshold)
		}
		if !due {
			continue
		}
		claimed, err := pg.ClaimWorkflowFire(w.ID, now, threshold)
		if err != nil {
			continue
		}
		if claimed {
			s.fireWorkflow(w, false)
		}
	}
}
