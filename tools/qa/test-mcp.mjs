import { spawn } from "node:child_process";
import { once } from "node:events";

const child = spawn(process.execPath, ["/workspace/tools/qa/browser-mcp.mjs"], {
  env: { ...process.env, QA_BASE_URL: process.env.QA_BASE_URL || "https://dev.geoguessme.com", QA_ARTIFACT_DIR: "/tmp/qa-contract" },
  stdio: ["pipe", "pipe", "pipe"],
});
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
  for (const required of ["session_create", "browser_observe", "browser_screenshot", "qa_record_finding", "qa_finish"]) {
    if (!names.has(required)) throw new Error(`missing tool ${required}`);
  }
  const session = await request(3, "tools/call", { name: "session_create", arguments: { width: 800, height: 600 } });
  if (!session.structuredContent?.session_id) throw new Error("session_create returned no session id");
  await request(4, "tools/call", { name: "session_close", arguments: { session_id: session.structuredContent.session_id } });
  const report = await request(5, "tools/call", { name: "qa_finish", arguments: { status: "PASS", summary: "contract test", journeys_exercised: [] } });
  if (!report.structuredContent?.report_path) throw new Error("qa_finish returned no report path");
  child.stdin.end();
  await once(child, "close");
  console.log("MCP lifecycle contract PASSED");
} catch (error) {
  child.kill("SIGTERM");
  console.error(error.message);
  process.exitCode = 1;
}
