import { createServer } from "node:http";
import { once } from "node:events";
import { MailboxGateway, resolveMailboxAccessCredentials } from "./mailbox.mjs";

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
const identityVerificationMessage = {
  ...message,
  id: "message-idp",
  text: "Verify: https://auth.geoguessme.com/realms/geoguessme/login-actions/action-token?key=secret-token&client_id=qa",
  html: "<a href=\"https://auth.geoguessme.com/realms/geoguessme/login-actions/action-token?key=secret-token&amp;client_id=qa\">Verify</a>",
};
const identityResetMessage = {
  ...identityVerificationMessage,
  id: "message-idp-reset",
  subject: "Reset password",
};
const rawMessage = [
  "From: no-reply@geoguessme.com",
  "To: qa-release-20260815-1+run-test@geoguessme.com",
  "Subject: Base64 fixture",
  `Date: ${message.createdAt}`,
  "Content-Type: text/plain; charset=utf-8",
  "Content-Transfer-Encoding: base64",
  "",
  Buffer.from(message.text).toString("base64"),
  "",
].join("\r\n");
const quotedPrintableMessage = [
  "From: no-reply@geoguessme.com",
  "To: qa-release-20260815-1+run-test@geoguessme.com",
  "Subject: Verify your GeoGuessMe account",
  `Date: ${message.createdAt}`,
  "MIME-Version: 1.0",
  'Content-Type: multipart/alternative; boundary="qa-boundary"',
  "",
  "--qa-boundary",
  "Content-Type: text/plain; charset=UTF-8",
  "Content-Transfer-Encoding: quoted-printable",
  "",
  "Verify: https://auth.geoguessme.com/realms/geoguessme/login-actions/action-token?key=3Dsynthetic-verifi=",
  "cation-token&client_id=3Dqa",
  "--qa-boundary",
  "Content-Type: text/html; charset=UTF-8",
  "Content-Transfer-Encoding: quoted-printable",
  "",
  '<a href=3D"https://auth.geoguessme.com/realms/geoguessme/login-actions/action-token?key=3Dsynthetic-verifi=',
  'cation-token&amp;client_id=3Dqa">Verify</a>',
  "--qa-boundary--",
  "",
].join("\r\n");
const cloudflareAccessHeaders = [];
const sameOriginCredentials = resolveMailboxAccessCredentials({
  provider: "cloudflare",
  productUrl: "https://dev.geoguessme.test",
  apiUrl: "https://dev.geoguessme.test/_qa-mailbox",
  appClientId: "dev-access-id",
  appClientSecret: "dev-access-secret",
});
if (sameOriginCredentials.mailboxAccessClientId !== "dev-access-id" || sameOriginCredentials.mailboxAccessClientSecret !== "dev-access-secret") {
  throw new Error("Same-origin mailbox relay did not use the QA app Access credentials");
}
const unrelatedOriginCredentials = resolveMailboxAccessCredentials({
  provider: "cloudflare",
  productUrl: "https://dev.geoguessme.test",
  apiUrl: "https://mailbox.example.test/_qa-mailbox",
  appClientId: "dev-access-id",
  appClientSecret: "dev-access-secret",
});
if (unrelatedOriginCredentials.mailboxAccessClientId || unrelatedOriginCredentials.mailboxAccessClientSecret) {
  throw new Error("QA app Access credentials leaked to a mailbox relay on another origin");
}
const explicitMailboxCredentials = resolveMailboxAccessCredentials({
  provider: "cloudflare",
  productUrl: "https://dev.geoguessme.test",
  apiUrl: "https://mailbox.example.test/_qa-mailbox",
  appClientId: "dev-access-id",
  appClientSecret: "dev-access-secret",
  mailboxClientId: "mailbox-access-id",
  mailboxClientSecret: "mailbox-access-secret",
});
if (explicitMailboxCredentials.mailboxAccessClientId !== "mailbox-access-id" || explicitMailboxCredentials.mailboxAccessClientSecret !== "mailbox-access-secret") {
  throw new Error("Explicit mailbox Access credentials were not selected");
}
try {
  resolveMailboxAccessCredentials({ productUrl: "https://dev.geoguessme.test", mailboxClientId: "mailbox-access-id" });
  throw new Error("Incomplete mailbox Access credentials were accepted");
} catch (error) {
  if (!error.message.includes("must be supplied together")) throw error;
}
const mailTmCredentials = resolveMailboxAccessCredentials({
  productUrl: "https://dev.geoguessme.test",
  apiUrl: "",
  appClientId: "dev-access-id",
  appClientSecret: "dev-access-secret",
});
if (mailTmCredentials.mailboxAccessClientId || mailTmCredentials.mailboxAccessClientSecret) {
  throw new Error("Empty Mail.tm API configuration inherited app Access credentials");
}

