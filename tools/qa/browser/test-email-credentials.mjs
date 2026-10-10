import assert from "node:assert/strict";
import { EmailCredentials } from "./email-credentials.mjs";

const baseUrl = "https://dev.example.test";
const identityUrl = "https://auth.example.test";
const mailbox = { create: async () => ({ mailbox_id: "owned-mailbox", address: "qa@example.test" }) };

function fakePage({ oidc = false, unsafeIdentity = false, loginFailure = false, fieldCount } = {}) {
  const state = { url: `${baseUrl}/`, fields: new Map(), mode: "", cookiesCleared: 0 };
  const fieldName = (label) => {
    const name = String(label).toLowerCase();
    if (name.includes("confirm")) return "confirm";
    if (name.includes("new password")) return "new";
    if (name.includes("password")) return "password";
    return String(label);
  };
  const page = {
    state,
    url: () => state.url,
    context: () => ({ clearCookies: async () => { state.cookiesCleared++; } }),
    goto: async (url) => { state.url = url; state.mode = new URL(url).pathname.slice(1); },
    locator: () => {
      const field = (index) => ({
        fill: async (value) => state.fields.set(state.mode === "login" ? "password" : index ? "confirm" : "new", value),
        waitFor: async ({ state: expected }) => { if (expected === "hidden") assert.ok(!["reset", "update-password"].includes(state.mode)); },
      });
      return { count: async () => fieldCount ?? (oidc && state.mode !== "login" ? 2 : 1), first: () => field(0), nth: field };
    },
    getByPlaceholder: (label) => ({
      count: async () => label === "Username" && !oidc ? 1 : 0,
      fill: async (value) => state.fields.set(fieldName(label), value),
    }),
    getByLabel: (label) => ({
      count: async () => oidc && state.mode === "update-password" ? 1 : 0,
      check: async () => {},
      fill: async (value) => {
        if (label instanceof RegExp) assert.equal(label.test(fieldName(label) === "confirm" ? "Confirm password *" : "New Password *"), true);
        state.fields.set(fieldName(label), value);
      },
      waitFor: async ({ state: expected }) => {
        if (expected === "hidden") assert.notEqual(state.mode, "reset");
      },
    }),
    getByRole: (_role, { name }) => ({
      or: () => ({ first: () => ({ waitFor: async () => {} }) }),
      count: async () => oidc && name === "Create account" && state.mode === "registration" ? 1 : 0,
      click: async () => {
        const origin = unsafeIdentity ? "https://untrusted.example.test" : identityUrl;
        if (name === "Continue to create account") { state.url = `${origin}/registration`; state.mode = "registration"; }
        if (name === "Create account") { state.url = `${origin}/update-password`; state.mode = "update-password"; }
        if (name === "Continue to password") { state.url = `${origin}/login`; state.mode = "login"; }
        if (["Sign Up", "Login", "Sign In"].includes(name)) { state.url = `${baseUrl}/groups`; state.mode = "groups"; }
        if (["Submit", "Reset password"].includes(name)) { state.url = `${origin}/login`; state.mode = "login"; }
      },
    }),
    waitForLoadState: async () => {},
    waitForURL: async (predicate) => {
      if (loginFailure) throw new Error("Sign-in was not confirmed");
      assert.equal(predicate(new URL(state.url)), true);
    },
  };
  return page;
}

