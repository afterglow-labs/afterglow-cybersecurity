import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import vm from "node:vm";

function loadWorker(fetch, storedState = {}) {
  const addListener = () => {};
  const chrome = {
    storage: {
      session: { get: async () => storedState, set: async (state) => Object.assign(storedState, state) },
      local: { get: async () => ({ serverUrl: "http://127.0.0.1:8787", token: "fixture" }) },
    },
    debugger: {
      onEvent: { addListener }, onDetach: { addListener },
      attach: async () => {}, sendCommand: async () => {}, detach: async () => {},
    },
    tabs: { query: async () => [{ id: 42 }] },
    action: { setBadgeText: () => {}, setBadgeBackgroundColor: () => {} },
    alarms: { onAlarm: { addListener }, create: () => {}, clear: async () => {} },
    runtime: { onMessage: { addListener } },
  };
  const context = vm.createContext({ chrome, fetch, URL, TextEncoder });
  vm.runInContext(readFileSync(new URL("./background.js", import.meta.url), "utf8"), context);
  return context;
}

test("manual flush drains every queued browser exchange", async () => {
  const batches = [];
  const context = loadWorker(async (_url, request) => {
    batches.push(JSON.parse(request.body).exchanges);
    return { ok: true, json: async () => ({ ingested: batches.at(-1).length, failed: 0 }) };
  });
  await vm.runInContext("ready", context);
  context.items = Array.from({ length: 23 }, (_, i) => ({ url: `https://lab.example.test/${i}` }));
  vm.runInContext("session.queue.push(...items)", context);
  await vm.runInContext("flushAll()", context);
  assert.deepEqual(batches.map((batch) => batch.length), [10, 10, 3]);
  assert.equal(vm.runInContext("session.queue.length", context), 0);
  assert.equal(vm.runInContext("session.ingested", context), 23);
});

test("capture token is sent only to HTTPS or loopback HTTP", () => {
  const context = loadWorker(async () => { throw new Error("unexpected fetch"); });
  for (const url of ["https://lab.example.test", "http://localhost:8787", "http://127.0.0.1:8787"]) {
    context.urlToCheck = url;
    assert.equal(vm.runInContext("validServerURL(urlToCheck)", context), true, url);
  }
  for (const url of ["http://lab.example.test", "ftp://lab.example.test", "https://user:pass@lab.example.test", "https://lab.example.test/path"]) {
    context.urlToCheck = url;
    assert.equal(vm.runInContext("validServerURL(urlToCheck)", context), false, url);
  }
});

test("failed upload stays queued for a later manual retry", async () => {
  let calls = 0;
  const context = loadWorker(async () => {
    calls += 1;
    if (calls === 1) throw new Error("offline");
    return { ok: true, json: async () => ({ ingested: 3, failed: 0 }) };
  });
  await vm.runInContext("ready", context);
  context.items = Array.from({ length: 3 }, (_, i) => ({ url: `https://lab.example.test/${i}` }));
  vm.runInContext("session.queue.push(...items)", context);
  await vm.runInContext("flushAll()", context);
  assert.equal(vm.runInContext("session.queue.length", context), 3);
  await vm.runInContext("flushAll()", context);
  assert.equal(vm.runInContext("session.queue.length", context), 0);
  assert.equal(calls, 2);
});

test("service worker restores active capture state", async () => {
  const context = loadWorker(async () => { throw new Error("unexpected fetch"); }, {
    capturing: true, tabId: 42, ingested: 7, failed: 1,
  });
  await vm.runInContext("ready", context);
  assert.equal(vm.runInContext("session.capturing && session.tabId === 42 && session.ingested === 7 && session.failed === 1", context), true);
});

test("unsent exchanges survive worker restart and the next capture", async () => {
  const storedState = {};
  const first = loadWorker(async () => { throw new Error("offline"); }, storedState);
  await vm.runInContext("ready", first);
  first.items = [{ method: "GET", url: "https://lab.example.test/one", status: 200 }];
  vm.runInContext("session.queue.push(...items)", first);
  await vm.runInContext("saveState()", first);

  const restarted = loadWorker(async () => { throw new Error("offline"); }, storedState);
  await vm.runInContext("ready", restarted);
  assert.equal(vm.runInContext("session.queue.length", restarted), 1);
  await vm.runInContext("start()", restarted);
  assert.equal(vm.runInContext("session.queue.length", restarted), 1);
});
