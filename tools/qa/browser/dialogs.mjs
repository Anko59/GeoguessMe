// Native confirmation is a user choice, never an implicit approval or a way to
// enter credentials. Scope the listener to one action and one dialog only.
export async function withNextDialog(page, choice, action, safeText) {
  if (choice !== undefined && !["accept", "dismiss"].includes(choice)) throw new Error("dialog_action must be accept or dismiss");
  let result;
  let handling;
  let failure;
  const listener = (dialog) => {
    const accept = choice === "accept" && dialog.type() === "confirm";
    result = { type: dialog.type(), message: safeText(dialog.message()), action: accept ? "accepted" : "dismissed" };
    handling = (accept ? dialog.accept() : dialog.dismiss()).catch((error) => { failure = error; });
  };
  page.once("dialog", listener);
  try {
    await action();
    if (handling) await handling;
    if (failure) throw failure;
    return result;
  } finally {
    page.off("dialog", listener);
    if (handling) await handling;
  }
}