for (const oidc of [false, true]) {
  const credentials = new EmailCredentials({ baseUrl, identityOrigins: [identityUrl] });
  const page = fakePage({ oidc });
  const result = await credentials.signup({ page, mailbox, accountRole: "owner" });
  const original = page.state.fields.get(oidc ? "new" : "password");
  assert.match(original, /^Qa[a-f0-9]{20}1$/);
  assert.equal(JSON.stringify(result).includes(original), false);
  assert.equal(credentials.redact(`unexpected reflected value ${original}`), "unexpected reflected value [redacted]");
  await assert.rejects(() => credentials.signup({ page, mailbox, accountRole: "member" }), /Only one/);
  await assert.rejects(() => credentials.reset({ page, mailboxId: "pool-account" }), /Only this run/);
  await assert.rejects(() => credentials.reset({ page, mailboxId: result.mailbox_id }), /mailbox link/);

  page.state.url = `${oidc ? identityUrl : baseUrl}/settings`;
  assert.throws(() => credentials.authorizeReset(result.mailbox_id, page), /credential-reset route/);
  page.state.url = `${oidc ? `${identityUrl}/realms/geoguessme/login-actions/required-action` : `${baseUrl}/reset-password`}?token=private-reset`;
  page.state.mode = "reset";
  credentials.authorizeReset(result.mailbox_id, page);
  await assert.rejects(() => credentials.reset({ page: fakePage({ oidc }), mailboxId: result.mailbox_id }), /this tab/);
  page.state.url += "-changed";
  await assert.rejects(() => credentials.reset({ page, mailboxId: result.mailbox_id }), /this tab/);
  credentials.authorizeReset(result.mailbox_id, page);
  const submitted = await credentials.reset({ page, mailboxId: result.mailbox_id });
  const changed = page.state.fields.get("new");
  assert.notEqual(changed, original);
  assert.equal(submitted.changed_password_verified, false);
  assert.equal(JSON.stringify(submitted).includes(changed), false);
  if (oidc) assert.equal(page.state.fields.get("confirm"), changed);
  await assert.rejects(() => credentials.reset({ page, mailboxId: result.mailbox_id }), /mailbox link/);
  await assert.rejects(() => credentials.login({ page: fakePage({ oidc, loginFailure: true }), mailboxId: result.mailbox_id }), /not confirmed/);

  const loginPage = fakePage({ oidc });
  const verified = await credentials.login({ page: loginPage, mailboxId: result.mailbox_id });
  assert.equal(verified.authenticated, true);
  assert.equal(verified.changed_password_verified, true);
  assert.equal(loginPage.state.cookiesCleared, 1);
  assert.equal(loginPage.state.fields.get("password"), changed);
  assert.equal(JSON.stringify(verified).includes(changed), false);
  assert.equal(credentials.redact(`${original} ${changed}`), "[redacted] [redacted]");
  assert.equal((await credentials.login({ page: fakePage({ oidc }), mailboxId: result.mailbox_id })).changed_password_verified, false);
  if (oidc) {
    const unsafe = fakePage({ oidc, unsafeIdentity: true });
    await assert.rejects(() => credentials.login({ page: unsafe, mailboxId: result.mailbox_id }), /origin is blocked/);
    assert.equal(unsafe.state.fields.has("password"), false);
  }
  const unexpected = fakePage({ oidc, fieldCount: 3 });
  await assert.rejects(() => credentials.login({ page: unexpected, mailboxId: result.mailbox_id }), /Unexpected credential form/);
  assert.equal(unexpected.state.fields.has("password"), false);
  credentials.clear();
  assert.equal(credentials.owns(result.mailbox_id), false);
  await assert.rejects(() => credentials.login({ page, mailboxId: result.mailbox_id }), /Only this run/);
}

const rejected = new EmailCredentials({ baseUrl, identityOrigins: [identityUrl] });
const unsafeSignup = fakePage({ oidc: true, unsafeIdentity: true });
await assert.rejects(() => rejected.signup({ page: unsafeSignup, mailbox, accountRole: "owner" }), /origin is blocked/);
assert.equal(unsafeSignup.state.fields.has("new"), false);
await assert.rejects(() => new EmailCredentials({ baseUrl }).signup({ page: fakePage({ oidc: true }), mailbox, accountRole: "owner" }), /origin is blocked/);
console.log("QA owned email credential reset, sign-in, origin binding and redaction contracts PASSED");
