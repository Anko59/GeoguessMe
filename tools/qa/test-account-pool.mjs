import assert from "node:assert/strict";
import { loginAccount } from "./account-pool.mjs";

function fakePage() {
  const state = { urls: [], fields: new Map(), clicked: [] };
  const page = {
    state,
    url: () => state.urls.at(-1) || "https://dev.geoguessme.com/signup",
    goto: async (url) => state.urls.push(url),
    getByLabel: (label, options = {}) => ({ fill: async (value) => state.fields.set(`${label}|${options.exact === true}`, value) }),
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
await assert.rejects(
  () => loginAccount(first, new URL("https://dev.geoguessme.com/"), "owner"),
  /QA_ACCOUNT_PASSWORD is missing/,
);
assert.deepEqual(first.state.urls, []);

process.env.QA_ACCOUNT_PASSWORD = "ConfiguredPassword1";
const second = fakePage();
const member = await loginAccount(second, new URL("https://dev.geoguessme.com/"), "member");
assert.deepEqual(member, { account_role: "member", authenticated: true });
assert.match(second.state.urls[0], /\/login$/);
assert.equal(second.state.fields.get("Username or email|true"), "qa_release_member");
assert.equal(second.state.fields.get("Password|false"), "ConfiguredPassword1");
assert.deepEqual(second.state.clicked, ["Login"]);

console.log("QA account-pool bootstrap contract PASSED");
