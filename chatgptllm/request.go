package chatgptllm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/Autumn-27/norma/llm"
)

const reasoningPrefix = "artex-chatgpt-reasoning-v1:"

var validToolName = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

func wireToolName(name string) string {
	if validToolName.MatchString(name) {
		return name
	}
	sum := sha256.Sum256([]byte(name))
	return "tool_" + hex.EncodeToString(sum[:16])
}

func encodeRequest(cfg Config, req llm.CompletionRequest) ([]byte, map[string]string, error) {
	names := make(map[string]string, len(req.Tools))
	functions := make([]map[string]any, 0, len(req.Tools))
	for _, tool := range req.Tools {
		wire := wireToolName(tool.Name)
		if tool.Name == "" || names[wire] != "" {
			return nil, nil, failure("invalid_tool", "Tool names must be nonempty and unique.", nil)
		}
		names[wire] = tool.Name
		parameters := tool.InputSchema
		if parameters == nil {
			parameters = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		functions = append(functions, map[string]any{"type": "function", "name": wire, "description": tool.Description, "parameters": parameters, "strict": false})
	}
	input := make([]any, 0, len(req.Messages))
	images, err := prepareImageInputs(req.Messages)
	if err != nil {
		return nil, nil, err
	}
	imageIndex := 0
	nextImage := func() map[string]any {
		part := images[imageIndex]
		imageIndex++
		return part
	}
	for _, msg := range req.Messages {
		if msg.Role != llm.RoleUser && msg.Role != llm.RoleAssistant {
			return nil, nil, failure("unsupported_role", "Unsupported conversation role.", nil)
		}
		if len(msg.ProviderData) > 0 {
			items, err := replayItems(msg)
			if err != nil {
				return nil, nil, err
			}
			for _, item := range items {
				input = append(input, item)
			}
			continue
		}
		for _, block := range msg.Content {
			switch block.Type {
			case llm.BlockText:
				kind := "input_text"
				if msg.Role == llm.RoleAssistant {
					kind = "output_text"
				}
				input = append(input, map[string]any{"type": "message", "role": string(msg.Role), "content": []any{map[string]any{"type": kind, "text": block.Text}}})
			case llm.BlockImage:
				if msg.Role != llm.RoleUser {
					return nil, nil, unsupportedBlock(block.Type)
				}
				parts := []any{}
				if block.Image != nil && block.Image.Path != "" {
					parts = append(parts, map[string]any{"type": "input_text", "text": block.Image.Description()})
				}
				parts = append(parts, nextImage())
				input = append(input, map[string]any{"type": "message", "role": "user", "content": parts})
			case llm.BlockToolUse:
				if msg.Role != llm.RoleAssistant || block.ID == "" || block.Name == "" || !validArguments(block.Input) {
					return nil, nil, failure("invalid_tool_call", "Malformed tool call record.", nil)
				}
				input = append(input, map[string]any{"type": "function_call", "call_id": block.ID, "name": wireToolName(block.Name), "namespace": "artex", "arguments": string(block.Input)})
			case llm.BlockToolResult:
				if msg.Role != llm.RoleUser || block.ToolUseID == "" {
					return nil, nil, failure("invalid_tool_result", "Malformed tool result record.", nil)
				}
				var parts []string
				var visualParts []any
				hasImage := false
				if block.IsError {
					visualParts = append(visualParts, map[string]any{"type": "input_text", "text": "Tool execution error:"})
				}
				for _, content := range block.Content {
					switch content.Type {
					case llm.BlockText:
						parts = append(parts, content.Text)
						visualParts = append(visualParts, map[string]any{"type": "input_text", "text": content.Text})
					case llm.BlockImage:
						hasImage = true
						if content.Image != nil && content.Image.Path != "" {
							visualParts = append(visualParts, map[string]any{"type": "input_text", "text": content.Image.Description()})
						}
						visualParts = append(visualParts, nextImage())
					default:
						return nil, nil, unsupportedBlock(content.Type)
					}
				}
				output := strings.Join(parts, "\n")
				if block.IsError {
					output = "Tool execution error:\n" + output
				}
				var wireOutput any = output
				if hasImage {
					wireOutput = visualParts
				}
				input = append(input, map[string]any{"type": "function_call_output", "call_id": block.ToolUseID, "output": wireOutput})
			case llm.BlockThinking:
				// Context projection may intentionally discard opaque replay state
				// after pruning tools. A visible summary alone is not model state.
				if msg.Role == llm.RoleAssistant && block.Signature == "" {
					continue
				}
				if msg.Role != llm.RoleAssistant || !strings.HasPrefix(block.Signature, reasoningPrefix) {
					return nil, nil, failure("unsupported_reasoning", "This provider's reasoning history cannot be replayed to OpenAI. Start a new conversation.", nil)
				}
				raw := json.RawMessage(strings.TrimPrefix(block.Signature, reasoningPrefix))
				var item struct {
					Type string `json:"type"`
				}
				if json.Unmarshal(raw, &item) != nil || item.Type != "reasoning" {
					return nil, nil, failure("invalid_reasoning", "Malformed reasoning record.", nil)
				}
				input = append(input, raw)
			default:
				return nil, nil, unsupportedBlock(block.Type)
			}
		}
	}
	// Only API-key requests may carry token and sampling controls.
	body := map[string]any{"model": cfg.Model, "input": input, "store": false, "stream": true, "instructions": strings.Join(req.System, "\n\n")}
	if cfg.AccessProgramCyber != "" {
		body["access_programs"] = map[string]string{"cyber": cfg.AccessProgramCyber}
	}
	if cfg.APIKey != "" {
		if req.MaxTokens > 0 {
			body["max_output_tokens"] = req.MaxTokens
		}
		if req.Temperature != nil {
			body["temperature"] = *req.Temperature
		}
	}
	if cfg.APIKey == "" && auxiliaryRequest(req) {
		body["instructions"] = fmt.Sprintf("%s\n\nReturn a concise result within approximately %d tokens and %d UTF-8 bytes. Follow the required output format exactly; omit preambles and repeated source material.", body["instructions"], req.MaxTokens, auxiliaryByteLimit(req))
	}
	if len(functions) > 0 {
		body["tools"] = []any{map[string]any{"type": "namespace", "name": "artex", "description": "ARTEX local tools", "tools": functions}}
	}
	if req.Thinking != "disabled" && (cfg.APIKey == "" || cfg.ReasoningEffort != "") {
		reasoning := map[string]any{"summary": "auto"}
		if cfg.ReasoningEffort != "" {
			reasoning["effort"] = cfg.ReasoningEffort
		}
		body["reasoning"] = reasoning
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, nil, failure("invalid_request", "Could not serialize the request as JSON.", err)
	}
	return encoded, names, nil
}

// prepareImageInputs keeps recent visual evidence bounded without breaking
// tool-call pairing or serializing pixels as text. Older screenshots remain
// available through their artifact path and can be read again on demand.
// Missing artifacts become an actionable tool observation, not a permanent
// provider error that prevents the whole saved conversation from resuming.
func prepareImageInputs(messages []llm.Message) ([]map[string]any, error) {
	var sources []*llm.ImageSource
	for _, message := range messages {
		if len(message.ProviderData) > 0 {
			continue
		}
		for _, block := range message.Content {
			if block.Type == llm.BlockImage {
				sources = append(sources, block.Image)
			}
			if block.Type == llm.BlockToolResult {
				for _, child := range block.Content {
					if child.Type == llm.BlockImage {
						sources = append(sources, child.Image)
					}
				}
			}
		}
	}
	parts := make([]map[string]any, len(sources))
	count, encodedBytes := 0, 0
	for i := len(sources) - 1; i >= 0; i-- {
		source := sources[i]
		if source == nil {
			return nil, failure("invalid_image", "Image data is missing.", nil)
		}
		note := "An earlier image was omitted from this request. If needed, use Read to open its file again. " + source.Description()
		if count < llm.MaxImagesPerRequest {
			url, err := source.DataURL()
			if err != nil {
				if source.Path == "" {
					return nil, failure("invalid_image", err.Error(), nil)
				}
				note = "The image must be read or captured again: " + err.Error() + " " + source.Description()
			} else if encodedBytes+len(url) <= 32<<20 {
				parts[i] = map[string]any{"type": "input_image", "image_url": url, "detail": "auto"}
				count++
				encodedBytes += len(url)
				continue
			}
		}
		parts[i] = map[string]any{"type": "input_text", "text": note}
	}
	return parts, nil
}

func unsupportedBlock(kind llm.BlockType) error {
	return failure("unsupported_content", fmt.Sprintf("This content type is not supported by the current OpenAI connection: %q", kind), nil)
}

func validArguments(raw []byte) bool {
	var object map[string]json.RawMessage
	return json.Unmarshal(raw, &object) == nil && object != nil
}
