import { randomUUID } from "node:crypto";

const mailTmApi = "https://api.mail.tm";
const linkPatterns = {
  verification: /(?:verify|confirm|email)/i,
  "group-invite": /(?:invite|group)/i,
  "password-reset": /(?:reset|password)/i,
};

export function resolveMailboxAccessCredentials({ provider = "mailtm", productUrl, apiUrl, appClientId = "", appClientSecret = "", mailboxClientId = "", mailboxClientSecret = "" }) {
  if (Boolean(mailboxClientId) !== Boolean(mailboxClientSecret)) {
    throw new Error("QA_MAILBOX_ACCESS_CLIENT_ID and QA_MAILBOX_ACCESS_CLIENT_SECRET must be supplied together");
  }
  if (mailboxClientId) {
    return { mailboxAccessClientId: mailboxClientId, mailboxAccessClientSecret: mailboxClientSecret };
  }
  const resolvedApiUrl = apiUrl || (provider === "mailtm" ? mailTmApi : "");
  if (resolvedApiUrl && new URL(resolvedApiUrl, productUrl).origin === new URL(productUrl).origin) {
    return { mailboxAccessClientId: appClientId, mailboxAccessClientSecret: appClientSecret };
  }
  return { mailboxAccessClientId: "", mailboxAccessClientSecret: "" };
}

export class MailboxGateway {
  constructor({ provider = "mailtm", productUrl, apiUrl = mailTmApi, address = "", mailboxAccessClientId = "", mailboxAccessClientSecret = "", allowedLinkOrigins = [] }) {
    this.provider = provider;
    this.apiUrl = String(apiUrl || (provider === "mailtm" ? mailTmApi : "")).replace(/\/$/, "");
    this.productOrigin = new URL(productUrl).origin;
    this.allowedLinkOrigins = new Set([this.productOrigin, ...allowedLinkOrigins.map((origin) => new URL(origin).origin)]);
    this.fixedAddress = address;
    this.mailboxAccessClientId = mailboxAccessClientId;
    this.mailboxAccessClientSecret = mailboxAccessClientSecret;
    this.mailboxes = new Map();
    this.lastRequestAt = 0;
  }

  async create() {
    if (this.provider === "cloudflare") {
      const match = /^(?<local>[A-Za-z0-9][A-Za-z0-9._+-]*)@(?<domain>[A-Za-z0-9.-]+)$/.exec(this.fixedAddress);
      if (!match || !this.apiUrl) throw new Error("Cloudflare QA mailbox requires QA_MAILBOX_ADDRESS and QA_MAILBOX_API_URL");
      const mailboxId = "mailbox-1";
      this.mailboxes.set(mailboxId, { address: this.fixedAddress, localPart: match.groups.local });
      return { mailbox_id: mailboxId, address: this.fixedAddress, provider: "cloudflare-worker" };
    }
    if (this.provider !== "mailtm") throw new Error(`Unsupported QA mailbox provider: ${this.provider}`);
    const domains = await this.request("/domains");
    const entries = Array.isArray(domains) ? domains : (Array.isArray(domains["hydra:member"]) ? domains["hydra:member"] : []);
    const domain = entries.find((entry) => entry.isActive && !entry.isPrivate)?.domain;
    if (!domain) throw new Error(`QA mailbox provider returned no active public domain (entries=${entries.length}, keys=${Object.keys(domains).join(",")})`);
    const localPart = `qa${Date.now().toString(36)}${randomUUID().replaceAll("-", "").slice(0, 10)}`;
    const address = `${localPart}@${domain}`;
    const password = `${randomUUID()}Qa!`;
    const account = await this.request("/accounts", { method: "POST", body: { address, password } });
    const tokenResponse = await this.request("/token", { method: "POST", body: { address, password } });
    const mailboxId = `mailbox-${this.mailboxes.size + 1}`;
    this.mailboxes.set(mailboxId, { accountId: account.id, address, token: tokenResponse.token });
    return { mailbox_id: mailboxId, address, provider: "mail.tm" };
  }

