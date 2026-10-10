import assert from "node:assert/strict";
import { createServer } from "node:http";
import { chromium } from "/workspace/frontend/node_modules/playwright/index.mjs";
import { EmailCredentials } from "./email-credentials.mjs";

let mode;
let heldPassword;
let loginSubmissions;
let appUrl;
let identityUrl;
let fixtureError;
const mailbox = { create: async () => ({ mailbox_id: "owned-mailbox", address: "qa@example.test" }) };
const age = '<label><input type="checkbox">I confirm I am at least 15 years old</label>';
const passwordForm = (action, button, confirm = false) => `<form method="post" action="${action}"><label>* New Password * <input name="password" type="password" required></label>${confirm ? '<label>* Confirm password * <input name="confirmation" type="password" required></label>' : ""}<button>${button}</button></form>`;

async function requestBody(request) {
  let body = "";
  for await (const chunk of request) body += chunk;
  return new URLSearchParams(body);
}

const server = (identity) => createServer((request, response) => {
  handle(request, response, identity).catch((error) => { fixtureError = error; response.statusCode = 500; response.end("fixture failed"); });
});

async function handle(request, response, identity) {
  const path = new URL(request.url, appUrl || "http://localhost").pathname;
  response.setHeader("Content-Type", "text/html; charset=utf-8");
  const redirect = (url) => { response.writeHead(302, { Location: url }); response.end(); };
  if (request.method === "POST") {
    const body = await requestBody(request);
    if (["/register", "/reset"].includes(path)) {
      if (identity) assert.equal(body.get("confirmation"), body.get("password"));
      heldPassword = body.get("password");
      if (identity) response.end("<h1>Your password has been updated</h1>");
      else redirect(`${appUrl}/${path === "/register" ? "groups" : "login"}`);
      return;
    }
    if (path === "/login") {
      loginSubmissions++;
      assert.equal(body.get("password"), heldPassword);
      redirect(`${appUrl}/groups`);
      return;
    }
  }
  if (identity) {
    if (path === "/registration") {
      response.end('<form action="/realms/geoguessme/login-actions/required-action"><button>Create account</button></form>');
    } else if (path === "/realms/geoguessme/login-actions/required-action") {
      response.end(passwordForm(request.url.includes("reset") ? "/reset" : "/register", "Submit", true));
    } else if (path === "/login") {
      response.end('<form method="post" action="/login"><label>Password<input type="password" name="password"></label><button>Login</button></form>');
    } else { response.statusCode = 404; response.end("missing fixture"); }
    return;
  }
  if (path === "/signup") {
    response.end(mode === "oidc"
      ? `<form action="${identityUrl}/registration">${age}<label>Email address<input name="email"></label><button>Continue to create account</button></form>`
      : `<form method="post" action="/register">${age}<input name="username" placeholder="Username"><input name="email" placeholder="Email — verify to enable account recovery"><input name="password" placeholder="Password" type="password"><button>Sign Up</button></form>`);
  } else if (path === "/reset-password") {
    response.end(passwordForm("/reset", "Reset password"));
  } else if (path === "/login") {
    if (request.headers.cookie?.includes("auto-sso=1")) return redirect(`${appUrl}/groups`);
    response.end(mode === "oidc"
      ? `<form action="${identityUrl}/login"><label>Email address<input name="email"></label><button>Continue to password</button></form>`
      : '<form method="post" action="/login"><label>Username or email<input name="email"></label><label>Password<input name="password" type="password"></label><button>Login</button></form>');
  } else if (path === "/groups") response.end("<h1>Groups</h1>");
  else { response.statusCode = 404; response.end("missing fixture"); }
}

const app = server(false);
const identity = server(true);
await Promise.all([app, identity].map((service) => new Promise((resolve) => service.listen(0, "127.0.0.1", resolve))));
appUrl = `http://127.0.0.1:${app.address().port}`;
identityUrl = `http://127.0.0.1:${identity.address().port}`;
let browser;
try {
  browser = await chromium.launch({ headless: true });
  for (mode of ["direct", "oidc"]) {
    loginSubmissions = 0;
    const credentials = new EmailCredentials({ baseUrl: appUrl, identityOrigins: [identityUrl] });
    const context = await browser.newContext();
    const page = await context.newPage();
    const signup = await credentials.signup({ page, mailbox, accountRole: "owner" });
    const original = heldPassword;
    assert.match(original, /^Qa[a-f0-9]{20}1$/);
    assert.equal(JSON.stringify(signup).includes(original), false);
    await page.goto(mode === "oidc"
      ? `${identityUrl}/realms/geoguessme/login-actions/required-action?reset=private-token`
      : `${appUrl}/reset-password?token=private-token`);
    credentials.authorizeReset(signup.mailbox_id, page);
    const reset = await credentials.reset({ page, mailboxId: signup.mailbox_id });
    assert.equal(reset.changed_password_verified, false);
    assert.notEqual(heldPassword, original);
    await context.addCookies([{ name: "auto-sso", value: "1", url: appUrl }]);
    const verified = await credentials.login({ page, mailboxId: signup.mailbox_id });
    assert.equal(verified.changed_password_verified, true);
    assert.equal(loginSubmissions, 1, "SSO must not replace a real password submission");
    assert.equal(JSON.stringify(verified).includes(heldPassword), false);
    credentials.clear();
    await context.close();
  }
  console.log("QA real-browser direct and identity password recovery contracts PASSED");
} catch (error) {
  throw fixtureError || error;
} finally {
  await browser?.close();
  await Promise.all([app, identity].map((service) => new Promise((resolve) => service.close(resolve))));
}
