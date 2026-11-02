package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Autumn-27/artex/agent"
	actool "github.com/Autumn-27/norma/tool"
)

const askOperatorMaxImage = 10 << 20

var askOperatorImageExt = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".gif": true}

// toolAskOperator lets a worker formally request operator input (captcha, OTP,
// 2FA, a decision): the question and an optional screenshot land in the intent's
// transcript as a user-role turn with an attachment card, and the intent pauses.
// The operator replies through the existing message-to-worker flow; the reply
// resumes the run with the answer as the next input (#41).
func (s *Server) toolAskOperator() actool.CoreTool {
	return wrTool("ask_operator",
		"Ask the human operator for input you cannot obtain yourself (a captcha solution, an email/SMS OTP code, a 2FA answer, or a decision). The question appears in this intent's chat for the operator, optionally with a screenshot, and this intent then pauses; the operator's reply resumes this run with the answer as your next input. Ask precisely, and only when you are genuinely blocked. For environmental blocks, describe the blocker and wait for the operator to decide how to proceed.",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"question": map[string]any{
					"type":        "string",
					"description": "What you need from the operator, precisely (e.g. 'enter the 6-digit code sent to x@y', 'solve the captcha in the attached screenshot'). For environmental blocks, explain the blocker and ask the operator how to proceed.",
				},
				"image_path": map[string]any{
					"type":        "string",
					"description": "Optional path to a screenshot (png/jpg/jpeg/webp/gif, max 10MB), e.g. the browser_take_screenshot output. It is copied into the task uploads and shown to the operator.",
				},
			},
			"required": []any{"question"},
		},
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			var req struct {
				Question  string `json:"question"`
				ImagePath string `json:"image_path"`
			}
			_ = json.Unmarshal(in, &req)
			q := strings.TrimSpace(req.Question)
			if q == "" {
				return actool.Errorf("Error: question is required"), nil
			}
			if len(q) > 4000 {
				return actool.Errorf("Error: question is too long"), nil
			}
			ri := agent.RunInfoFrom(ctx)
			if ri.TaskID == 0 || ri.IntentID == 0 {
				return actool.Errorf("Error: ask_operator can only be used inside a worker intent run"), nil
			}
			taskID := fmt.Sprintf("%d", ri.TaskID)
			t := s.m.ResolveTask(taskID)
			if t == nil {
				return actool.Errorf("Error: task not found"), nil
			}
			att, err := stageAskOperatorImage(s.m.dir, taskID, req.ImagePath)
			if err != nil {
				return actool.Errorf("Error: " + err.Error()), nil
			}
			text := "OPERATOR INPUT REQUESTED: " + q + " (this intent is pausing; reply here to resume it with your answer)"
			act := userActivityWithAttachments("worker", text, att)
			node := ri.IntentID
			act.NodeID = &node
			s.engine.emitActivity(t, act)
			// The control call runs on a detached context: pausing may cancel the
			// run's context, and the pause bookkeeping itself must complete.
			if _, err := s.applyIntentControl(context.Background(), t, ri.IntentID, "pause", firstLine(q, 120), ""); err != nil {
				return actool.Errorf("Error: question delivered but pausing the intent failed: " + err.Error()), nil
			}
			return actool.Text("Question delivered to the operator. This intent is pausing now: finish your current step, record everything you already know as facts, and stop. The operator's reply will resume this run and arrive as your next input."), nil
		})
}

// stageAskOperatorImage copies the referenced image into the task's uploads dir
// (the same place chat uploads land, so the transcript attachment card resolves)
// and returns the attachment descriptor. Empty src returns no attachment.
func stageAskOperatorImage(dataDir, taskID, src string) ([]chatAttachment, error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return nil, nil
	}
	ext := strings.ToLower(filepath.Ext(src))
	if !askOperatorImageExt[ext] {
		return nil, fmt.Errorf("image_path must be a png/jpg/jpeg/webp/gif file, got %q", filepath.Base(src))
	}
	if _, err := strconv.ParseInt(taskID, 10, 64); err != nil {
		return nil, fmt.Errorf("invalid task id")
	}
	taskDir, err := filepath.Abs(filepath.Join(dataDir, "tasks", taskID))
	if err != nil {
		return nil, fmt.Errorf("task directory not readable")
	}
	if !filepath.IsAbs(src) {
		src, err = filepath.Abs(src)
		if err != nil {
			return nil, fmt.Errorf("image not readable")
		}
	}
	rel, err := filepath.Rel(taskDir, src)
	if err != nil || !filepath.IsLocal(rel) || rel == "." {
		return nil, fmt.Errorf("image_path must be inside this task's directory")
	}
	base, err := os.OpenRoot(dataDir)
	if err != nil {
		return nil, fmt.Errorf("task directory not readable")
	}
	defer base.Close()
	root, err := base.OpenRoot(filepath.Join("tasks", taskID))
	if err != nil {
		return nil, fmt.Errorf("task directory not readable")
	}
	defer root.Close()
	in, err := root.Open(rel)
	if err != nil {
		return nil, fmt.Errorf("image not readable")
	}
	defer in.Close()
	st, err := in.Stat()
	if err != nil || !st.Mode().IsRegular() {
		return nil, fmt.Errorf("image_path must be a regular file")
	}
	if st.Size() > askOperatorMaxImage {
		return nil, fmt.Errorf("image too large (%s, max 10MB)", humanBytes(st.Size()))
	}
	var signature [512]byte
	n, err := in.Read(signature[:])
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("image not readable")
	}
	mediaType := http.DetectContentType(signature[:n])
	validType := map[string]string{".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".webp": "image/webp", ".gif": "image/gif"}
	if validType[ext] != mediaType {
		return nil, fmt.Errorf("image content type is not supported")
	}
	if _, err := in.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("image not readable")
	}
	if err := root.MkdirAll("uploads", 0o700); err != nil {
		return nil, err
	}
	uploads, err := root.OpenRoot("uploads")
	if err != nil {
		return nil, err
	}
	defer uploads.Close()
	var nonce [12]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, err
	}
	name := "ask-operator-" + hex.EncodeToString(nonce[:]) + ext
	out, err := uploads.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	defer out.Close()
	copied, err := io.Copy(out, io.LimitReader(in, askOperatorMaxImage+1))
	if err != nil || copied > askOperatorMaxImage {
		_ = uploads.Remove(name)
		return nil, fmt.Errorf("image copy failed or exceeded 10MB")
	}
	return []chatAttachment{{Name: name, Path: "uploads/" + name, Size: st.Size()}}, nil
}
