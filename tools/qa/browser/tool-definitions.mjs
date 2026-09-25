import { actionSchema, pageSchema, tool } from "../mcp-schemas.mjs";

export const tools = [
  tool("session_create", "Create an isolated browser session.", {
    type: "object",
    properties: { width: { type: "integer" }, height: { type: "integer" } },
  }),
  tool("session_close", "Close an isolated browser session.", {
    type: "object",
    required: ["session_id"],
    properties: { session_id: { type: "string" } },
  }),
  tool("qa_account_login", "Log an isolated session into a dedicated owner, member, or outsider QA account. If no QA account password is configured, start a fresh disposable mailbox-backed signup and keep its mailbox inside the browser provider; complete any visible identity-provider registration and verification before using the role.", { type: "object", required: ["session_id", "account_role"], properties: { session_id: { type: "string" }, tab_id: { type: "string" }, account_role: { type: "string", enum: ["owner", "member", "outsider"] } } }),
  tool("qa_email_account_signup", "Create a fresh email-backed QA account through the visible signup flow; generated passwords stay inside the browser provider. The result may require completing the visible identity-provider registration and opening a mailbox verification link before the role is authenticated.", { type: "object", required: ["session_id", "account_role"], properties: { session_id: { type: "string" }, tab_id: { type: "string" }, account_role: { type: "string", enum: ["owner", "member", "outsider"] } } }),
  tool("tab_open", "Open a second tab in an existing session.", {
    type: "object",
    required: ["session_id"],
    properties: { session_id: { type: "string" } },
  }),
  tool("tab_switch", "Switch the active tab in a session.", {
    type: "object",
    required: ["session_id", "tab_id"],
    properties: { session_id: { type: "string" }, tab_id: { type: "string" } },
  }),
  tool("browser_navigate", "Navigate the active tab within the deployed application.", {
    type: "object",
    required: ["session_id", "url"],
    properties: { session_id: { type: "string" }, url: { type: "string" } },
  }),
  tool("browser_observe", "Read visible text, accessibility state, URL, and recent diagnostics.", {
    type: "object",
    required: ["session_id"],
    properties: { session_id: { type: "string" }, tab_id: { type: "string" } },
  }),
  tool("browser_transfer_link", "Capture a visible same-origin group invite into an opaque single-use transfer; never return the link.", { ...actionSchema(), required: ["session_id", "target", "kind"], properties: { ...actionSchema().properties, kind: { type: "string", enum: ["group-invite"] } } }),
  tool("browser_open_transferred_link", "Open an opaque group invite transfer in this isolated browser session; never return the link.", {
    type: "object", required: ["session_id", "transfer_id"], properties: { session_id: { type: "string" }, tab_id: { type: "string" }, transfer_id: { type: "string" } },
  }),
  tool("browser_capabilities", "Probe the granted synthetic camera and location services with a fixed safe check.", pageSchema()),
  tool("browser_click", "Click one visible control selected by role, label, text, or placeholder.", actionSchema()),
  tool("browser_type", "Fill one visible text control selected by role, label, text, or placeholder.", {
    ...actionSchema(),
    required: ["session_id", "target", "text"],
    properties: { ...actionSchema().properties, text: { type: "string" } },
  }),
  tool("browser_select", "Select an option in one visible select control.", {
    ...actionSchema(),
    required: ["session_id", "target", "value"],
    properties: { ...actionSchema().properties, value: { type: "string" } },
  }),
  tool("browser_upload", "Upload a generated small image fixture through a visible file control.", {
    ...actionSchema(),
    required: ["session_id", "target"],
  }),
  tool("browser_key", "Send a keyboard key to the active tab.", {
    type: "object",
    required: ["session_id", "key"],
    properties: { session_id: { type: "string" }, tab_id: { type: "string" }, key: { type: "string" } },
  }),
  tool("browser_reload", "Reload the active tab.", pageSchema()),
  tool("browser_back", "Go back in the active tab history.", pageSchema()),
  tool("browser_forward", "Go forward in the active tab history.", pageSchema()),
  tool("browser_resize", "Change the active tab viewport.", {
    type: "object",
    required: ["session_id", "width", "height"],
    properties: {
      session_id: { type: "string" }, tab_id: { type: "string" },
      width: { type: "integer", minimum: 320 }, height: { type: "integer", minimum: 240 },
    },
  }),
  tool("browser_wait_for", "Wait for a visible state, text, role, or URL condition.", {
    type: "object",
    required: ["session_id"],
    properties: {
      session_id: { type: "string" }, tab_id: { type: "string" },
      text: { type: "string" }, role: { type: "string" }, name: { type: "string" },
      label: { type: "string" }, url: { type: "string" },
      timeout_ms: { type: "integer", minimum: 100, maximum: 30000 },
    },
  }),
  tool("browser_screenshot", "Capture a targeted screenshot as evidence; do not use by default.", {
    ...pageSchema(),
    properties: { ...pageSchema().properties, purpose: { type: "string" } },
  }),
  tool("browser_diagnostics", "Read recent console and network summaries without sensitive payloads.", pageSchema()),
  tool("mailbox_create", "Create a disposable QA mailbox for a fresh test account; its password stays inside the provider gateway.", {
    type: "object", properties: {},
  }),
  tool("mailbox_search", "Search a QA mailbox for product email and optionally wait for delivery.", {
    type: "object",
    required: ["mailbox_id"],
    properties: {
      mailbox_id: { type: "string" }, subject_contains: { type: "string" }, from_contains: { type: "string" },
      since: { type: "string" }, wait_ms: { type: "integer", minimum: 0, maximum: 60000 },
    },
  }),
  tool("mailbox_read", "Read safe mailbox metadata and body text with links and email secrets removed.", {
    type: "object", required: ["mailbox_id", "message_id"],
    properties: { mailbox_id: { type: "string" }, message_id: { type: "string" } },
  }),
  tool("mailbox_open_link", "Open one matching product link from a mailbox message without returning its tokenized URL.", {
    type: "object", required: ["session_id", "mailbox_id", "message_id", "kind"],
    properties: {
      session_id: { type: "string" }, tab_id: { type: "string" }, mailbox_id: { type: "string" }, message_id: { type: "string" },
      kind: { type: "string", enum: ["verification", "group-invite", "password-reset", "any"] },
    },
  }),
  tool("qa_record_finding", "Record a reproducible black-box QA finding.", {
    type: "object",
    required: ["category", "severity", "title", "steps", "expected", "actual", "impact"],
    properties: {
      category: { type: "string", enum: ["BUG", "UX_DEBT", "VISUAL", "PERFORMANCE"] },
      severity: { type: "string", enum: ["low", "medium", "high", "critical"] },
      title: { type: "string" }, steps: { type: "array", items: { type: "string" } },
      expected: { type: "string" }, actual: { type: "string" }, impact: { type: "string" },
      artifacts: { type: "array", items: { type: "string" } }, notes: { type: "string" },
    },
  }),
  tool("qa_finish", "Finish the run and write the revision-bound evidence report.", {
    type: "object",
    required: ["status", "summary", "journeys_exercised"],
    properties: {
      status: { type: "string", enum: ["PASS", "FINDINGS", "BLOCKED"] },
      summary: { type: "string" },
      journeys_exercised: { type: "array", items: { type: "string" } },
      journeys_not_exercised: { type: "array", items: { type: "string" } },
      limitations: { type: "array", items: { type: "string" } },
    },
  }),
];