  async search({ mailbox_id: mailboxId, subject_contains: subject, from_contains: from, since, wait_ms: waitMs = 30000 }) {
    const mailbox = this.mailbox(mailboxId);
    const deadline = Date.now() + Math.min(Math.max(Number(waitMs) || 0, 0), 60000);
    if (this.provider === "cloudflare") {
      let messages = [];
      do {
        // The Worker matches the literal path. Percent-encoding '+' would
        // produce '%2B', which its path matcher does not decode.
        const listed = await this.request(`/v1/inbox/${mailbox.localPart}`);
        messages = [];
        for (const entry of Array.isArray(listed?.messages) ? listed.messages : []) {
          const message = await this.cloudflareMessage(mailbox, entry.id);
          if (!matchesMessage(message, { subject, from, since })) continue;
          messages.push({
            message_id: entry.id,
            subject: safeText(message.subject),
            from: safeText(message.from?.address),
            created_at: message.createdAt,
            preview: redactMailboxText(message.intro),
          });
        }
        if (messages.length || Date.now() >= deadline) break;
        await new Promise((resolve) => setTimeout(resolve, 1000));
      } while (Date.now() < deadline);
      return { mailbox_id: mailboxId, address: mailbox.address, messages: messages.slice(0, 20) };
    }
    let messages = [];
    do {
      const response = await this.request("/messages", { token: mailbox.token });
      messages = (response["hydra:member"] || []).filter((message) => matchesMessage(message, { subject, from, since }));
      if (messages.length || Date.now() >= deadline) break;
      await new Promise((resolve) => setTimeout(resolve, 1000));
    } while (Date.now() < deadline);
    return {
      mailbox_id: mailboxId,
      address: mailbox.address,
      messages: messages.slice(0, 20).map((message) => ({
        message_id: message.id,
        subject: safeText(message.subject),
        from: safeText(message.from?.address),
        created_at: message.createdAt,
        preview: redactMailboxText(message.intro),
      })),
    };
  }

  async read({ mailbox_id: mailboxId, message_id: messageId }) {
    const message = await this.message(mailboxId, messageId);
    return safeMessage(mailboxId, message, this.allowedLinkOrigins);
  }

  async link({ mailbox_id: mailboxId, message_id: messageId, kind = "any" }) {
    if (!Object.hasOwn(linkPatterns, kind) && kind !== "any") {
      throw new Error("Mailbox link kind must be verification, group-invite, password-reset, or any");
    }
    const message = await this.message(mailboxId, messageId);
    const candidates = extractUrls(`${message.text || ""}\n${message.html || ""}`);
    const candidate = candidates.find((value) => {
      const url = new URL(value);
      if (!this.allowedLinkOrigins.has(url.origin)) return false;
      return kind === "any" || matchesLinkKind(url, kind, message.subject);
    });
    if (!candidate) throw new Error("No matching safe product link was found in that mailbox message");
    return { mailbox_id: mailboxId, message_id: messageId, kind, url: candidate };
  }

  mailbox(mailboxId) {
    const mailbox = this.mailboxes.get(mailboxId);
    if (!mailbox) throw new Error(`Unknown QA mailbox: ${mailboxId}`);
    return mailbox;
  }

  async cleanup() {
    if (this.provider === "cloudflare") {
      this.mailboxes.clear();
      return;
    }
    for (const mailbox of this.mailboxes.values()) {
      if (!mailbox.accountId) continue;
      await this.request(`/accounts/${encodeURIComponent(mailbox.accountId)}`, { method: "DELETE", token: mailbox.token }).catch(() => {});
    }
    this.mailboxes.clear();
  }

  async message(mailboxId, messageId) {
    const mailbox = this.mailbox(mailboxId);
    if (!/^[A-Za-z0-9-]+$/.test(messageId || "")) throw new Error("Invalid QA mailbox message id");
    if (this.provider === "cloudflare") return this.cloudflareMessage(mailbox, messageId);
    return this.request(`/messages/${encodeURIComponent(messageId)}`, { token: mailbox.token });
  }

  async cloudflareMessage(mailbox, messageId) {
    const raw = await this.request(`/v1/inbox/${mailbox.localPart}/message/${encodeURIComponent(messageId)}`, { raw: true });
    return { id: messageId, ...parseRawMessage(raw) };
  }

