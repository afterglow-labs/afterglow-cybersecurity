// Afterglow Traffic Capture — background service worker.
//
// Captures the network activity of one tab through the chrome.debugger API
// (Network domain) and pushes text request/response pairs to the ARTEX
// ingest endpoint in batches. State lives in chrome.storage.session so the
// popup and the service worker share one view. Unsent entries remain in
// session storage until successfully flushed or the browser closes.

const FLUSH_AT = 10; // entries per batch
const MAX_BODY = 256 * 1024; // matches the server-side ingest cap
const MAX_QUEUE_BYTES = 7 * 1024 * 1024; // below chrome.storage.session's quota
const SKIP_MIME = ["image/", "font/", "audio/", "video/", "application/octet-stream"];

let session = {
  capturing: false,
  tabId: null,
  queue: [], // pending ExternalExchange objects
  requests: new Map(), // debugger requestId -> partial request info
  ingested: 0,
  failed: 0,
  lastError: "",
};
let flushTask = null;
let saveTask = Promise.resolve();
const pendingCaptures = new Set();

const ready = chrome.storage.session.get(["capturing", "tabId", "queue", "ingested", "failed", "lastError"])
  .then((state) => {
    session.capturing = !!state.capturing;
    session.tabId = state.tabId ?? null;
    session.queue = Array.isArray(state.queue) ? state.queue : [];
    session.ingested = state.ingested || 0;
    session.failed = state.failed || 0;
    session.lastError = state.lastError || "";
  });

async function saveState() {
  const snapshot = {
    capturing: session.capturing,
    tabId: session.tabId,
    ingested: session.ingested,
    failed: session.failed,
    lastError: session.lastError,
    queued: session.queue.length,
    queue: session.queue.slice(),
  };
  saveTask = saveTask.catch(() => {}).then(() => chrome.storage.session.set(snapshot));
  await saveTask;
}

function trimQueue() {
  while (session.queue.length > 200 ||
    new TextEncoder().encode(JSON.stringify(session.queue)).length > MAX_QUEUE_BYTES) {
    session.queue.shift();
    session.failed += 1;
    session.lastError = "Capture queue was full; oldest entries were dropped.";
  }
}

async function getConfig() {
  const { serverUrl = "", token = "" } = await chrome.storage.local.get(["serverUrl", "token"]);
  return { serverUrl: serverUrl.replace(/\/+$/, ""), token };
}

function validServerURL(raw) {
  try {
    const url = new URL(raw);
    const loopback = ["localhost", "127.0.0.1", "[::1]"].includes(url.hostname);
    return !url.username && !url.password && !url.search && !url.hash &&
      (url.pathname === "/" || url.pathname === "") &&
      (url.protocol === "https:" || (url.protocol === "http:" && loopback));
  } catch {
    return false;
  }
}

async function getActiveTabId() {
  const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
  return tab ? tab.id : null;
}

async function start() {
  await ready;
  if (session.capturing) return { ok: true };
  const { serverUrl, token } = await getConfig();
  if (!serverUrl || !token) {
    return { ok: false, error: "Set the Afterglow URL and token first." };
  }
  if (!validServerURL(serverUrl)) {
    return { ok: false, error: "Use HTTPS, or HTTP on localhost, for the Afterglow URL." };
  }
  const tabId = await getActiveTabId();
  if (tabId == null) return { ok: false, error: "No active tab." };
  try {
    await chrome.debugger.attach({ tabId }, "1.3");
  } catch (e) {
    return { ok: false, error: `attach failed: ${e.message}` };
  }
  await chrome.debugger.sendCommand({ tabId }, "Network.enable", {});
  session = { ...session, capturing: true, tabId, requests: new Map(), lastError: "" };
  chrome.action.setBadgeText({ text: "REC" });
  chrome.action.setBadgeBackgroundColor({ color: "#c2410c" });
  chrome.alarms.create("flush-capture", { periodInMinutes: 0.5 });
  await saveState();
  return { ok: true };
}

async function stop() {
  await ready;
  if (session.capturing && session.tabId != null) {
    session.capturing = false;
    await Promise.allSettled([...pendingCaptures]);
    await flushAll();
    try {
      await chrome.debugger.sendCommand({ tabId: session.tabId }, "Network.disable", {});
      await chrome.debugger.detach({ tabId: session.tabId });
    } catch (_) {
      // tab may already be gone; nothing to do
    }
  }
  session.capturing = false;
  session.tabId = null;
  await chrome.alarms.clear("flush-capture");
  chrome.action.setBadgeText({ text: "" });
  await saveState();
  return { ok: true };
}

function skipBody(mimeType) {
  const mt = (mimeType || "").toLowerCase();
  return SKIP_MIME.some((p) => mt.startsWith(p));
}

function truncate(s) {
  if (s.length <= MAX_BODY) return s;
  return s.slice(0, MAX_BODY) + "\n... [truncated by extension]";
}

