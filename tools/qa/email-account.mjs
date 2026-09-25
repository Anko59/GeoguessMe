import { randomUUID } from "node:crypto";

const accountRoles = ["owner", "member", "outsider"];

export async function signUpEmailAccount({ page, baseUrl, mailbox, accountRole }) {
  if (!accountRoles.includes(accountRole)) throw new Error("account_role must be owner, member, or outsider");
  const mailboxAccount = await mailbox.create();
  const username = `qa_email_${Date.now().toString(36)}${randomUUID().replaceAll("-", "").slice(0, 8)}`;
  const password = `Qa${randomUUID().replaceAll("-", "").slice(0, 20)}1`;
  await page.goto(new URL("/signup", baseUrl).toString(), { waitUntil: "domcontentloaded", timeout: 30000 });
  const legacyUsername = page.getByPlaceholder("Username", { exact: true });
  if (await legacyUsername.count()) {
    await legacyUsername.fill(username);
    await page.getByPlaceholder("Email — verify to enable account recovery", { exact: true }).fill(mailboxAccount.address);
    await page.getByPlaceholder("Password", { exact: true }).fill(password);
    await page.getByLabel(/I confirm I am at least 15 years old/).check();
    await page.getByRole("button", { name: "Sign Up", exact: true }).click();
    await page.waitForURL((url) => url.origin === baseUrl.origin && url.pathname === "/groups", { timeout: 15000 });
    return { account_role: accountRole, mailbox_id: mailboxAccount.mailbox_id, address: mailboxAccount.address, authenticated: true };
  }

  await page.getByLabel(/I confirm I am at least 15 years old/).check();
  await page.getByLabel("Email address", { exact: true }).fill(mailboxAccount.address);
  await page.getByRole("button", { name: "Continue to create account", exact: true }).click();
  await page.waitForLoadState("domcontentloaded").catch(() => {});
  const createAccount = page.getByRole("button", { name: "Create account", exact: true });
  if (await createAccount.count()) {
    await createAccount.click();
    await page.waitForLoadState("domcontentloaded").catch(() => {});
  }
  const newPassword = page.getByLabel("New Password", { exact: true });
  if (await newPassword.count()) {
    await newPassword.fill(password);
    await page.getByLabel("Confirm password", { exact: true }).fill(password);
    await page.getByRole("button", { name: "Submit", exact: true }).click();
    await page.waitForLoadState("domcontentloaded").catch(() => {});
  }
  return {
    account_role: accountRole,
    mailbox_id: mailboxAccount.mailbox_id,
    address: mailboxAccount.address,
    authenticated: false,
    verification_required: true,
  };
}
