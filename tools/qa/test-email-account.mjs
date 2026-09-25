import assert from "node:assert/strict";
import { signUpEmailAccount } from "./email-account.mjs";

function fakePage({ oidc }) {
  const state = { url: "", fields: new Map(), checked: [], clicked: [] };
  return {
    state,
    url: () => state.url,
    goto: async (url) => { state.url = url; },
    getByPlaceholder: (label) => ({
      count: async () => label === "Username" && !oidc ? 1 : 0,
      fill: async (value) => state.fields.set(label, value),
    }),
    getByLabel: (label) => ({
      count: async () => oidc && ["New Password", "Confirm password"].includes(String(label)) && state.url.endsWith("update-password") ? 1 : 0,
      check: async () => state.checked.push(String(label)),
      fill: async (value) => state.fields.set(String(label), value),
    }),
    getByRole: (_role, options) => ({
      count: async () => oidc && options.name === "Create account" && state.url.endsWith("registration") ? 1 : 0,
      click: async () => {
        state.clicked.push(options.name);
        if (options.name === "Sign Up") state.url = "https://dev.geoguessme.test/groups";
        if (options.name === "Continue to create account") state.url = "https://auth.geoguessme.test/realms/geoguessme/login-actions/registration";
        if (options.name === "Create account") state.url = "https://auth.geoguessme.test/realms/geoguessme/login-actions/update-password";
      },
    }),
    waitForURL: async (predicate) => assert.equal(predicate(new URL("https://dev.geoguessme.test/groups")), true),
    waitForLoadState: async () => {},
  };
}

const mailbox = { create: async () => ({ mailbox_id: "mailbox-1", address: "qa@example.test" }) };
const baseUrl = new URL("https://dev.geoguessme.test/");

const oidcPage = fakePage({ oidc: true });
const oidcResult = await signUpEmailAccount({ page: oidcPage, baseUrl, mailbox, accountRole: "owner" });
assert.equal(oidcResult.authenticated, false);
assert.equal(oidcResult.verification_required, true);
assert.equal(oidcResult.address, "qa@example.test");
assert.equal(oidcPage.state.fields.get("Email address"), "qa@example.test");
assert.match(oidcPage.state.fields.get("New Password"), /^Qa[a-f0-9]{20}1$/);
assert.equal(oidcPage.state.fields.get("Confirm password"), oidcPage.state.fields.get("New Password"));
assert.deepEqual(oidcPage.state.clicked, ["Continue to create account", "Create account", "Submit"]);

const legacyPage = fakePage({ oidc: false });
const legacyResult = await signUpEmailAccount({ page: legacyPage, baseUrl, mailbox, accountRole: "member" });
assert.equal(legacyResult.authenticated, true);
assert.equal(legacyPage.state.fields.get("Email — verify to enable account recovery"), "qa@example.test");
assert.equal(legacyPage.state.checked.length, 1);
assert.deepEqual(legacyPage.state.clicked, ["Sign Up"]);

console.log("QA email-account signup contract PASSED");
