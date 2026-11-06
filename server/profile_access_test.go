package server

import (
	"github.com/afterglow-labs/afterglow-cybersecurity/agent"
	"github.com/afterglow-labs/afterglow-cybersecurity/db"
	"testing"
)

func TestProfileAccessProgramReachesProviderConfiguration(t *testing.T) {
	s := &Server{chatGPTDir: t.TempDir()}
	defer func() {
		if s.chatGPT != nil {
			_ = s.chatGPT.Close()
		}
	}()
	for _, mode := range []string{"api_key", "chatgpt_plan"} {
		cfg := agent.Config{}
		p := &db.LLMProfile{AuthMode: mode, Model: "gpt-6-sol", AccessProgramCyber: "daybreak_blue"}
		s.configureChatGPT(&cfg, p)
		if cfg.AccessProgramCyber != "daybreak_blue" {
			t.Fatalf("%s lost the saved access program", mode)
		}
	}
}
