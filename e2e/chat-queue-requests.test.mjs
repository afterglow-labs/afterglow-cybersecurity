import test from "node:test";
import assert from "node:assert/strict";
import {assertQueuedUserPromptOrder} from "./chat-queue-requests.mjs";

const call = (...messages) => JSON.stringify(messages);
const user = content => ({role:"user",content});
test("planner wording and prior history cannot impersonate an admitted prompt", () => {
  const requests = [
    call({role:"system",content:"Wait for five seconds."},user("This scheduled heartbeat finds no open intents.")),
    call(user("second"),{role:"assistant",content:"old history"},user("third revised")),
    call(user("third revised"),{role:"assistant",content:"complete"},user("second\n\n[Uploaded document]\nqueue-report.txt")),
  ];
  assert.deepEqual(assertQueuedUserPromptOrder(requests,["third revised","second"]),[1,2]);
});
test("reversed queue execution fails", () => {
  assert.throws(() => assertQueuedUserPromptOrder([call(user("second")),call(user("third revised"))],["third revised","second"]),/order changed/);
});
test("missing and prefix-only prompts fail", () => {
  assert.throws(() => assertQueuedUserPromptOrder([call(user("secondary"))],["second"]),/order changed/);
});
test("multipart user messages preserve order and discarded admissions fail", () => {
  assert.deepEqual(assertQueuedUserPromptOrder([call(user([{type:"text",text:"first"}]))],["first"]),[0]);
  assert.throws(() => assertQueuedUserPromptOrder([call(user("lost-prompt-sentinel"))],[]),/discarded prompt replayed/);
});
