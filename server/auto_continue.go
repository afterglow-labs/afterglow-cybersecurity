package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/Autumn-27/artex/db"
)

const (
	autoContinueEnabledKey = "auto_continue_enabled"
	autoContinueDelayKey   = "auto_continue_delay_min"
	autoContinueBudgetKey  = "auto_continue_wave_timeout_min"
)

type autoContinuePolicy struct {
	Enabled            bool `json:"enabled"`
	DelayMinutes       int  `json:"delay_minutes"`
	WaveTimeoutMinutes int  `json:"wave_timeout_minutes"`
}

func (m *Manager) autoContinuePolicy() autoContinuePolicy {
	policy := autoContinuePolicy{DelayMinutes: 10, WaveTimeoutMinutes: 60}
	if m == nil || m.pg == nil {
		return policy
	}
	policy.Enabled = m.pg.GetBool(autoContinueEnabledKey, false)
	if value, ok, err := m.pg.GetSetting(autoContinueDelayKey); err == nil && ok {
		if n, err := strconv.Atoi(value); err == nil && n >= 1 && n <= 1440 {
			policy.DelayMinutes = n
		}
	}
	if value, ok, err := m.pg.GetSetting(autoContinueBudgetKey); err == nil && ok {
		if n, err := strconv.Atoi(value); err == nil && n >= 5 && n <= 480 {
			policy.WaveTimeoutMinutes = n
		}
	}
	return policy
}

func (m *Manager) setAutoContinuePolicy(policy autoContinuePolicy) error {
	if policy.DelayMinutes < 1 || policy.DelayMinutes > 1440 || policy.WaveTimeoutMinutes < 5 || policy.WaveTimeoutMinutes > 480 {
		return errors.New("delay must be 1–1440 minutes and wave timeout 5–480 minutes")
	}
	if err := m.pg.SetSetting(autoContinueDelayKey, strconv.Itoa(policy.DelayMinutes)); err != nil {
		return err
	}
	if err := m.pg.SetSetting(autoContinueBudgetKey, strconv.Itoa(policy.WaveTimeoutMinutes)); err != nil {
		return err
	}
	if err := m.pg.SetSetting(autoContinueEnabledKey, strconv.FormatBool(policy.Enabled)); err != nil {
		return err
	}
	if !policy.Enabled {
		return m.pg.CancelPendingAutoWaves()
	}
	return nil
}

func (m *Manager) scheduleResidualWave(taskID int64, status string) {
	if status != "done" && status != "timeout" {
		return
	}
	policy := m.autoContinuePolicy()
	if !policy.Enabled {
		return
	}
	t, ok := m.Task(strconv.FormatInt(taskID, 10))
	if !ok || t.Store == nil {
		return
	}
	openGoal, goalErr := t.Store.HasOpenGoal()
	activeIntent, intentErr := t.Store.HasActiveIntent()
	if goalErr != nil || intentErr != nil {
		log.Printf("[auto-continue] task %d residual check failed: %v", taskID, errors.Join(goalErr, intentErr))
		return
	}
	if status != "timeout" && !openGoal && !activeIntent {
		return
	}
	reason := "timeout"
	if status != "timeout" && openGoal {
		reason = "open-goals"
	} else if status != "timeout" {
		reason = "open-intents"
	}
	detail := fmt.Sprintf("Task %d stopped with status %s; open goals=%t, active intents=%t", taskID, status, openGoal, activeIntent)
	if _, err := m.pg.ScheduleAutoContinueWave(taskID, reason, detail, time.Now().Add(time.Duration(policy.DelayMinutes)*time.Minute)); err != nil {
		log.Printf("[auto-continue] task %d schedule failed: %v", taskID, err)
	}
}

func (s *Server) sweepAutoWaves(ctx context.Context) {
	if s.m == nil || s.m.pg == nil || !s.m.autoContinuePolicy().Enabled {
		return
	}
	for i := 0; i < 10 && ctx.Err() == nil; i++ {
		wave, err := s.m.pg.ClaimDueAutoContinueWave(time.Now())
		if err != nil {
			log.Printf("[auto-continue] claim failed: %v", err)
			return
		}
		if wave == nil {
			return
		}
		s.spawnAutoWave(ctx, wave)
	}
}

