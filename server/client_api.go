package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/afterglow-labs/afterglow-cybersecurity/chatstate"
)

const ClientAPIVersion = "1"

type clientConversationState struct {
	Cursor          string `json:"cursor"`
	Running         bool   `json:"running"`
	Paused          bool   `json:"paused"`
	HistoryRevision string `json:"history_revision"`
	ControlRevision string `json:"control_revision"`
	ContextChanging bool   `json:"context_changing"`
	ActiveRunID     string `json:"active_run_id,omitempty"`
}

func (s *Server) registerClientAPI(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/ready", s.clientReadiness)
	mux.HandleFunc("GET /api/v1/meta", s.clientMeta)
	mux.HandleFunc("GET /api/v1/capabilities", s.clientCapabilities)
	mux.HandleFunc("POST /api/v1/auth/login", s.authLogin)
	mux.HandleFunc("GET /api/v1/auth/session", s.authSession)
	mux.HandleFunc("POST /api/v1/auth/restore", s.authRestore)
	mux.HandleFunc("POST /api/v1/auth/logout", s.authLogout)
	mux.HandleFunc("POST /api/auth/logout", s.authLogout)
	mux.HandleFunc("GET /api/v1/conversations/{id}/events", s.clientConversationEvents)
	// Preserve existing cybersecurity DTOs and identifiers behind a versioned
	// route boundary. Authentication runs against the original v1 path first.
	mux.Handle("/api/v1/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target := r.Clone(r.Context())
		clonedURL := *r.URL
		target.URL = &clonedURL
		target.URL.Path = "/api/" + strings.TrimPrefix(r.URL.Path, "/api/v1/")
		target.URL.RawPath = ""
		mux.ServeHTTP(w, target)
	}))
}

func (s *Server) clientMeta(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"service": "aptex", "name": "APTEX", "publisher": "Afterglow Labs",
		"api_version": ClientAPIVersion, "server_version": BuildVersion, "api_base": "/api/v1",
		"auth": map[string]string{"scheme": "bearer", "login": "/api/v1/auth/login", "session": "/api/v1/auth/session",
			"restore": "/api/v1/auth/restore", "logout": "/api/v1/auth/logout"},
		"readiness": "/api/ready", "capabilities": "/api/v1/capabilities"})
}

func (s *Server) clientReadiness(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()
	ready := s.m != nil && s.m.pg != nil
	if ready {
		ready = s.m.pg.PingContext(ctx) == nil
	}
	status := http.StatusOK
	if !ready {
		status = http.StatusServiceUnavailable
	}
	// The listener is created only after mandatory recovery. Models and MCPs
	// are optional services, so their availability does not gate the API.
	writeJSON(w, status, map[string]any{"ready": ready, "service": "aptex", "api_version": ClientAPIVersion,
		"required": map[string]bool{"database": ready}, "optional": []string{"models", "mcp", "browser"}})
}

func (s *Server) clientCapabilities(w http.ResponseWriter, r *http.Request) {
	s.cfgMu.Lock()
	configured := s.llmOn
	s.cfgMu.Unlock()
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"api_version": ClientAPIVersion, "model_configured": configured,
		"features":      []string{"tasks", "findings", "evidence", "graphs", "workflows", "traffic", "interception", "conversations", "model_profiles", "chatgpt_plan"},
		"sessions":      map[string]any{"revocable": true, "credentials_in_urls": false},
		"events":        map[string]string{"conversations": "/api/v1/conversations/{id}/events", "resume_header": "Last-Event-ID", "cursor_type": "int64-string"},
		"chat_controls": map[string]any{"durable_inputs": true, "idempotent_requests": true, "revision_checked": true}})
}

func writeClientEvent(w http.ResponseWriter, kind, id string, value any) error {
	controller := http.NewResponseController(w)
	_ = controller.SetWriteDeadline(time.Now().Add(15 * time.Second))
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if id != "" {
		if _, err = fmt.Fprintf(w, "id: %s\n", id); err != nil {
			return err
		}
	}
	if _, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", kind, data); err != nil {
		return err
	}
	return controller.Flush()
}

// Reconnect reads the durable activity cursor. Disconnecting this observer
// never cancels an agent run or changes admission/queue state.
func (s *Server) clientConversationEvents(w http.ResponseWriter, r *http.Request) {
	pg, conversation, ok := s.convByID(w, r)
	if !ok {
		return
	}
	cursorText := r.Header.Get("Last-Event-ID")
	if cursorText == "" {
		cursorText = r.URL.Query().Get("cursor")
	}
	var cursor int64
	if cursorText != "" {
		var err error
		cursor, err = strconv.ParseInt(cursorText, 10, 64)
		if err != nil || cursor < 0 {
			writeErr(w, 400, "Invalid event cursor")
			return
		}
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	token := extractToken(r)
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	var previousState *clientConversationState
	for {
		claims, err := s.validateAdminSession(r.Context(), token)
		if err != nil {
			_ = writeClientEvent(w, "unavailable", "", map[string]string{"error": "Session storage unavailable"})
			return
		}
		if claims == nil {
			_ = writeClientEvent(w, "session_expired", "", map[string]bool{"reauthenticate": true})
			return
		}
		items, next, err := pg.ConvActivityListContext(r.Context(), conversation.ID, cursor, 256)
		if err != nil {
			_ = writeClientEvent(w, "unavailable", "", map[string]string{"error": "Conversation unavailable"})
			return
		}
		for _, item := range activityDTOs(items) {
			id := strconv.FormatInt(item.Seq, 10)
			if err = writeClientEvent(w, "activity", id, item); err != nil {
				return
			}
		}
		cursor = next
		if len(items) == 256 {
			continue
		}
		s.chatMu.Lock()
		running := s.chatBusy[s.convBusyKey(conversation.ID)]
		s.chatMu.Unlock()
		controls, controlErr := pg.ChatControlState(r.Context(), chatstate.SessionRef{ConversationID: conversation.ID})
		if controlErr != nil {
			_ = writeClientEvent(w, "unavailable", "", map[string]string{"error": "Conversation state unavailable"})
			return
		}
		state := clientConversationState{Cursor: strconv.FormatInt(cursor, 10), Running: running, Paused: controls.Paused,
			HistoryRevision: strconv.FormatInt(controls.HistoryRevision, 10), ControlRevision: strconv.FormatInt(controls.Revision, 10), ContextChanging: controls.ContextChanging}
		if controls.Active != nil {
			state.ActiveRunID = controls.Active.ID
			state.Running = state.Running || controls.Active.Active
		}
		if previousState == nil || *previousState != state {
			if err = writeClientEvent(w, "state", "", state); err != nil {
				return
			}
			previousState = &state
		}
		select {
		case <-r.Context().Done():
			return
		case <-s.ctx.Done():
			return
		case <-heartbeat.C:
			if err = writeClientEvent(w, "heartbeat", "", map[string]any{"cursor": strconv.FormatInt(cursor, 10)}); err != nil {
				return
			}
		case <-tick.C:
		}
	}
}