async function queueExchange(info, response) {
  const respHeaders = {};
  for (const h of response.headers || []) respHeaders[h.name.toLowerCase()] = h.value;
  const mime = respHeaders["content-type"] || "";

  let reqBody = truncate(info.postData || "");
  let respBody = "";
  if (!skipBody(mime)) {
    try {
      const body = await chrome.debugger.sendCommand(
        { tabId: session.tabId },
        "Network.getResponseBody",
        { requestId: info.requestId }
      );
      if (body && !body.base64Encoded) {
        respBody = truncate(body.body || "");
      }
    } catch (_) {
      // body already discarded by the browser; capture the pair without bodies
    }
  }

  session.queue.push({
    method: info.method,
    url: info.url,
    status: response.status || 0,
    requestHeaders: info.headers || {},
    requestBody: reqBody,
    responseHeaders: respHeaders,
    responseBody: respBody,
  });
  trimQueue();
  await saveState();
  if (session.queue.length >= FLUSH_AT) await flushAll();
}

async function flush() {
  if (session.queue.length === 0) return true;
  const { serverUrl, token } = await getConfig();
  if (!serverUrl || !token || !validServerURL(serverUrl)) {
    session.lastError = "missing token or invalid server URL (HTTPS or localhost HTTP required)";
    await saveState();
    return false;
  }
  const batch = session.queue.splice(0, FLUSH_AT);
  try {
    const res = await fetch(`${serverUrl}/api/traffic/ingest`, {
      method: "POST",
      headers: { "Authorization": `Bearer ${token}`, "Content-Type": "application/json" },
      body: JSON.stringify({ exchanges: batch }),
    });
    if (!res.ok) throw new Error(`HTTP ${res.status}: ${(await res.text()).slice(0, 200)}`);
    const out = await res.json();
    session.ingested += out.ingested || 0;
    session.failed += out.failed || 0;
    session.lastError = out.failed ? `${out.failed} entries rejected by the server` : "";
  } catch (e) {
    // put the batch back (oldest first) but cap the queue so a dead server
    // cannot grow memory unbounded
    session.queue = batch.concat(session.queue);
    trimQueue();
    session.lastError = e.message;
    await saveState();
    return false;
  }
  await saveState();
  return true;
}

async function flushAll() {
  if (flushTask) return flushTask;
  flushTask = (async () => {
    while (session.queue.length > 0) {
      if (!(await flush())) break;
    }
  })();
  try {
    await flushTask;
  } finally {
    flushTask = null;
  }
}

chrome.debugger.onEvent.addListener((source, method, params) => {
  ready.then(() => onDebugEvent(source, method, params));
});

function onDebugEvent(source, method, params) {
  if (!session.capturing || !source.tabId || source.tabId !== session.tabId) return;
  if (method === "Network.requestWillBeSent") {
    session.requests.set(params.requestId, {
      requestId: params.requestId,
      method: params.request.method,
      url: params.request.url,
      headers: params.request.headers,
      postData: params.request.postData || "",
    });
  } else if (method === "Network.responseReceived") {
    const info = session.requests.get(params.requestId);
    if (info) info.response = params.response;
  } else if (method === "Network.loadingFinished") {
    const info = session.requests.get(params.requestId);
    session.requests.delete(params.requestId);
    if (!info || !info.response) return;
    const task = queueExchange(info, info.response);
    pendingCaptures.add(task);
    task.catch((e) => {
      session.lastError = e.message;
      saveState();
    }).finally(() => pendingCaptures.delete(task));
  }
}

chrome.debugger.onDetach.addListener((source) => {
  ready.then(() => onDebugDetach(source));
});

function onDebugDetach(source) {
  if (session.capturing && source.tabId === session.tabId) {
    // DevTools opened, or the tab navigated somewhere undebaggable: stop
    // cleanly instead of silently dropping events.
    session.capturing = false;
    session.tabId = null;
    session.lastError = "debugger detached (DevTools opened?)";
    chrome.action.setBadgeText({ text: "" });
    saveState();
  }
}

chrome.alarms.onAlarm.addListener((alarm) => {
  if (alarm.name === "flush-capture") ready.then(flushAll);
});

chrome.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
  (async () => {
    await ready;
    switch (msg?.type) {
      case "getState": {
        sendResponse({
          capturing: session.capturing,
          tabId: session.tabId,
          ingested: session.ingested,
          failed: session.failed,
          queued: session.queue.length,
          lastError: session.lastError,
          configured: !!(await getConfig()).serverUrl,
        });
        return;
      }
      case "start":
        sendResponse(await start());
        return;
      case "stop":
        sendResponse(await stop());
        return;
      case "flushNow":
        await flushAll();
        sendResponse(await stateSnapshot());
        return;
      default:
        sendResponse({ ok: false, error: "unknown message" });
    }
  })();
  return true; // async sendResponse
});

async function stateSnapshot() {
  return {
    capturing: session.capturing,
    tabId: session.tabId,
    ingested: session.ingested,
    failed: session.failed,
    queued: session.queue.length,
    lastError: session.lastError,
  };
}
