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
      target: {
        type: "object", additionalProperties: false,
        properties: {
          role: { type: "string" }, name: { type: "string" }, label: { type: "string" },
          placeholder: { type: "string" }, text: { type: "string" }, test_id: { type: "string" },
          exact: { type: "boolean", default: true },
        },
        anyOf: ["role", "label", "placeholder", "text", "test_id"].map((key) => ({ required: [key] })),
      },
      dialog_action: dialogActionSchema(),
    },
  };
}

export function dialogActionSchema() {
  return { type: "string", enum: ["accept", "dismiss"], default: "dismiss", description: "Explicitly accept the next native confirmation for this action, or dismiss it. Other dialog types, including credential prompts, are always dismissed. Returned dialog metadata never includes prompt default values." };
}
