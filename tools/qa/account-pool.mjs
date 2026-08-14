const roles = ["owner", "member", "outsider"];

function configuredAccount(role) {
  if (!roles.includes(role)) throw new Error("account_role must be owner, member, or outsider");
  const suffix = role.toUpperCase();
  const username = process.env[`QA_ACCOUNT_${suffix}_USERNAME`] || `qa_release_${role}`;
  const password = process.env.QA_ACCOUNT_PASSWORD;
  if (!password) throw new Error("Dedicated QA account pool is not configured; QA_ACCOUNT_PASSWORD is missing");
  return { username, password };
}

export async function loginAccount(page, baseUrl, role) {
  const account = configuredAccount(role);
  await page.goto(new URL("/login", baseUrl), { waitUntil: "domcontentloaded", timeout: 30000 });
  await page.getByLabel("Username").fill(account.username);
  await page.getByLabel("Password").fill(account.password);
  await page.getByRole("button", { name: "Login" }).click();
  await page.waitForURL((url) => url.origin === baseUrl.origin && url.pathname === "/groups", { timeout: 15000 });
  return { account_role: role, authenticated: true };
}
