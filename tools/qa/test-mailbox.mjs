import { createServer } from "node:http";
import { once } from "node:events";
import { MailboxGateway } from "./mailbox.mjs";

const message = {
  id: "message-1",
  subject: "Verify your GeoGuessMe account",
  from: { address: "no-reply@geoguessme.com" },
  to: [{ address: "qa@example.test" }],
  createdAt: new Date().toISOString(),
  intro: "Verify your account",
  text: "Verify: https://dev.geoguessme.test/verify-email?token=secret-token",
  html: "<a href=\"https://dev.geoguessme.test/verify-email?token=secret-token\">Verify</a>",
};

const server = createServer((request, response) => {
  response.setHeader("Content-Type", "application/json");
  if (request.url === "/domains") return respond(response, { "hydra:member": [{ domain: "example.test", isActive: true, isPrivate: false }] });
  if (request.method === "DELETE" && request.url === "/accounts/account-1") return respond(response, null, 204);
  if (request.url === "/accounts") return respond(response, { id: "account-1" }, 201);
  if (request.url === "/token") return respond(response, { token: "test-token" });
  if (request.url === "/messages") return respond(response, { "hydra:member": [message] });
  if (request.url === "/messages/message-1") return respond(response, message);
  response.statusCode = 404;
  return respond(response, { error: "not found" });
});

function respond(response, body, status = 200) {
  response.statusCode = status;
  response.end(JSON.stringify(body));
}

await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
const { port } = server.address();
const gateway = new MailboxGateway({ productUrl: "https://dev.geoguessme.test", apiUrl: `http://127.0.0.1:${port}` });
try {
  const created = await gateway.create();
  const found = await gateway.search({ mailbox_id: created.mailbox_id, subject_contains: "verify" });
  if (found.messages.length !== 1) throw new Error("mailbox search did not find the fixture");
  const safe = await gateway.read({ mailbox_id: created.mailbox_id, message_id: "message-1" });
  if (safe.body.includes("secret-token") || !safe.links_available.includes("verification")) throw new Error("mailbox redaction contract failed");
  const link = await gateway.link({ mailbox_id: created.mailbox_id, message_id: "message-1", kind: "verification" });
  if (!link.url.includes("secret-token")) throw new Error("mailbox link resolver failed");
  await gateway.cleanup();
  if (gateway.mailboxes.size !== 0) throw new Error("mailbox cleanup contract failed");
  console.log("Mailbox gateway contract PASSED");
} finally {
  server.close();
  await once(server, "close");
}
