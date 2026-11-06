package server

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/afterglow-labs/afterglow-cybersecurity/agent"
	"github.com/afterglow-labs/afterglow-cybersecurity/db"
)

type stallWatch struct {
	SteeredAt    int64
	LastObserved int64
}

func stallMinutes(name string, fallback, min, max int) int64 {
	value, err := strconv.Atoi(os.Getenv(name))
	if err != nil || value < min || value > max {
		return int64(fallback)
	}
	return int64(value)
}

// stallTick runs on the existing 30-second scheduler cadence. Only active
// work or an open LLM call can be stalled; an idle frontier is left alone.
func (e *Engine) stallTick() {
	if e == nil || e.m == nil || os.Getenv("ARTEX_ENABLE_STALL_WATCHDOG") == "false" {
		return
	}
	now := time.Now().Unix()
	threshold := stallMinutes("ARTEX_STALL_MIN", 15, 5, 120) * 60
	grace := stallMinutes("ARTEX_STALL_GRACE_MIN", 10, 2, 60) * 60
	for _, task := range e.m.List() {
		state := task.lifecycleSnapshot()
		if state.Queued || state.Paused || isTerminalStatus(state.Status) || e.IsDeleting(task.ID) {
			e.clearStall(task.ID)
			continue
		}
		lastRaw, ok := e.lastAct.Load(task.ID)
		if !ok {
			e.touch(task.ID)
			continue
		}
		last, ok := lastRaw.(int64)
		if !ok || now-last < threshold {
			e.clearStall(task.ID)
			continue
		}
		ids := e.runningWorkIDs(task.ID)
		llmOpen := e.ActiveLLMCalls(task.ID) > 0
		if len(ids) == 0 && !llmOpen {
			e.clearStall(task.ID)
			continue
		}
		e.stallMu.Lock()
		if e.stallState == nil {
			e.stallState = make(map[string]stallWatch)
		}
		watch := e.stallState[task.ID]
		if watch.LastObserved != last {
			watch = stallWatch{LastObserved: last}
		}
		if watch.SteeredAt == 0 {
			watch.SteeredAt = now
			e.stallState[task.ID] = watch
			e.stallMu.Unlock()
			for _, id := range ids {
				_ = e.SteerWork(id, fmt.Sprintf("[stall watchdog] No activity for %d minutes. Record a concrete blocker or take the next authorized step within your assigned intent.", (now-last)/60))
			}
			e.watchdogActivity(task, fmt.Sprintf("[watchdog] task idle for %d minutes; nudged %d running work(s)", (now-last)/60, len(ids)))
			continue
		}
		if now-watch.SteeredAt < grace {
			e.stallMu.Unlock()
			continue
		}
		watch.SteeredAt = now
		e.stallState[task.ID] = watch
		e.stallMu.Unlock()
		if len(ids) > 0 {
			for _, id := range ids {
				e.stopStalledWork(id)
			}
			e.watchdogActivity(task, fmt.Sprintf("[watchdog] stopped %d stalled work(s); planner will reassess", len(ids)))
		} else if llmOpen {
			e.cancelExec(task.ID, agent.AbortStallWatchdog)
			e.watchdogActivity(task, "[watchdog] cancelled stalled LLM round for retry")
		}
	}
}

func (e *Engine) watchdogActivity(task *Task, summary string) {
	record := db.Activity{Worker: "system", Kind: "text", Summary: summary}
	if id, err := e.appendActivity(task, record); err == nil {
		record.ID = id
		record.CreatedAt = time.Now()
		e.bc.Publish(task.ID, record)
	}
}

func (e *Engine) clearStall(taskID string) {
	e.stallMu.Lock()
	delete(e.stallState, taskID)
	e.stallMu.Unlock()
}

func (e *Engine) runningWorkIDs(taskID string) []int64 {
	e.workMu.Lock()
	defer e.workMu.Unlock()
	ids := make([]int64, 0)
	for id, run := range e.work {
		if run.task == taskID {
			ids = append(ids, id)
		}
	}
	return ids
}

func (e *Engine) stopStalledWork(intentID int64) {
	e.workMu.Lock()
	run := e.work[intentID]
	e.workMu.Unlock()
	if run != nil {
		run.cancel(agent.AbortStallWatchdog)
	}
}
