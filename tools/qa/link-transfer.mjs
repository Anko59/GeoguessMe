export class LinkTransferStore {
  constructor({ baseUrl }) {
    this.origin = new URL(baseUrl).origin;
    this.links = new Map();
    this.sequence = 0;
  }

  capture(value) {
    if (typeof value !== "string" || !value) throw new Error("Visible target does not contain a transferable invite link");
    let url;
    try {
      url = new URL(value, `${this.origin}/`);
    } catch {
      throw new Error("Visible target does not contain a valid invite link");
    }
    if (url.origin !== this.origin || !/(?:[?#&])invite(?:=|$)/i.test(`${url.search}${url.hash}`)) {
      throw new Error("Visible target is not a same-origin group invite link");
    }
    const transferId = `transfer-${++this.sequence}`;
    this.links.set(transferId, url.toString());
    return { transfer_id: transferId, link_kind: "group-invite" };
  }

  consume(transferId) {
    const value = this.links.get(transferId);
    if (!value) throw new Error("Unknown or already consumed invite transfer");
    this.links.delete(transferId);
    return value;
  }

  clear() {
    this.links.clear();
  }
}
