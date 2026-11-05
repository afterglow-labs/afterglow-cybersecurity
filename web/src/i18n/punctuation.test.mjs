import assert from "node:assert/strict";
import test from "node:test";
import { bracketLabel, closingQuote, listSeparator, openingQuote, questionMark, quoted, sentenceStop } from "./punctuation.ts";

test("English dynamic text uses English punctuation by default", () => {
  assert.equal(quoted("task", "en"), "“task”");
  assert.equal(listSeparator("en"), ", ");
  assert.equal(questionMark("en"), "?");
  assert.equal(sentenceStop("en"), ".");
  assert.equal(bracketLabel("Tool", "en"), "[Tool]");
});

test("translated quotes remain paired with the corresponding catalogs", () => {
  for (const locale of ["ja", "zh-Hans", "zh-Hant"]) {
    assert.equal(openingQuote(locale), "「");
    assert.equal(closingQuote(locale), "」");
    assert.equal(listSeparator(locale), "、");
  }
  assert.equal(quoted("tarea", "es"), "«tarea»");
  assert.equal(quoted("작업", "ko"), "“작업”");
});
