package chatgptllm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/Autumn-27/norma/llm"
)

// This data is part of the assistant message, so transcript persistence and
// compaction use the same lifetime as its visible content and tool calls.
type replayState struct {
	Version     int               `json:"version"`
	Provider    string            `json:"provider"`
	ContentHash string            `json:"content_hash"`
	Output      []json.RawMessage `json:"output"`
}

const replayProvider = "artex-chatgpt-responses"

func contentHash(content []llm.ContentBlock) string {
	b, _ := json.Marshal(content)
	hash := sha256.Sum256(b)
	return hex.EncodeToString(hash[:])
}

func attachReplayState(message *llm.Message, output []json.RawMessage) error {
	data, err := json.Marshal(replayState{Version: 1, Provider: replayProvider, ContentHash: contentHash(message.Content), Output: output})
	if err != nil {
		return failure("invalid_completion", "Could not save response state.", err)
	}
	message.ProviderData = data
	return nil
}

func replayItems(message llm.Message) ([]json.RawMessage, error) {
	var state replayState
	if message.Role != llm.RoleAssistant || json.Unmarshal(message.ProviderData, &state) != nil || state.Version != 1 || state.Provider != replayProvider || state.ContentHash != contentHash(message.Content) || len(state.Output) == 0 {
		return nil, failure("invalid_replay_state", "The saved OpenAI response state does not match the conversation. Start a new conversation.", nil)
	}
	ids := map[string]bool{}
	for _, raw := range state.Output {
		var item outputItem
		if json.Unmarshal(raw, &item) != nil || item.ID == "" || ids[item.ID] {
			return nil, failure("invalid_replay_state", "Malformed saved response item.", nil)
		}
		ids[item.ID] = true
		switch item.Type {
		case "message":
			if item.Role != "assistant" || !validPhase(item.Phase) {
				return nil, failure("invalid_replay_state", "Invalid saved response role or phase.", nil)
			}
		case "reasoning":
		case "function_call":
			if item.CallID == "" || item.Name == "" || item.Namespace != "artex" || !validArguments([]byte(item.Arguments)) {
				return nil, failure("invalid_replay_state", "Malformed saved tool call.", nil)
			}
		default:
			return nil, failure("invalid_replay_state", "Unsupported response state.", nil)
		}
	}
	return state.Output, nil
}

func validPhase(phase string) bool {
	return phase == "" || phase == "commentary" || phase == "final_answer"
}
