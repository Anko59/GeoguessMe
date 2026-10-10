import { randomUUID } from "node:crypto";

const accountRoles = ["owner", "member", "outsider"];

export function assertCredentialOrigin(page, baseUrl, identityOrigins, mode) {
  const origin = new URL(page.url()).origin;
  const allowed = mode === "oidc" ? identityOrigins.has(origin) : origin === new URL(baseUrl).origin;
  if (!allowed) throw new Error("Credential entry outside the configured account origin is blocked");
}

export async function passwordFields(page, expected) {
  const fields = page.locator('input[type="password"]:visible');
  await fields.first().waitFor({ state: "visible", timeout: 15000 });
  if (await fields.count() !== expected) throw new Error("Unexpected credential form; password entry is blocked");
  return fields;
}

export async function signUpEmailAccount({ page, baseUrl, mailbox, accountRole, retainCredential = () => {}, identityOrigins = new Set() }) {
  if (!accountRoles.includes(accountRole)) throw new Error("account_role must be owner, member, or outsider");
  const mailboxAccount = await mailbox.create();
  const username = `qa_email_${Date.now().toString(36)}${randomUUID().replaceAll("-", "").slice(0, 8)}`;
  const password = `Qa${randomUUID().replaceAll("-", "").slice(0, 20)}1`;
  await page.goto(new URL("/signup", baseUrl).toString(), { waitUntil: "domcontentloaded", timeout: 30000 });
  await page.getByLabel(/I confirm I am at least 15 years old/).check();
  const legacyUsername = page.getByPlaceholder("Username", { exact: true });
  if (await legacyUsername.count()) {
    retainCredential({ mailboxId: mailboxAccount.mailbox_id, address: mailboxAccount.address, username, password, accountRole, mode: "direct" });
    await legacyUsername.fill(username);
    await page.getByPlaceholder("Email — verify to enable account recovery", { exact: true }).fill(mailboxAccount.address);
    assertCredentialOrigin(page, baseUrl, identityOrigins, "direct");
    await page.getByPlaceholder("Password", { exact: true }).fill(password);
    await page.getByRole("button", { name: "Sign Up", exact: true }).click();
    await page.waitForURL((url) => url.origin === baseUrl.origin && url.pathname === "/groups", { timeout: 15000 });
    return { account_role: accountRole, mailbox_id: mailboxAccount.mailbox_id, address: mailboxAccount.address, authenticated: true };
  }

  retainCredential({ mailboxId: mailboxAccount.mailbox_id, address: mailboxAccount.address, username, password, accountRole, mode: "oidc" });
  await page.getByLabel("Email address", { exact: true }).fill(mailboxAccount.address);
  await page.getByRole("button", { name: "Continue to create account", exact: true }).click();
  await page.waitForURL((url) => url.origin !== new URL(baseUrl).origin, { timeout: 15000 });
  assertCredentialOrigin(page, baseUrl, identityOrigins, "oidc");
  const createAccount = page.getByRole("button", { name: "Create account", exact: true });
  await createAccount.or(page.getByRole("button", { name: "Submit", exact: true })).first().waitFor({ state: "visible", timeout: 15000 });
  if (await createAccount.count()) {
    await createAccount.click();
    await page.waitForLoadState("domcontentloaded");
  }
  const fields = await passwordFields(page, 2);
  assertCredentialOrigin(page, baseUrl, identityOrigins, "oidc");
  await fields.nth(0).fill(password);
  await fields.nth(1).fill(password);
  await page.getByRole("button", { name: "Submit", exact: true }).click();
  await fields.first().waitFor({ state: "hidden", timeout: 15000 });
  return {
    account_role: accountRole,
    mailbox_id: mailboxAccount.mailbox_id,
    address: mailboxAccount.address,
    authenticated: false,
    verification_required: true,
  };
}
