import { randomUUID } from "node:crypto";

const roles = ["owner", "member", "outsider"];
const provisionedAccounts = new Map();

function configuredAccount(role) {
  if (!roles.includes(role)) throw new Error("account_role must be owner, member, or outsider");
  const suffix = role.toUpperCase();
  const username = process.env[`QA_ACCOUNT_${suffix}_USERNAME`] || `qa_release_${role}`;
  const password = process.env.QA_ACCOUNT_PASSWORD;
  return password ? { username, password } : null;
}

function provisionedAccount(role) {
  const suffix = `${Date.now().toString(36)}${randomUUID().replaceAll("-", "").slice(0, 8)}`;
  return {
    username: `qa_${role}_${suffix}`,
    password: `Qa${randomUUID().replaceAll("-", "").slice(0, 20)}1`,
  };
}

async function signUpAccount(page, baseUrl, role) {
  const account = provisionedAccount(role);
  await page.goto(new URL("/signup", baseUrl).toString(), { waitUntil: "domcontentloaded", timeout: 30000 });
  await page.getByLabel("Username").fill(account.username);
  await page.getByLabel("Password").fill(account.password);
  await page.getByRole("button", { name: "Sign Up" }).click();
  await page.waitForURL((url) => url.origin === baseUrl.origin && url.pathname === "/groups", { timeout: 15000 });
  provisionedAccounts.set(role, account);
  return account;
}

export async function loginAccount(page, baseUrl, role) {
  let account = configuredAccount(role);
  if (!account) {
    account = provisionedAccounts.get(role) || await signUpAccount(page, baseUrl, role);
    if (page.url && new URL(page.url()).pathname === "/groups") {
      return { account_role: role, authenticated: true };
    }
  }
  await page.goto(new URL("/login", baseUrl).toString(), { waitUntil: "domcontentloaded", timeout: 30000 });
  await page.getByLabel("Username").fill(account.username);
  await page.getByLabel("Password").fill(account.password);
  await page.getByRole("button", { name: "Login" }).click();
  await page.waitForURL((url) => url.origin === baseUrl.origin && url.pathname === "/groups", { timeout: 15000 });
  return { account_role: role, authenticated: true };
}