  async request(path, { method = "GET", body, token, raw = false } = {}) {
    for (let attempt = 0; attempt < 4; attempt += 1) {
      const throttle = this.lastRequestAt + 150 - Date.now();
      if (throttle > 0) await new Promise((resolve) => setTimeout(resolve, throttle));
      this.lastRequestAt = Date.now();
      const response = await fetch(`${this.apiUrl}${path}`, {
        method,
        headers: {
          Accept: "application/json",
          ...(body ? { "Content-Type": "application/json" } : {}),
          ...(token ? { Authorization: `Bearer ${token}` } : {}),
          ...(this.provider === "cloudflare" && this.mailboxAccessClientId ? { "CF-Access-Client-Id": this.mailboxAccessClientId } : {}),
          ...(this.provider === "cloudflare" && this.mailboxAccessClientSecret ? { "CF-Access-Client-Secret": this.mailboxAccessClientSecret } : {}),
        },
        ...(body ? { body: JSON.stringify(body) } : {}),
        signal: AbortSignal.timeout(15000),
      });
      if (response.ok) {
        if (response.status === 204) return null;
        if (raw) return response.arrayBuffer();
        return response.json();
      }
      if (response.status !== 429 && response.status < 500) throw new Error(`QA mailbox provider request failed (${response.status})`);
      if (attempt < 3) await new Promise((resolve) => setTimeout(resolve, (attempt + 1) * 1000));
    }
    throw new Error("QA mailbox provider request failed after retries");
  }
}

function matchesMessage(message, { subject, from, since }) {
  if (subject && !String(message.subject || "").toLowerCase().includes(String(subject).toLowerCase())) return false;
  if (from && !String(message.from?.address || "").toLowerCase().includes(String(from).toLowerCase())) return false;
  if (since && (!message.createdAt || Date.parse(message.createdAt) < Date.parse(since))) return false;
  return true;
}

function parseRawMessage(raw) {
  const source = Buffer.from(raw);
  if (source.length > 2 * 1024 * 1024) throw new Error("QA email exceeds the MIME parser limit");
  const { headers, body } = splitMimeHeaders(source);
  const parts = { text: [], html: [], count: 0 };
  collectMimeParts(headers, body, parts, 0);
  const text = parts.text.join("\n");
  const html = parts.html.join("\n");
  return {
    subject: decodeMimeHeader(headers.subject || ""),
    from: { address: extractAddress(headers.from || "") },
    to: [{ address: extractAddress(headers.to || "") }],
    createdAt: headers.date || new Date().toISOString(),
    intro: (text || plainText(html)).slice(0, 2000),
    text,
    html,
  };
}

function splitMimeHeaders(source) {
  const crlf = source.indexOf("\r\n\r\n");
  const lf = crlf < 0 ? source.indexOf("\n\n") : -1;
  const separator = crlf < 0 ? lf : crlf;
  const width = crlf < 0 ? 2 : 4;
  return {
    headers: parseHeaders(source.subarray(0, separator < 0 ? source.length : separator).toString("latin1")),
    body: separator < 0 ? Buffer.alloc(0) : source.subarray(separator + width),
  };
}

function collectMimeParts(headers, body, parts, depth) {
  if (depth > 5 || ++parts.count > 64) throw new Error("QA email has too many MIME parts");
  const contentType = headers["content-type"] || "text/plain";
  const mediaType = contentType.split(";", 1)[0].trim().toLowerCase();
  if (mediaType.startsWith("multipart/")) {
    const boundary = /(?:^|;)\s*boundary\s*=\s*(?:"([^"]+)"|([^;\s]+))/i.exec(contentType)?.slice(1).find(Boolean);
    if (!boundary) throw new Error("QA multipart email has no boundary");
    for (const part of splitMimeParts(body, boundary)) {
      const nested = splitMimeHeaders(part);
      collectMimeParts(nested.headers, nested.body, parts, depth + 1);
    }
    return;
  }
  if (mediaType !== "text/plain" && mediaType !== "text/html") return;
  const encoding = (headers["content-transfer-encoding"] || "").trim().toLowerCase();
  let decoded = body;
  if (encoding === "base64") decoded = Buffer.from(body.toString("latin1").replace(/\s+/g, ""), "base64");
  if (encoding === "quoted-printable") decoded = decodeQuotedPrintable(body);
  const charset = /(?:^|;)\s*charset\s*=\s*(?:"([^"]+)"|([^;\s]+))/i.exec(contentType)?.slice(1).find(Boolean) || "utf-8";
  let content;
  try { content = new TextDecoder(charset).decode(decoded); } catch { content = new TextDecoder().decode(decoded); }
  parts[mediaType === "text/html" ? "html" : "text"].push(content);
}

