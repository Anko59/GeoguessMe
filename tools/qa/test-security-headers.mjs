import assert from "node:assert/strict";
import { inspectDocumentResponse, summarizeSecurityHeaders } from "./security-headers.mjs";

const sensitive = "sensitive-nonce-value";
const summary = summarizeSecurityHeaders({
  "content-security-policy": `default-src 'self'; script-src 'nonce-${sensitive}' 'unsafe-inline'; connect-src https://example.test`,
  "strict-transport-security": "max-age=31536000; includeSubDomains",
  "set-cookie": `session=${sensitive}`,
  location: `/?token=${sensitive}`,
  "x-frame-options": "DENY",
});
assert.equal(summary.present["content-security-policy"], true);
assert.equal(summary.present["referrer-policy"], false);
assert.equal(summary.present["x-frame-options"], true);
assert.deepEqual(summary.csp_directives, ["connect-src", "default-src", "script-src"]);
assert.equal(summary.csp_unsafe_inline, true);
assert.equal(summary.csp_unsafe_eval, false);
assert.equal(summary.hsts_includes_subdomains, true);
assert.equal(JSON.stringify(summary).includes(sensitive), false);
assert.equal(JSON.stringify(summary).includes("set-cookie"), false);

const page = { mainFrame: () => "top" };
const baseUrl = new URL("https://dev.example.test/");
const response = (resourceType, frame, url) => ({
  url: () => url,
  status: () => 200,
  request: () => ({ resourceType: () => resourceType, frame: () => frame }),
  allHeaders: async () => ({ "referrer-policy": "no-referrer", "set-cookie": `secret=${sensitive}` }),
});
assert.equal(await inspectDocumentResponse(response("xhr", "top", baseUrl.href), page, baseUrl), null);
assert.equal(await inspectDocumentResponse(response("document", "subframe", baseUrl.href), page, baseUrl), null);
assert.equal(await inspectDocumentResponse(response("document", "top", "https://other.example.test/"), page, baseUrl), null);
const inspected = await inspectDocumentResponse(response("document", "top", baseUrl.href), page, baseUrl);
assert.equal(inspected.status, 200);
assert.equal(inspected.present["referrer-policy"], true);
assert.equal(JSON.stringify(inspected).includes(sensitive), false);
console.log("QA security-header inventory PASSED");
