import assert from "node:assert/strict";
import test from "node:test";
import { defaultOpenAIAccess, isOpenAIEndpoint, openAIAccessProgram, openAIModelChoices, supportsDaybreakBlue } from "./openai-models.ts";

test("all requested models are selectable without discarding account catalog entries", () => {
  const choices = openAIModelChoices([{ id: "account-model", name: "Account model" }, { id: "gpt-6-sol", name: "Account Sol" }]);
  for (const model of ["gpt-6-astra", "gpt-6.1-sol", "gpt-6-sol", "gpt-5.6-sol", "gpt-daybreak-blue-latest", "account-model"]) {
    assert.ok(choices.some((choice) => choice.id === model), model);
  }
  assert.equal(choices.filter((choice) => choice.id === "gpt-6-sol").length, 1);
});

test("Blue flags are explicit; Astra and GPT-6.1 Sol always use Standard", () => {
  for (const model of ["gpt-6-sol", "gpt-5.6-sol", "gpt-daybreak-blue-latest"]) {
    assert.equal(supportsDaybreakBlue(model), true);
    assert.equal(openAIAccessProgram(model, "daybreak_blue"), "daybreak_blue");
    assert.equal(openAIAccessProgram(model, "standard"), "standard");
    assert.equal(openAIAccessProgram(model, "daybreak_red"), "standard");
  }
  for (const model of ["gpt-6-astra", "gpt-6.1-sol", "gpt-6-luna"]) assert.equal(openAIAccessProgram(model, "daybreak_blue"), "standard");
  assert.equal(defaultOpenAIAccess("gpt-daybreak-blue-latest"), "daybreak_blue");
  assert.equal(defaultOpenAIAccess("gpt-6-sol"), "standard");
  assert.equal(isOpenAIEndpoint("https://api.openai.com/v1"), true);
  assert.equal(isOpenAIEndpoint("http://local-compatible:8000/v1"), false);
});
