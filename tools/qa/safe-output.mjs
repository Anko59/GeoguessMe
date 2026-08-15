export function redactUrls(value) {
  const absolute = String(value).replace(/https?:\/\/[^\s<>"']+/gi, (candidate) => {
    try {
      const url = new URL(candidate);
      if (/(?:token|secret|code|key|password|auth|cookie|invite|reset|verify)/i.test(`${url.pathname} ${url.search} ${url.hash}`)) return "[redacted-link]";
      return candidate;
    } catch {
      return "[redacted-link]";
    }
  });
  return absolute.replace(/(^|[\s"'(])((?:\/|#)[^\s<>"']*(?:token|secret|code|key|password|auth|cookie|invite|reset|verify)[^\s<>"']*)/gi, "$1[redacted-link]");
}
