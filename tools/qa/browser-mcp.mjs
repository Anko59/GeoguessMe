import { chromium } from "/workspace/frontend/node_modules/playwright/index.mjs";
import { mkdir, writeFile } from "node:fs/promises";
import process from "node:process";
import { fakeLocation, probe } from "./browser-capabilities.mjs";
import { LinkTransferStore } from "./link-transfer.mjs";
import { MailboxGateway } from "./mailbox.mjs";
import { redactUrls } from "./safe-output.mjs";
const baseUrl = new URL(process.env.QA_BASE_URL || "http://127.0.0.1/");
const artifactDir = process.env.QA_ARTIFACT_DIR || "/tmp/qa-artifacts";
const maxText = 12000;
const maxItems = 40;
const hostArtifactDir = process.env.QA_HOST_ARTIFACT_DIR || artifactDir;
const budgets = {
  fast: { maxMinutes: 15, maxFindings: 10, screenshotLimit: 4 },
  full: { maxMinutes: 45, maxFindings: 30, screenshotLimit: 12 },
  nightly: { maxMinutes: 120, maxFindings: 60, screenshotLimit: 30 },
};
const budget = budgets[process.env.QA_BUDGET] || budgets.full;
const sessions = new Map();
const findings = [];
const artifacts = [];
const mailbox = new MailboxGateway({ provider: process.env.QA_MAILBOX_PROVIDER || "mailtm", productUrl: baseUrl });
const linkTransfers = new LinkTransferStore({ baseUrl });
const startedAt = new Date().toISOString();
const deadline = Date.now() + budget.maxMinutes * 60 * 1000;
let browser;
let sequence = 0;
let finished = false;
const tools = [
  tool("session_create", "Create an isolated browser session.", {
    type: "object",
    properties: { width: { type: "integer" }, height: { type: "integer" } },
  }),
  tool("session_close", "Close an isolated browser session.", {
    type: "object",
    required: ["session_id"],
    properties: { session_id: { type: "string" } },
  }),
  tool("tab_open", "Open a second tab in an existing session.", {
    type: "object",
    required: ["session_id"],
    properties: { session_id: { type: "string" } },
  }),
  tool("tab_switch", "Switch the active tab in a session.", {
    type: "object",
    required: ["session_id", "tab_id"],
    properties: { session_id: { type: "string" }, tab_id: { type: "string" } },
  }),
  tool("browser_navigate", "Navigate the active tab within the deployed application.", {
    type: "object",
    required: ["session_id", "url"],
    properties: { session_id: { type: "string" }, url: { type: "string" } },
  }),
  tool("browser_observe", "Read visible text, accessibility state, URL, and recent diagnostics.", {
    type: "object",
    required: ["session_id"],
    properties: { session_id: { type: "string" }, tab_id: { type: "string" } },
  }),
  tool("browser_transfer_link", "Capture a visible same-origin group invite into an opaque single-use transfer; never return the link.", { ...actionSchema(), required: ["session_id", "target", "kind"], properties: { ...actionSchema().properties, kind: { type: "string", enum: ["group-invite"] } } }),
  tool("browser_open_transferred_link", "Open an opaque group invite transfer in this isolated browser session; never return the link.", {
    type: "object", required: ["session_id", "transfer_id"], properties: { session_id: { type: "string" }, tab_id: { type: "string" }, transfer_id: { type: "string" } },
  }),
  tool("browser_capabilities", "Probe the granted synthetic camera and location services with a fixed safe check.", pageSchema()),
  tool("browser_click", "Click one visible control selected by role, label, text, or placeholder.", actionSchema()),
  tool("browser_type", "Fill one visible text control selected by role, label, text, or placeholder.", {
    ...actionSchema(),
    required: ["session_id", "target", "text"],
    properties: { ...actionSchema().properties, text: { type: "string" } },
  }),
  tool("browser_select", "Select an option in one visible select control.", {
    ...actionSchema(),
    required: ["session_id", "target", "value"],
    properties: { ...actionSchema().properties, value: { type: "string" } },
  }),
  tool("browser_upload", "Upload a generated small image fixture through a visible file control.", {
    ...actionSchema(),
    required: ["session_id", "target"],
  }),
  tool("browser_key", "Send a keyboard key to the active tab.", {
    type: "object",
    required: ["session_id", "key"],
    properties: { session_id: { type: "string" }, tab_id: { type: "string" }, key: { type: "string" } },
  }),
  tool("browser_reload", "Reload the active tab.", pageSchema()),
  tool("browser_back", "Go back in the active tab history.", pageSchema()),
  tool("browser_forward", "Go forward in the active tab history.", pageSchema()),
  tool("browser_resize", "Change the active tab viewport.", {
    type: "object",
    required: ["session_id", "width", "height"],
    properties: {
      session_id: { type: "string" }, tab_id: { type: "string" },
      width: { type: "integer", minimum: 320 }, height: { type: "integer", minimum: 240 },
    },
  }),
  tool("browser_wait_for", "Wait for a visible state, text, role, or URL condition.", {
    type: "object",
    required: ["session_id"],
    properties: {
      session_id: { type: "string" }, tab_id: { type: "string" },
      text: { type: "string" }, role: { type: "string" }, name: { type: "string" },
      label: { type: "string" }, url: { type: "string" },
      timeout_ms: { type: "integer", minimum: 100, maximum: 30000 },
    },
  }),
  tool("browser_screenshot", "Capture a targeted screenshot as evidence; do not use by default.", {
    ...pageSchema(),
    properties: { ...pageSchema().properties, purpose: { type: "string" } },
  }),
  tool("browser_diagnostics", "Read recent console and network summaries without sensitive payloads.", pageSchema()),
  tool("mailbox_create", "Create a disposable QA mailbox for a fresh test account; its password stays inside the provider gateway.", {
    type: "object", properties: {},
  }),
  tool("mailbox_search", "Search a QA mailbox for product email and optionally wait for delivery.", {
    type: "object",
    required: ["mailbox_id"],
    properties: {
      mailbox_id: { type: "string" }, subject_contains: { type: "string" }, from_contains: { type: "string" },
      since: { type: "string" }, wait_ms: { type: "integer", minimum: 0, maximum: 60000 },
    },
  }),
  tool("mailbox_read", "Read safe mailbox metadata and body text with links and email secrets removed.", {
    type: "object", required: ["mailbox_id", "message_id"],
    properties: { mailbox_id: { type: "string" }, message_id: { type: "string" } },
  }),
  tool("mailbox_open_link", "Open one matching product link from a mailbox message without returning its tokenized URL.", {
    type: "object", required: ["session_id", "mailbox_id", "message_id", "kind"],
    properties: {
      session_id: { type: "string" }, tab_id: { type: "string" }, mailbox_id: { type: "string" }, message_id: { type: "string" },
      kind: { type: "string", enum: ["verification", "group-invite", "password-reset", "any"] },
    },
  }),
  tool("qa_record_finding", "Record a reproducible black-box QA finding.", {
    type: "object",
    required: ["category", "severity", "title", "steps", "expected", "actual", "impact"],
    properties: {
      category: { type: "string", enum: ["BUG", "UX_DEBT", "VISUAL", "PERFORMANCE"] },
      severity: { type: "string", enum: ["low", "medium", "high", "critical"] },
      title: { type: "string" }, steps: { type: "array", items: { type: "string" } },
      expected: { type: "string" }, actual: { type: "string" }, impact: { type: "string" },
      artifacts: { type: "array", items: { type: "string" } }, notes: { type: "string" },
    },
  }),
  tool("qa_finish", "Finish the run and write the revision-bound evidence report.", {
    type: "object",
    required: ["status", "summary", "journeys_exercised"],
    properties: {
      status: { type: "string", enum: ["PASS", "FINDINGS", "BLOCKED"] },
      summary: { type: "string" },
      journeys_exercised: { type: "array", items: { type: "string" } },
      journeys_not_exercised: { type: "array", items: { type: "string" } },
      limitations: { type: "array", items: { type: "string" } },
    },
  }),
];
function tool(name, description, inputSchema) {
  return { name, description, inputSchema: inputSchema || { type: "object", properties: {} } };
}
function pageSchema() {
  return {
    type: "object",
    required: ["session_id"],
    properties: { session_id: { type: "string" }, tab_id: { type: "string" } },
  };
}
function actionSchema() {
  return {
    type: "object",
    required: ["session_id", "target"],
    properties: {
      session_id: { type: "string" }, tab_id: { type: "string" },
      target: { type: "object", additionalProperties: false },
    },
  };
}
function clipped(value, limit = maxText) {
  const text = String(value ?? "");
  return text.length <= limit ? text : `${text.slice(0, limit)}\n[truncated]`;
}
function safeUrl(value) {
  try {
    const url = new URL(value, baseUrl);
    if (/(?:token|secret|code|key|password|auth|cookie|invite|reset|verify)/i.test(`${url.pathname} ${url.search} ${url.hash}`)) return "[redacted-url]";
    for (const key of [...url.searchParams.keys()]) {
      if (/token|secret|code|key|password|auth|cookie/i.test(key)) url.searchParams.set(key, "[redacted]");
    }
    url.hash = "";
    return url.toString();
  } catch {
    return "[invalid-url]";
  }
}
function safeText(value) {
  let text = redactUrls(String(value ?? ""));
  for (const secret of [process.env.QA_ACCESS_CLIENT_ID, process.env.QA_ACCESS_CLIENT_SECRET]) {
    if (secret && secret.length > 3) text = text.split(secret).join("[redacted]");
  }
  text = text.replace(
    /((?:textbox|combobox|input|textarea)\s+["']?(?:password|passcode|token|secret|authorization|cookie)[^:\n]*:\s*)[^\n]+/gi,
    "$1[redacted]",
  );
  text = text.replace(
    /(["']?(?:password|passcode|token|secret|authorization|cookie)["']?\s*[:=]\s*)[^\s,}\n]+/gi,
    "$1[redacted]",
  );
  return clipped(text);
}
function sessionFor(args) {
  const session = sessions.get(args.session_id);
  if (!session) throw new Error(`Unknown session: ${args.session_id}`);
  const tab = session.tabs.get(args.tab_id || session.activeTab);
  if (!tab) throw new Error(`Unknown tab in session: ${args.tab_id}`);
  return { session, tab, page: tab.page };
}
async function ensureBrowser() {
  browser ||= await chromium.launch({
    headless: true,
    args: ["--use-fake-ui-for-media-stream", "--use-fake-device-for-media-stream"],
  });
  return browser;
}
async function newSession(args) {
  const instance = await ensureBrowser();
  const context = await instance.newContext({
    viewport: { width: args.width || 1440, height: args.height || 900 },
    permissions: ["camera", "geolocation"],
    geolocation: fakeLocation(),
    extraHTTPHeaders: accessHeaders(),
  });
  const page = await context.newPage();
  const sessionId = `session-${++sequence}`;
  const tabId = `tab-${sequence}-1`;
  const session = { context, tabs: new Map(), activeTab: tabId };
  session.tabs.set(tabId, attachPage(page, tabId));
  sessions.set(sessionId, session);
  return { session_id: sessionId, tab_id: tabId, viewport: await page.evaluate(() => ({ width: innerWidth, height: innerHeight })) };
}
function accessHeaders() {
  const id = process.env.QA_ACCESS_CLIENT_ID;
  const secret = process.env.QA_ACCESS_CLIENT_SECRET;
  return id && secret ? { "CF-Access-Client-Id": id, "CF-Access-Client-Secret": secret } : {};
}
function attachPage(page, tabId) {
  const state = { page, tabId, console: [], network: [] };
  page.on("console", (message) => pushBounded(state.console, { type: message.type(), text: safeText(message.text()) }));
  page.on("pageerror", (error) => pushBounded(state.console, { type: "pageerror", text: safeText(error.message) }));
  page.on("requestfailed", (request) => pushBounded(state.network, { kind: "failed", method: request.method(), url: safeUrl(request.url()), error: safeText(request.failure()?.errorText) }));
  page.on("response", (response) => {
    const status = response.status();
    if (status >= 400) pushBounded(state.network, { kind: "response", method: response.request().method(), url: safeUrl(response.url()), status });
  });
  return state;
}
function pushBounded(list, value) {
  list.push(value);
  if (list.length > maxItems) list.splice(0, list.length - maxItems);
}
async function locate(page, target) {
  if (!target || typeof target !== "object") throw new Error("A role, label, text, placeholder, or test id target is required");
  let locator;
  if (target.role) locator = page.getByRole(target.role, target.name ? { name: target.name, exact: target.exact !== false } : undefined);
  else if (target.label) locator = page.getByLabel(target.label, { exact: target.exact !== false });
  else if (target.placeholder) locator = page.getByPlaceholder(target.placeholder, { exact: target.exact !== false });
  else if (target.text) locator = page.getByText(target.text, { exact: target.exact !== false });
  else if (target.test_id) locator = page.getByTestId(target.test_id);
  else throw new Error("CSS selectors and arbitrary JavaScript are not part of the QA contract");
  const count = await locator.count();
  if (count !== 1) throw new Error(`Target resolved to ${count} elements; refine it with an accessible role or label`);
  return locator;
}
function waitLocator(page, args) {
  if (args.text) return page.getByText(args.text, { exact: true });
  if (args.role) return page.getByRole(args.role, args.name ? { name: args.name, exact: true } : undefined);
  if (args.label) return page.getByLabel(args.label, { exact: true });
  throw new Error("browser_wait_for requires text, role, or label, or a URL");
}
async function observe(args) {
  const { page, tab } = sessionFor(args);
  const [title, text, aria] = await Promise.all([
    page.title().catch(() => ""),
    page.locator("body").innerText({ timeout: 3000 }).catch(() => ""),
    page.locator("body").ariaSnapshot({ timeout: 3000 }).catch(() => ""),
  ]);
  return {
    tab_id: tab.tabId,
    url: safeUrl(page.url()),
    title: safeText(title),
    visible_text: safeText(text),
    accessibility: safeText(aria),
    diagnostics: { console: tab.console.slice(-10), network: tab.network.slice(-10) },
  };
}
function assertAllowedNavigation(value) {
  const url = new URL(value, baseUrl);
  if (url.origin !== baseUrl.origin) throw new Error("Navigation outside QA_BASE_URL origin is blocked");
  return url.toString();
}
async function writeReport(summary = {}) {
  await mkdir(artifactDir, { recursive: true });
  const report = {
    schema_version: 1,
    started_at: startedAt,
    finished_at: new Date().toISOString(),
    target: { base_url: `${baseUrl.origin}${baseUrl.pathname}`, build_sha: process.env.QA_BUILD_SHA || "unknown" },
    runtime: process.env.QA_RUNTIME || "unknown",
    budget: process.env.QA_BUDGET || "full",
    status: summary.status || (findings.length ? "FINDINGS" : "BLOCKED"),
    summary: summary.summary || "The runtime ended before qa_finish was called.",
    journeys_exercised: summary.journeys_exercised || [],
    journeys_not_exercised: summary.journeys_not_exercised || [],
    limitations: summary.limitations || [],
    capabilities: {
      camera_mock: true,
      geolocation_mock: fakeLocation(),
      mailbox_provider: process.env.QA_MAILBOX_PROVIDER || "mailtm",
      mailboxes_created: mailbox.mailboxes.size,
    },
    findings,
    artifacts,
  };
  await writeFile(`${artifactDir}/qa-report.json`, `${JSON.stringify(report, null, 2)}\n`, { mode: 0o600 });
  return { report_path: `${hostArtifactDir}/qa-report.json`, finding_count: findings.length, blocking_finding_count: findings.filter((finding) => finding.blocking).length };
}

async function call(name, args) {
  if (name === "session_create") return newSession(args);
  if (name === "session_close") {
    const session = sessions.get(args.session_id);
    if (!session) throw new Error(`Unknown session: ${args.session_id}`);
    await session.context.close();
    sessions.delete(args.session_id);
    return { closed: args.session_id };
  }
  if (name === "tab_open") {
    const session = sessions.get(args.session_id);
    if (!session) throw new Error(`Unknown session: ${args.session_id}`);
    const page = await session.context.newPage();
    const tabId = `tab-${args.session_id}-${session.tabs.size + 1}`;
    session.tabs.set(tabId, attachPage(page, tabId));
    session.activeTab = tabId;
    return { tab_id: tabId };
  }
  if (name === "tab_switch") {
    const session = sessions.get(args.session_id);
    if (!session?.tabs.has(args.tab_id)) throw new Error(`Unknown tab: ${args.tab_id}`);
    session.activeTab = args.tab_id;
    return { active_tab: args.tab_id };
  }
  if (name === "browser_navigate") {
    const { page } = sessionFor(args);
    const url = assertAllowedNavigation(args.url);
    await page.goto(url, { waitUntil: "domcontentloaded", timeout: 30000 });
    return observe(args);
  }
  if (name === "browser_observe") return observe(args);
  if (name === "browser_transfer_link") {
    const { page } = sessionFor(args);
    const locator = await locate(page, args.target);
    return linkTransfers.capture(await locator.inputValue().catch(() => locator.getAttribute("href")));
  }
  if (name === "browser_open_transferred_link") {
    const { page } = sessionFor(args);
    await page.goto(linkTransfers.consume(args.transfer_id), { waitUntil: "domcontentloaded", timeout: 30000 });
    return observe(args);
  }
  if (name === "browser_capabilities") {
    const { page } = sessionFor(args);
    return probe(page);
  }
  if (["browser_click", "browser_type", "browser_select", "browser_upload"].includes(name)) {
    const { page } = sessionFor(args);
    const locator = await locate(page, args.target);
    if (name === "browser_click") await locator.click({ timeout: 10000 });
    if (name === "browser_type") await locator.fill(args.text);
    if (name === "browser_select") await locator.selectOption(args.value);
    if (name === "browser_upload") await locator.setInputFiles({ name: "qa-fixture.png", mimeType: "image/png", buffer: Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=", "base64") });
    return observe(args);
  }
  if (name === "browser_key") {
    const { page } = sessionFor(args);
    await page.keyboard.press(args.key);
    return observe(args);
  }
  if (name === "browser_reload") {
    const { page } = sessionFor(args);
    await page.reload({ waitUntil: "domcontentloaded", timeout: 30000 });
    return observe(args);
  }
  if (name === "browser_back" || name === "browser_forward") {
    const { page } = sessionFor(args);
    await (name === "browser_back" ? page.goBack() : page.goForward());
    return observe(args);
  }
  if (name === "browser_resize") {
    const { page } = sessionFor(args);
    await page.setViewportSize({ width: args.width, height: args.height });
    return observe(args);
  }
  if (name === "browser_wait_for") {
    const { page } = sessionFor(args);
    const timeout = args.timeout_ms || 10000;
    if (args.url) await page.waitForURL((url) => safeUrl(url.toString()).includes(args.url), { timeout });
    else await waitLocator(page, args).first().waitFor({ state: "visible", timeout });
    return observe(args);
  }
  if (name === "browser_screenshot") {
    if (artifacts.filter((artifact) => artifact.type === "screenshot").length >= budget.screenshotLimit) {
      throw new Error(`Screenshot budget exhausted for ${process.env.QA_BUDGET || "full"} run`);
    }
    const { page } = sessionFor(args);
    const file = `screenshot-${String(++sequence).padStart(3, "0")}.png`;
    await page.screenshot({ path: `${artifactDir}/${file}`, fullPage: false });
    const path = `${hostArtifactDir}/${file}`;
    artifacts.push({ type: "screenshot", path, purpose: safeText(args.purpose || "targeted evidence") });
    return { artifact_path: path, purpose: safeText(args.purpose || "targeted evidence") };
  }
  if (name === "browser_diagnostics") {
    const { tab } = sessionFor(args);
    return { console: tab.console.slice(-20), network: tab.network.slice(-20) };
  }
  if (name === "mailbox_create") return mailbox.create();
  if (name === "mailbox_search") return mailbox.search(args);
  if (name === "mailbox_read") return mailbox.read(args);
  if (name === "mailbox_open_link") {
    const { page } = sessionFor(args);
    const link = await mailbox.link(args);
    try {
      await page.goto(link.url, { waitUntil: "domcontentloaded", timeout: 30000 });
    } catch {
      throw new Error("Mailbox link navigation failed");
    }
    return observe(args);
  }
  if (name === "qa_record_finding") {
    if (findings.length >= budget.maxFindings) {
      throw new Error(`Finding budget exhausted for ${process.env.QA_BUDGET || "full"} run`);
    }
    const finding = { id: `finding-${findings.length + 1}`, ...args, blocking: args.category === "BUG", recorded_at: new Date().toISOString() };
    finding.title = safeText(finding.title);
    finding.steps = finding.steps.map(safeText);
    finding.expected = safeText(finding.expected);
    finding.actual = safeText(finding.actual);
    finding.impact = safeText(finding.impact);
    finding.notes = safeText(finding.notes);
    findings.push(finding);
    return { id: finding.id, blocking: finding.blocking, finding_count: findings.length };
  }
  if (name === "qa_finish") {
    finished = true;
    return writeReport(args);
  }
  throw new Error(`Unknown tool: ${name}`);
}

async function close() {
  if (!finished) await writeReport();
  for (const session of sessions.values()) await session.context.close().catch(() => {});
  await browser?.close().catch(() => {});
  linkTransfers.clear();
  await mailbox.cleanup();
}

process.stdin.setEncoding("utf8");
let buffer = "";
process.stdin.on("data", (chunk) => {
  buffer += chunk;
  const lines = buffer.split("\n");
  buffer = lines.pop() || "";
  for (const line of lines) if (line.trim()) handle(line).catch((error) => process.stderr.write(`${error.stack || error}\n`));
});
process.stdin.on("end", () => close().finally(() => process.exit(0)));
process.on("SIGTERM", () => close().finally(() => process.exit(0)));

async function handle(line) {
  const request = JSON.parse(line);
  if (request.method?.startsWith("notifications/")) return;
  if (Date.now() > deadline && request.method === "tools/call" && request.params?.name !== "qa_finish") {
    return respondError(request.id, -32000, `QA budget expired after ${budget.maxMinutes} minutes`);
  }
  if (request.method === "initialize") return respond(request.id, { protocolVersion: request.params?.protocolVersion || "2025-06-18", capabilities: { tools: { listChanged: false } }, serverInfo: { name: "qa-browser", version: "1.0.0" } });
  if (request.method === "ping") return respond(request.id, {});
  if (request.method === "tools/list") return respond(request.id, { tools });
  if (request.method === "tools/call") {
    try {
      const result = await call(request.params.name, request.params.arguments || {});
      return respond(request.id, { content: [{ type: "text", text: JSON.stringify(result) }], structuredContent: result });
    } catch (error) {
      return respond(request.id, { isError: true, content: [{ type: "text", text: safeText(error.message) }] });
    }
  }
  respondError(request.id, -32601, `Unknown method: ${request.method}`);
}

function respond(id, result) {
  process.stdout.write(`${JSON.stringify({ jsonrpc: "2.0", id, result })}\n`);
}

function respondError(id, code, message) {
  process.stdout.write(`${JSON.stringify({ jsonrpc: "2.0", id, error: { code, message } })}\n`);
}
