import { createHash, randomUUID } from "node:crypto";
import { assertCredentialOrigin, passwordFields, signUpEmailAccount } from "../email-account.mjs";

const fingerprint = (page) => createHash("sha256").update(page.url()).digest("hex");
const password = () => `Qa${randomUUID().replaceAll("-", "").slice(0, 20)}1`;

// Only the one disposable mailbox-backed account created by this run is held
// here. Pool credentials and arbitrary product accounts cannot be reset.
export class EmailCredentials {
  #accounts = new Map();
  #resetGrants = new Map();
  #secrets = new Set();
  #signupStarted = false;

  constructor({ baseUrl, identityOrigins = [] }) {
    this.baseUrl = new URL(baseUrl);
    this.identityOrigins = new Set(identityOrigins.map((origin) => new URL(origin).origin));
  }

  #account(mailboxId) {
    const account = this.#accounts.get(mailboxId);
    if (!account) throw new Error("Only this run's mailbox-backed account may use the credential helper");
    return account;
  }

  #assertOrigin(page, mode) {
    assertCredentialOrigin(page, this.baseUrl, this.identityOrigins, mode);
  }

  owns(mailboxId) { return this.#accounts.has(mailboxId); }

  async signup({ page, mailbox, accountRole }) {
    if (this.#signupStarted) throw new Error("Only one mailbox-backed signup is allowed per QA run");
    this.#signupStarted = true;
    return signUpEmailAccount({
      page, mailbox, accountRole, baseUrl: this.baseUrl, identityOrigins: this.identityOrigins,
      retainCredential: (account) => {
        this.#secrets.add(account.password);
        this.#accounts.set(account.mailboxId, account);
      },
    });
  }

  authorizeReset(mailboxId, page) {
    const account = this.#account(mailboxId);
    this.#assertOrigin(page, account.mode);
    const path = new URL(page.url()).pathname;
    const validPath = account.mode === "oidc"
      ? /^\/realms\/[^/]+\/login-actions\/(action-token|required-action|reset-credentials)$/.test(path)
      : path === "/reset-password";
    if (!validPath) throw new Error("This mailbox link did not open the account's credential-reset route");
    this.#resetGrants.set(mailboxId, { page, fingerprint: fingerprint(page) });
  }

  async reset({ page, mailboxId }) {
    const account = this.#account(mailboxId);
    const grant = this.#resetGrants.get(mailboxId);
    if (!grant || grant.page !== page || grant.fingerprint !== fingerprint(page)) {
      throw new Error("Open this account's password-reset mailbox link in this tab first");
    }
    this.#resetGrants.delete(mailboxId);
    this.#assertOrigin(page, account.mode);
    const nextPassword = password();
    this.#secrets.add(nextPassword);
    const fields = await passwordFields(page, account.mode === "oidc" ? 2 : 1);
    const field = fields.first();
    await field.fill(nextPassword);
    if (account.mode === "oidc") await fields.nth(1).fill(nextPassword);
    account.pendingPassword = nextPassword;
    const resetUrl = page.url();
    await page.getByRole("button", { name: account.mode === "oidc" ? "Submit" : "Reset password", exact: true }).click();
    await page.waitForURL((url) => url.toString() !== resetUrl, { timeout: 15000 });
    return { mailbox_id: mailboxId, reset_submitted: true, changed_password_verified: false };
  }

  async login({ page, mailboxId }) {
    const account = this.#account(mailboxId);
    // Discard only this isolated QA context's cookies so SSO cannot substitute
    // for actually entering and verifying the held password.
    await page.context().clearCookies();
    await page.goto(new URL("/login", this.baseUrl).toString(), { waitUntil: "domcontentloaded", timeout: 30000 });
    if (account.mode === "oidc") {
      await page.getByLabel("Email address", { exact: true }).fill(account.address);
      await page.getByRole("button", { name: "Continue to password", exact: true }).click();
    } else {
      await page.getByLabel("Username or email", { exact: true }).fill(account.address);
    }
    const field = (await passwordFields(page, 1)).first();
    this.#assertOrigin(page, account.mode);
    await field.fill(account.pendingPassword || account.password);
    await page.getByRole("button", { name: "Login", exact: true }).click();
    await page.waitForURL((url) => url.origin === this.baseUrl.origin && url.pathname === "/groups", { timeout: 15000 });
    const changedPasswordVerified = Boolean(account.pendingPassword);
    if (changedPasswordVerified) {
      account.password = account.pendingPassword;
      delete account.pendingPassword;
    }
    return { mailbox_id: mailboxId, account_role: account.accountRole, authenticated: true, changed_password_verified: changedPasswordVerified };
  }

  redact(value) {
    let text = String(value);
    for (const secret of this.#secrets) text = text.split(secret).join("[redacted]");
    return text;
  }

  clear() {
    this.#accounts.clear();
    this.#resetGrants.clear();
    this.#secrets.clear();
  }
}
