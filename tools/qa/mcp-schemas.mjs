export function tool(name, description, inputSchema) {
  return { name, description, inputSchema: inputSchema || { type: "object", properties: {} } };
}

export function pageSchema() {
  return {
    type: "object",
    required: ["session_id"],
    properties: { session_id: { type: "string" }, tab_id: { type: "string" } },
  };
}

export function actionSchema() {
  return {
    type: "object",
    required: ["session_id", "target"],
    properties: {
      session_id: { type: "string" }, tab_id: { type: "string" },
      target: { type: "object", additionalProperties: false },
    },
  };
}
