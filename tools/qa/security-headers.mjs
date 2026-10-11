// Inspect only fixed security-header names on the top-level document response.
// Never return raw header values: CSP can contain per-request nonces and other
// headers (notably Set-Cookie and Location) can carry credentials.
const names = [
  "content-security-policy", "content-security-policy-report-only",
  "strict-transport-security", "referrer-policy", "permissions-policy",
  "x-content-type-options", "x-frame-options", "cross-origin-opener-policy",
  "cross-origin-embedder-policy", "cross-origin-resource-policy",
];

const knownDirectives = new Set([
  "default-src", "script-src", "script-src-elem", "script-src-attr", "style-src",
  "style-src-elem", "style-src-attr", "img-src", "connect-src", "font-src",
  "media-src", "object-src", "frame-src", "child-src", "worker-src",
  "manifest-src", "form-action", "frame-ancestors", "base-uri", "navigate-to",
  "upgrade-insecure-requests", "block-all-mixed-content", "report-uri", "report-to",
]);

export function summarizeSecurityHeaders(headers) {
  const present = Object.fromEntries(names.map((name) => [name, Object.hasOwn(headers, name)]));
  const csp = headers["content-security-policy"] || "";
  const directives = csp.split(";").map((part) => part.trim().split(/\s+/, 1)[0].toLowerCase()).filter((name) => knownDirectives.has(name));
  return {
    present,
    csp_directives: [...new Set(directives)].sort(),
    csp_unsafe_inline: /(?:^|\s)'unsafe-inline'(?:\s|;|$)/i.test(csp),
    csp_unsafe_eval: /(?:^|\s)'unsafe-eval'(?:\s|;|$)/i.test(csp),
    hsts_includes_subdomains: /(?:^|;)\s*includesubdomains\s*(?:;|$)/i.test(headers["strict-transport-security"] || ""),
  };
}

export async function inspectDocumentResponse(response, page, baseUrl) {
  const request = response.request();
  if (request.resourceType() !== "document" || request.frame() !== page.mainFrame()) return null;
  const url = new URL(response.url());
  if (url.origin !== baseUrl.origin) return null;
  const headers = await response.allHeaders();
  return { status: response.status(), ...summarizeSecurityHeaders(headers) };
}