const server = createServer((request, response) => {
  if (request.url.startsWith("/v1/inbox/qa-release-20260815-1+run-test")) {
    cloudflareAccessHeaders.push({
      clientId: request.headers["cf-access-client-id"],
      clientSecret: request.headers["cf-access-client-secret"],
    });
  }
  if (request.url === "/v1/inbox/qa-release-20260815-1+run-test/message/message-1") {
    response.setHeader("Content-Type", "message/rfc822");
    response.end(rawMessage);
    return;
  }
  if (request.url === "/v1/inbox/qa-release-20260815-1+run-test/message/message-qp") {
    response.setHeader("Content-Type", "message/rfc822");
    response.end(quotedPrintableMessage);
    return;
  }
  if (request.url === "/v1/inbox/qa-release-20260815-1+run-test") return respond(response, { messages: [{ id: "message-1", created_at: message.createdAt }, { id: "message-qp", created_at: message.createdAt }] });
  response.setHeader("Content-Type", "application/json");
  if (request.url === "/domains") return respond(response, { "hydra:member": [{ domain: "example.test", isActive: true, isPrivate: false }] });
  if (request.method === "DELETE" && request.url === "/accounts/account-1") return respond(response, null, 204);
  if (request.url === "/accounts") return respond(response, { id: "account-1" }, 201);
  if (request.url === "/token") return respond(response, { token: "test-token" });
  if (request.url === "/messages") return respond(response, { "hydra:member": [message] });
  if (request.url === "/messages/message-1") return respond(response, message);
  if (request.url === "/messages/message-idp") return respond(response, identityVerificationMessage);
  if (request.url === "/messages/message-idp-reset") return respond(response, identityResetMessage);
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
const defaultGateway = new MailboxGateway({ productUrl: "https://dev.geoguessme.test", apiUrl: "" });
if (defaultGateway.apiUrl !== "https://api.mail.tm") throw new Error("Empty Mail.tm API configuration did not use its provider default");
const identityGateway = new MailboxGateway({
  productUrl: "https://dev.geoguessme.test",
  apiUrl: `http://127.0.0.1:${port}`,
  allowedLinkOrigins: ["https://auth.geoguessme.com"],
});
const identityMailbox = { accountId: "account-1", address: "qa@example.test", token: "test-token" };
identityGateway.mailboxes.set("mailbox-idp", identityMailbox);
const identitySafe = await identityGateway.read({ mailbox_id: "mailbox-idp", message_id: "message-idp" });
if (!identitySafe.links_available.includes("verification") || identitySafe.body.includes("secret-token")) {
  throw new Error("Configured identity-origin verification links were not safely recognized");
}
const identityLink = await identityGateway.link({ mailbox_id: "mailbox-idp", message_id: "message-idp", kind: "verification" });
if (!identityLink.url.startsWith("https://auth.geoguessme.com/realms/geoguessme/login-actions/action-token")) {
  throw new Error("Configured identity-origin verification link was not resolved");
}
const identityResetSafe = await identityGateway.read({ mailbox_id: "mailbox-idp", message_id: "message-idp-reset" });
if (!identityResetSafe.links_available.includes("password-reset") || identityResetSafe.links_available.includes("verification")) {
  throw new Error("Identity-provider reset mail was misclassified as verification");
}
const identityResetLink = await identityGateway.link({ mailbox_id: "mailbox-idp", message_id: "message-idp-reset", kind: "password-reset" });
if (!identityResetLink.url.includes("/login-actions/action-token")) throw new Error("Identity-provider reset link was not resolved");
try {
  await identityGateway.link({ mailbox_id: "mailbox-idp", message_id: "message-idp-reset", kind: "verification" });
  throw new Error("Identity-provider reset link was accepted as verification");
} catch (error) {
  if (!error.message.includes("No matching safe product link")) throw error;
}
try {
  const created = await gateway.create();
  try {
    await gateway.link({ mailbox_id: created.mailbox_id, message_id: "message-idp", kind: "verification" });
    throw new Error("Unconfigured identity-origin verification link was accepted");
  } catch (error) {
    if (!error.message.includes("No matching safe product link")) throw error;
  }
  const found = await gateway.search({ mailbox_id: created.mailbox_id, subject_contains: "verify" });
  if (found.messages.length !== 1) throw new Error("mailbox search did not find the fixture");
  const safe = await gateway.read({ mailbox_id: created.mailbox_id, message_id: "message-1" });
  if (safe.body.includes("secret-token") || !safe.links_available.includes("verification")) throw new Error("mailbox redaction contract failed");
  const link = await gateway.link({ mailbox_id: created.mailbox_id, message_id: "message-1", kind: "verification" });
  if (!link.url.includes("secret-token")) throw new Error("mailbox link resolver failed");
  await gateway.cleanup();
  if (gateway.mailboxes.size !== 0) throw new Error("mailbox cleanup contract failed");
  const cloudflareGateway = new MailboxGateway({
    provider: "cloudflare",
    productUrl: "https://dev.geoguessme.test",
    apiUrl: `http://127.0.0.1:${port}`,
    address: "qa-release-20260815-1+run-test@geoguessme.com",
    allowedLinkOrigins: ["https://auth.geoguessme.com"],
    accessClientId: "dev-app-access-id",
    accessClientSecret: "dev-app-access-secret",
    ...explicitMailboxCredentials,
  });
  const cloudflareMailbox = await cloudflareGateway.create();
  const cloudflareFound = await cloudflareGateway.search({ mailbox_id: cloudflareMailbox.mailbox_id, subject_contains: "verify", wait_ms: 0 });
  if (cloudflareFound.messages.length !== 1) throw new Error("Cloudflare mailbox search did not find the fixture");
  const cloudflareSafe = await cloudflareGateway.read({ mailbox_id: cloudflareMailbox.mailbox_id, message_id: "message-qp" });
  if (cloudflareSafe.body.includes("synthetic-verification-token") || cloudflareFound.messages[0].preview.includes("synthetic-verification-token") || !cloudflareSafe.links_available.includes("verification")) {
    throw new Error("Cloudflare quoted-printable mailbox redaction contract failed");
  }
  const cloudflareLink = await cloudflareGateway.link({ mailbox_id: cloudflareMailbox.mailbox_id, message_id: "message-qp", kind: "verification" });
  if (new URL(cloudflareLink.url).searchParams.get("key") !== "synthetic-verification-token" || new URL(cloudflareLink.url).searchParams.get("client_id") !== "qa") {
    throw new Error("Cloudflare quoted-printable verification link was not decoded");
  }
  const base64Safe = await cloudflareGateway.read({ mailbox_id: cloudflareMailbox.mailbox_id, message_id: "message-1" });
  if (base64Safe.subject !== "Base64 fixture" || !base64Safe.links_available.includes("verification")) throw new Error("Cloudflare base64 fixture failed");
  await cloudflareGateway.cleanup();
  if (cloudflareGateway.mailboxes.size !== 0) throw new Error("Cloudflare mailbox cleanup contract failed");
  if (cloudflareAccessHeaders.length === 0 || cloudflareAccessHeaders.some(({ clientId, clientSecret }) => clientId !== "mailbox-access-id" || clientSecret !== "mailbox-access-secret")) {
    throw new Error("Cloudflare mailbox requests did not isolate mailbox Access credentials from app Access credentials");
  }
  console.log("Mailbox gateway contract PASSED");
} finally {
  server.close();
  await once(server, "close");
}
