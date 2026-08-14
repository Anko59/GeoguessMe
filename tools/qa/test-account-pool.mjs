import assert from "node:assert/strict";
import { loginAccount } from "./account-pool.mjs";

function fakePage() {
  const state = { urls: [], fields: new Map(), clicked: [] };
  const page = {
    state,
    url: () => state.urls.at(-1) || "https://dev.geoguessme.com/signup",
    goto: async (url) => state.urls.push(url),
    getByLabel: (label) => ({ fill: async (value) => state.fields.set(label, value) }),
    getByRole: (_role, options) => ({ click: async () => state.clicked.push(options.name) }),
    waitForURL: async (predicate) => {
      const url = new URL("https://dev.geoguessme.com/groups");
      assert.equal(predicate(url), true);
      state.urls.push(url.toString());
    },
  };
  return page;
}

delete process.env.QA_ACCOUNT_PASSWORD;
const first = fakePage();
const owner = await loginAccount(first, new URL("https://dev.geoguessme.com/"), "owner");
assert.deepEqual(owner, { account_role: "owner", authenticated: true });
assert.match(first.state.urls[0], /\/signup$/);
assert.match(first.state.fields.get("Username"), /^qa_owner_[a-z0-9]+$/);
assert.match(first.state.fields.get("Password"), /^Qa[a-f0-9]{20}1$/);
assert.deepEqual(first.state.clicked, ["Sign Up"]);

process.env.QA_ACCOUNT_PASSWORD = "ConfiguredPassword1";
const second = fakePage();
const member = await loginAccount(second, new URL("https://dev.geoguessme.com/"), "member");
assert.deepEqual(member, { account_role: "member", authenticated: true });
assert.match(second.state.urls[0], /\/login$/);
assert.equal(second.state.fields.get("Username"), "qa_release_member");
assert.equal(second.state.fields.get("Password"), "ConfiguredPassword1");
assert.deepEqual(second.state.clicked, ["Login"]);

console.log("QA account-pool bootstrap contract PASSED");