function splitMimeParts(body, boundary) {
  const marker = `--${boundary}`;
  const lines = body.toString("latin1").split(/\r?\n/);
  const result = [];
  let current = null;
  for (const line of lines) {
    if (line === marker || line === `${marker}--`) {
      if (current) result.push(Buffer.from(current.join("\r\n"), "latin1"));
      if (line === `${marker}--`) break;
      current = [];
    } else if (current) {
      current.push(line);
    }
  }
  return result;
}

function decodeQuotedPrintable(body) {
  const source = body.toString("latin1").replace(/=\r?\n/g, "");
  const bytes = [];
  for (let index = 0; index < source.length; index += 1) {
    if (source[index] === "=" && /^[0-9a-f]{2}$/i.test(source.slice(index + 1, index + 3))) {
      bytes.push(Number.parseInt(source.slice(index + 1, index + 3), 16));
      index += 2;
    } else {
      bytes.push(source.charCodeAt(index));
    }
  }
  return Buffer.from(bytes);
}

function parseHeaders(source) {
  const unfolded = source.replace(/\n[ \t]+/g, " ");
  return Object.fromEntries(unfolded.split("\n").flatMap((line) => {
    const separator = line.indexOf(":");
    if (separator < 1) return [];
    return [[line.slice(0, separator).toLowerCase(), line.slice(separator + 1).trim()]];
  }));
}

function decodeMimeHeader(value) {
  return String(value).replace(/=\?([^?]+)\?([bq])\?([^?]+)\?=/gi, (_match, charset, encoding, payload) => {
    if (encoding.toLowerCase() === "b") return Buffer.from(payload, "base64").toString(charset);
    const decoded = payload.replace(/_/g, " ").replace(/=([0-9a-f]{2})/gi, (_hex, code) => String.fromCharCode(Number.parseInt(code, 16)));
    return Buffer.from(decoded, "binary").toString(charset);
  });
}

function extractAddress(value) {
  const match = /<([^>]+)>/.exec(value);
  return safeText(match ? match[1] : value.split(",")[0].trim());
}

function safeMessage(mailboxId, message, allowedLinkOrigins) {
  const body = plainText(`${message.text || ""}\n${message.html || ""}`);
  return {
    mailbox_id: mailboxId,
    message_id: message.id,
    subject: safeText(message.subject),
    from: safeText(message.from?.address),
    to: (message.to || []).map((recipient) => safeText(recipient.address)),
    created_at: message.createdAt,
    body: redactMailboxText(body),
    links_available: availableLinkKinds(message, allowedLinkOrigins),
  };
}

function extractUrls(value) {
  return [...String(value).replaceAll("&amp;", "&").matchAll(/https?:\/\/[^\s<>"']+/gi)].map((match) => match[0].replace(/[),.;]+$/, ""));
}

function matchesLinkKind(url, kind, subject) {
  if (url.pathname.includes("/login-actions/action-token")) {
    if (/\b(?:reset|password)\b/i.test(subject || "")) return kind === "password-reset";
    if (/\b(?:verify|confirm|email)\b/i.test(subject || "")) return kind === "verification";
  }
  return linkPatterns[kind].test(`${url.pathname} ${url.search}`);
}

function availableLinkKinds(message, allowedLinkOrigins) {
  const urls = extractUrls(`${message.text || ""}\n${message.html || ""}`).filter((candidate) => {
    try { return allowedLinkOrigins.has(new URL(candidate).origin); } catch { return false; }
  });
  return Object.keys(linkPatterns).filter((kind) => urls.some((value) => matchesLinkKind(new URL(value), kind, message.subject)));
}

function plainText(value) {
  return String(value)
    .replace(/<style[\s\S]*?<\/style[^>]*>|<script[\s\S]*?<\/script[^>]*>/gi, " ")
    .replace(/<[^>]*>/g, " ")
    .replace(/&nbsp;/gi, " ")
    .replace(/&amp;/gi, "&")
    .replace(/\s+/g, " ")
    .trim();
}

function redactMailboxText(value) {
  return safeText(String(value)
    .replace(/https?:\/\/[^\s<>"']+/gi, "[link available via mailbox_open_link]")
    .replace(/\b(?:verification|reset|invite)?\s*(?:code|token)\s*[:=-]?\s*\S+/gi, "[email secret redacted]"));
}

function safeText(value) {
  return String(value ?? "").replace(/[\r\n]+/g, " ").slice(0, 2000);
}
