import assert from "node:assert/strict";
import fs from "node:fs";
import test from "node:test";
import vm from "node:vm";
import ts from "typescript";

const source = fs.readFileSync(new URL("./auth.ts", import.meta.url), "utf8");
const { outputText } = ts.transpileModule(source, {
  compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
});
const token = `header.${Buffer.from(JSON.stringify({ sub: "Corey" })).toString("base64url")}.synthetic-signature`;

function fixture(reply) {
  const saved = new Map();
  const calls = [];
  const exports = {};
  const document = { cookie: "" };
  vm.runInNewContext(outputText, { exports, process: { env: {} }, window: {}, document,
    localStorage: { getItem: key => saved.get(key) ?? null, setItem: (key, value) => saved.set(key, value), removeItem: key => saved.delete(key) },
    atob, AbortController, setTimeout, clearTimeout,
    fetch: async (url, init) => { calls.push({ url, init }); return reply(); },
  });
  return { auth: exports.auth, saved, calls, document };
}

test("browser sign-in retains display metadata and never persists the bearer credential", () => {
  const { auth, saved, document } = fixture(() => { throw Error("unexpected network"); });
  auth.setToken(token);
  assert.equal(saved.has("afterglow_token"), false);
  assert.equal(auth.getCurrentUser().username, "Corey");
  assert.equal([...saved.values()].some(value => value.includes(token)), false);
  assert.equal(document.cookie, "");
});

test("legacy restoration exchanges header authentication for the server cookie without URL credentials", async () => {
  const { auth, saved, calls } = fixture(() => Response.json({ user: { username: "Afterglow" } }));
  saved.set("afterglow_token", token);
  assert.equal(await auth.restoreSession(), true);
  assert.equal(calls[0].url, "/api/v1/auth/restore");
  assert.equal(calls[0].init.headers.Authorization, `Bearer ${token}`);
  assert.equal(calls[0].init.credentials, "same-origin");
  assert.equal(saved.has("afterglow_token"), false);
  assert.equal(auth.getCurrentUser().username, "Afterglow");
});

test("a temporary server outage does not silently discard credentials or claim logout succeeded", async () => {
  const { auth, saved } = fixture(() => new Response("unavailable", { status: 503 }));
  auth.setToken(token);
  saved.set("afterglow_token", token);
  await assert.rejects(auth.restoreSession());
  assert.equal(saved.get("afterglow_token"), token);
  await assert.rejects(auth.logout());
  assert.equal(auth.getCurrentUser().username, "Corey");
  assert.equal(saved.get("afterglow_token"), token);
});

test("a rejected session clears both legacy credentials and stale user metadata", async () => {
  const { auth, saved } = fixture(() => new Response("expired", { status: 401 }));
  auth.setToken(token);
  saved.set("afterglow_token", token);
  assert.equal(await auth.restoreSession(), false);
  assert.equal(saved.size, 0);
  assert.equal(auth.getCurrentUser(), null);
});