func (s *Server) spawnAutoWave(ctx context.Context, wave *db.AutoContinueWave) {
	if ctx.Err() != nil || s.engine.IsDeleting(strconv.FormatInt(wave.ParentTaskID, 10)) {
		_ = s.m.pg.DeferAutoWave(wave.ID, time.Minute)
		return
	}
	if existingID, err := s.m.pg.TaskIDForAutoWave(wave.ID); err != nil {
		log.Printf("[auto-continue] wave %d recovery lookup failed: %v", wave.ID, err)
		_ = s.m.pg.DeferAutoWave(wave.ID, time.Minute)
		return
	} else if existingID > 0 {
		if err := s.m.pg.MarkAutoWaveSpawned(wave.ID, existingID); err != nil {
			log.Printf("[auto-continue] wave %d recovery mark failed: %v", wave.ID, err)
		}
		return
	}
	parent, err := s.m.pg.GetTask(wave.ParentTaskID)
	if err != nil || parent == nil {
		_ = s.m.pg.CancelAutoWave(wave.ID)
		return
	}
	policy := s.m.autoContinuePolicy()
	if !policy.Enabled {
		_ = s.m.pg.CancelAutoWave(wave.ID)
		return
	}
	description := fmt.Sprintf("Follow-up wave %d for task %d: %s", wave.WaveNumber, wave.ParentTaskID, wave.Detail)
	goal := fmt.Sprintf("Re-verify inherited findings and finish the residual work from task %d. Broaden the test approach within the inherited authorized scope. Record evidence for new findings. Wave %d of %d.", wave.ParentTaskID, wave.WaveNumber, db.MaxAutoWaves)
	timeout := policy.WaveTimeoutMinutes * 60
	if parent.TimeoutSeconds > 0 && parent.TimeoutSeconds < timeout {
		timeout = parent.TimeoutSeconds
	}
	child, err := s.m.CreateTaskWithOptions(description, goal, db.TaskCreateOptions{
		Name:           fmt.Sprintf("afterglow-wave-%d", wave.ID),
		SourceTaskIDs:  []int64{wave.ParentTaskID},
		LLMProfileIDs:  parent.LLMProfileIDs,
		Language:       taskLanguage(s.m.pg, strconv.FormatInt(wave.ParentTaskID, 10)),
		TimeoutSeconds: timeout,
		AutoWaveID:     wave.ID,
	})
	if err != nil {
		log.Printf("[auto-continue] wave %d task creation failed: %v", wave.ID, err)
		_ = s.m.pg.DeferAutoWave(wave.ID, time.Minute)
		return
	}
	childID, _ := strconv.ParseInt(child.ID, 10, 64)
	if err := s.m.pg.MarkAutoWaveSpawned(wave.ID, childID); err != nil {
		log.Printf("[auto-continue] wave %d spawned task %d but mark failed: %v", wave.ID, childID, err)
	}
	s.launchTask(child, description+" "+goal, true)
}

func (s *Server) autoContinueSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		writeJSON(w, 200, s.m.autoContinuePolicy())
		return
	}
	var policy autoContinuePolicy
	if err := decode(r, &policy); err != nil {
		writeErr(w, 400, "Invalid follow-up policy")
		return
	}
	if err := s.m.setAutoContinuePolicy(policy); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, s.m.autoContinuePolicy())
}

func (s *Server) taskAutoWaves(w http.ResponseWriter, r *http.Request) {
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
	rows, err := s.m.pg.ListAutoWavesForTask(taskID)
	if err != nil {
		writeErr(w, 500, "Could not list follow-up waves")
		return
	}
	writeJSON(w, 200, map[string]any{"waves": rows})
}

func (s *Server) registerAutoContinueRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/auto-continue", s.autoContinueSettings)
	mux.HandleFunc("PUT /api/auto-continue", s.autoContinueSettings)
	mux.HandleFunc("GET /api/tasks/{id}/waves", s.taskAutoWaves)
}
