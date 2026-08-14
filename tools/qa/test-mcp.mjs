import { spawn } from "node:child_process";
import { once } from "node:events";
import { createServer } from "node:http";

const pageServer = createServer((request, response) => {
  response.setHeader("Content-Type", "text/html");
  response.end(`<!doctype html><title>QA MCP contract</title><main>ready <form action="/groups" method="get"><label>Username <input aria-label="Username"></label><label>Password <input aria-label="Password" type="password"></label><button type="submit">Login</button></form><label>Invite link <input aria-label="Invite link" value="http://${request.headers.host}/group/join#invite=secret-invite"></label></main>`);
});
await new Promise((resolve) => pageServer.listen(0, "127.0.0.1", resolve));
const pageUrl = `http://127.0.0.1:${pageServer.address().port}`;
const child = spawn(process.execPath, ["/workspace/tools/qa/browser-mcp.mjs"], {
  env: { ...process.env, QA_BASE_URL: pageUrl, QA_ARTIFACT_DIR: "/tmp/qa-contract", QA_ACCOUNT_PASSWORD: "contract-password" },
  stdio: ["pipe", "pipe", "pipe"],
});
child.stderr.on("data", (chunk) => process.stderr.write(chunk));
let buffer = "";
const responses = new Map();
const waiters = new Map();
child.stdout.setEncoding("utf8");
child.stdout.on("data", (chunk) => {
  buffer += chunk;
  const lines = buffer.split("\n");
  buffer = lines.pop() || "";
  for (const line of lines) {
    if (!line.trim()) continue;
    const message = JSON.parse(line);
    if (message.id === undefined) continue;
    const waiter = waiters.get(message.id);
    if (waiter) {
      waiters.delete(message.id);
      waiter(message);
    } else {
      responses.set(message.id, message);
    }
  }
});

async function request(id, method, params = {}) {
  const message = responses.has(id)
    ? responses.get(id)
    : await new Promise((resolve, reject) => {
        const timer = setTimeout(() => {
          waiters.delete(id);
          reject(new Error(`timeout waiting for ${method}`));
        }, 15000);
        waiters.set(id, (response) => {
          clearTimeout(timer);
          resolve(response);
        });
        child.stdin.write(`${JSON.stringify({ jsonrpc: "2.0", id, method, params })}\n`);
      });
  if (message.error) throw new Error(message.error.message);
  return message.result;
}

try {
  await request(1, "initialize", { protocolVersion: "2025-06-18", capabilities: {}, clientInfo: { name: "contract-test", version: "1" } });
  child.stdin.write('{"jsonrpc":"2.0","method":"notifications/initialized"}\n');
  const listed = await request(2, "tools/list");
  const names = new Set(listed.tools.map((entry) => entry.name));
  for (const required of ["session_create", "qa_account_login", "browser_observe", "browser_screenshot", "browser_transfer_link", "browser_open_transferred_link", "qa_record_finding", "qa_finish"]) {
    if (!names.has(required)) throw new Error(`missing tool ${required}`);
  }
  if (process.env.QA_LIVE_MAILBOX === "1") {
    const liveMailbox = await request(20, "tools/call", { name: "mailbox_create", arguments: {} });
    if (!liveMailbox.structuredContent?.mailbox_id || !liveMailbox.structuredContent?.address) {
      throw new Error(`live mailbox creation failed: ${liveMailbox.content?.[0]?.text || "no address"}`);
    }
    console.log("Live mailbox provider contract PASSED");
  }
  const session = await request(3, "tools/call", { name: "session_create", arguments: { width: 800, height: 600 } });
  if (!session.structuredContent?.session_id) throw new Error("session_create returned no session id");
  const sessionId = session.structuredContent.session_id;
  await request(4, "tools/call", { name: "browser_navigate", arguments: { session_id: sessionId, url: pageUrl } });
  const observed = await request(5, "tools/call", { name: "browser_observe", arguments: { session_id: sessionId } });
  if (JSON.stringify(observed).includes("secret-invite")) throw new Error("safe browser output leaked an invite token");
  const transfer = await request(6, "tools/call", { name: "browser_transfer_link", arguments: { session_id: sessionId, target: { label: "Invite link" }, kind: "group-invite" } });
  if (!transfer.structuredContent?.transfer_id || JSON.stringify(transfer).includes("secret-invite")) throw new Error("invite transfer leaked a link or returned no opaque id");
  const member = await request(7, "tools/call", { name: "session_create", arguments: { width: 800, height: 600 } });
  const memberId = member.structuredContent?.session_id;
  if (!memberId) throw new Error("member session_create returned no session id");
  await request(8, "tools/call", { name: "browser_navigate", arguments: { session_id: memberId, url: pageUrl } });
  const opened = await request(9, "tools/call", { name: "browser_open_transferred_link", arguments: { session_id: memberId, transfer_id: transfer.structuredContent.transfer_id } });
  if (JSON.stringify(opened).includes("secret-invite")) throw new Error("opened invite transfer leaked an invite token");
  const reused = await request(14, "tools/call", { name: "browser_open_transferred_link", arguments: { session_id: memberId, transfer_id: transfer.structuredContent.transfer_id } });
  if (!reused.isError || JSON.stringify(reused).includes("secret-invite")) throw new Error("invite transfer was not single-use or leaked a token on reuse");
  const capabilities = await request(10, "tools/call", { name: "browser_capabilities", arguments: { session_id: sessionId } });
  if (!capabilities.structuredContent?.camera?.usable || !capabilities.structuredContent?.geolocation?.usable) {
    throw new Error("synthetic camera/location capability probe failed");
  }
  const loggedIn = await request(11, "tools/call", { name: "qa_account_login", arguments: { session_id: sessionId, account_role: "owner" } });
  if (!loggedIn.structuredContent?.authenticated || loggedIn.structuredContent.account_role !== "owner") {
    throw new Error("qa_account_login contract failed");
  }
  for (const required of ["browser_capabilities", "mailbox_create", "mailbox_search", "mailbox_read", "mailbox_open_link"]) {
    if (!names.has(required)) throw new Error(`missing extended QA tool ${required}`);
  }
  await request(11, "tools/call", { name: "session_close", arguments: { session_id: sessionId } });
  await request(12, "tools/call", { name: "session_close", arguments: { session_id: memberId } });
  const report = await request(15, "tools/call", { name: "qa_finish", arguments: { status: "PASS", summary: "contract test", journeys_exercised: [] } });
  if (!report.structuredContent?.report_path) throw new Error("qa_finish returned no report path");
  child.stdin.end();
  await once(child, "close");
  pageServer.close();
  await once(pageServer, "close");
  console.log("MCP lifecycle contract PASSED");
} catch (error) {
  child.kill("SIGTERM");
  pageServer.close();
  console.error(error.message);
  process.exitCode = 1;
}
