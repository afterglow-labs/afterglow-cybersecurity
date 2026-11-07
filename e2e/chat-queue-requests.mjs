import assert from "node:assert/strict";

// Provider requests contain system prompts and prior conversation history too.
// Queue execution is identified by the most recent user message in each call.
export function assertQueuedUserPromptOrder(requests, expected) {
  const prompts = requests.map(raw => {
    const messages = JSON.parse(raw);
    assert.ok(Array.isArray(messages), "captured provider messages must be an array");
    const content = messages.findLast(message => message.role === "user")?.content;
    if (typeof content === "string") return content;
    if (Array.isArray(content)) return content.filter(part => part.type === "text").map(part => part.text).join("\n");
    return "";
  });
  const positions = expected.map(text => prompts.findIndex(prompt => prompt === text || prompt.startsWith(text + "\n")));
  assert.ok(positions.every((position, index) => position >= 0 && (index === 0 || position > positions[index - 1])),
    `queued execution order changed: ${JSON.stringify({expected, positions})}`);
  assert.ok(!prompts.some(prompt => prompt.includes("lost-prompt-sentinel")), "discarded prompt replayed");
  return positions;
}
