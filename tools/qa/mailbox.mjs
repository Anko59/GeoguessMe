import { randomUUID } from "node:crypto";

const mailTmApi = "https://api.mail.tm";
const linkPatterns = {
  verification: /(?:verify|confirm|email)/i,
  "group-invite": /(?:invite|group)/i,
  "password-reset": /(?:reset|password)/i,
};

export class MailboxGateway {
  constructor({ provider = "mailtm", productUrl, apiUrl = mailTmApi }) {
    this.provider = provider;
    this.apiUrl = apiUrl;
    this.productOrigin = new URL(productUrl).origin;
    this.mailboxes = new Map();
    this.lastRequestAt = 0;
  }

  async create() {
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
    return safeMessage(mailboxId, message, this.productOrigin);
  }

  async link({ mailbox_id: mailboxId, message_id: messageId, kind = "any" }) {
    if (!Object.hasOwn(linkPatterns, kind) && kind !== "any") {
      throw new Error("Mailbox link kind must be verification, group-invite, password-reset, or any");
    }
    const message = await this.message(mailboxId, messageId);
    const candidates = extractUrls(`${message.text || ""}\n${message.html || ""}`);
    const candidate = candidates.find((value) => {
      const url = new URL(value);
      if (url.origin !== this.productOrigin) return false;
      return kind === "any" || linkPatterns[kind].test(`${url.pathname} ${url.search}`);
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
    for (const mailbox of this.mailboxes.values()) {
      if (!mailbox.accountId) continue;
      await this.request(`/accounts/${encodeURIComponent(mailbox.accountId)}`, { method: "DELETE", token: mailbox.token }).catch(() => {});
    }
    this.mailboxes.clear();
  }

  async message(mailboxId, messageId) {
    const mailbox = this.mailbox(mailboxId);
    if (!/^[A-Za-z0-9-]+$/.test(messageId || "")) throw new Error("Invalid QA mailbox message id");
    return this.request(`/messages/${encodeURIComponent(messageId)}`, { token: mailbox.token });
  }

  async request(path, { method = "GET", body, token } = {}) {
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
        },
        ...(body ? { body: JSON.stringify(body) } : {}),
        signal: AbortSignal.timeout(15000),
      });
      if (response.ok) return response.status === 204 ? null : response.json();
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

function safeMessage(mailboxId, message, productOrigin) {
  const body = plainText(`${message.text || ""}\n${message.html || ""}`);
  return {
    mailbox_id: mailboxId,
    message_id: message.id,
    subject: safeText(message.subject),
    from: safeText(message.from?.address),
    to: (message.to || []).map((recipient) => safeText(recipient.address)),
    created_at: message.createdAt,
    body: redactMailboxText(body),
    links_available: availableLinkKinds(`${message.text || ""}\n${message.html || ""}`, productOrigin),
  };
}

function extractUrls(value) {
  return [...String(value).replaceAll("&amp;", "&").matchAll(/https?:\/\/[^\s<>"']+/gi)].map((match) => match[0].replace(/[),.;]+$/, ""));
}

function availableLinkKinds(value, productOrigin) {
  const urls = extractUrls(value).filter((candidate) => {
    try { return new URL(candidate).origin === productOrigin; } catch { return false; }
  });
  return [...new Set(Object.keys(linkPatterns).filter((kind) => urls.some((url) => linkPatterns[kind].test(url))))];
}

function plainText(value) {
  return String(value)
    .replace(/<style[\s\S]*?<\/style\s*>|<script[\s\S]*?<\/script\s*>/gi, " ")
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
