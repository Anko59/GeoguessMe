const releaseJourneys = [
  { id: "authentication-and-session", label: "authentication and session" },
  { id: "email-verification-and-recovery", label: "email verification and recovery" },
  { id: "groups-and-authorization", label: "groups and authorization" },
  { id: "multi-user-group-chat", label: "multi-user group chat" },
  { id: "photo-challenge-game", label: "photo challenge game" },
  { id: "camera-location-media", label: "camera, location, and media" },
  { id: "refresh-and-reconnect", label: "refresh and reconnect" },
  { id: "leaderboard-and-progression", label: "leaderboard and progression" },
  { id: "profile-and-settings", label: "profile and settings" },
  { id: "responsive-mobile", label: "responsive mobile layout" },
];

export class CoverageTracker {
  constructor() {
    this.roles = new Set();
    this.roleSessions = new Map();
    this.chatObservationSessions = new Set();
    this.emailMailboxIds = new Set();
    this.searchedMailboxIds = new Set();
    this.emailSignup = false;
    this.verificationLinkOpened = false;
    this.resetLinkOpened = false;
    this.inviteCaptured = false;
    this.inviteOpened = false;
    this.outsiderGroupAttempt = false;
    this.capabilities = { camera: false, geolocation: false };
    this.challengeUploaded = false;
    this.challengeAccepted = false;
    this.guessPlaced = false;
    this.guessSubmitted = false;
    this.chatAction = false;
    this.challengeObserved = false;
    this.leaderboardObserved = false;
    this.profileVisited = false;
    this.settingsVisited = false;
    this.refreshed = false;
    this.mobile = false;
  }

  role(sessionId, role) {
    this.roles.add(role);
    this.roleSessions.set(sessionId, role);
  }

  emailAccount() { this.emailSignup = true; }

  mailboxCreated(mailboxId) { this.emailMailboxIds.add(mailboxId); }

  mailboxSearched(mailboxId) { this.searchedMailboxIds.add(mailboxId); }

  linkOpened(kind) {
    if (kind === "verification") this.verificationLinkOpened = true;
    if (kind === "password-reset") this.resetLinkOpened = true;
  }

  transferCaptured() { this.inviteCaptured = true; }

  transferOpened() { this.inviteOpened = true; }

  capabilitiesObserved(result) {
    this.capabilities.camera ||= result?.camera?.usable === true;
    this.capabilities.geolocation ||= result?.geolocation?.usable === true;
  }

  action(name, args) {
    const target = JSON.stringify(args.target || "").toLowerCase();
    if (name === "browser_upload" && /(challenge|camera|capture|photo)/.test(target) && !target.includes("profile")) this.challengeUploaded = true;
    if (name === "browser_reload") this.refreshed = true;
    if (name === "browser_resize" && Number(args.width) <= 480) this.mobile = true;
    if (target.includes("message") || target.includes("chat") || target.includes("send")) this.chatAction = true;
    if (target.includes("accept") && target.includes("challenge")) this.challengeAccepted = true;
    if (target.includes("guess map") || target.includes("place guess")) this.guessPlaced = true;
    if (target.includes("submit") && (target.includes("guess") || target.includes("challenge"))) this.guessSubmitted = true;
    if (target.includes("leaderboard")) this.leaderboardObserved = true;
    if (target.includes("settings")) this.settingsVisited = true;
    if (target.includes("profile")) this.profileVisited = true;
  }

  observed(sessionId, url, visibleText) {
    const text = String(visibleText || "").toLowerCase();
    const path = new URL(url).pathname;
    if (this.roleSessions.get(sessionId) === "outsider" && path.startsWith("/group/")) this.outsiderGroupAttempt = true;
    if (text.includes("chat") || text.includes("message")) this.chatObservationSessions.add(sessionId);
    if (text.includes("challenge") || text.includes("guess")) this.challengeObserved = true;
    if (text.includes("accept") && text.includes("challenge")) this.challengeAccepted = true;
    if (text.includes("leaderboard")) this.leaderboardObserved = true;
    if (path.startsWith("/profile")) this.profileVisited = true;
    if (path.startsWith("/settings") || text.includes("settings")) this.settingsVisited = true;
    if (path.startsWith("/group/")) this.outsiderGroupAttempt ||= this.roleSessions.get(sessionId) === "outsider";
  }

  snapshot(budget) {
    const full = ["full", "nightly"].includes(budget);
    const checks = {
      "authentication-and-session": this.roles.size > 0,
      "email-verification-and-recovery": this.emailSignup && [...this.emailMailboxIds].some((mailboxId) => this.searchedMailboxIds.has(mailboxId)) && this.verificationLinkOpened && this.resetLinkOpened,
      "groups-and-authorization": this.inviteCaptured && this.inviteOpened && this.outsiderGroupAttempt,
      "multi-user-group-chat": ["owner", "member", "outsider"].every((role) => this.roles.has(role)) && this.chatAction && this.chatObservationSessions.size >= 2,
      "photo-challenge-game": this.challengeUploaded && this.challengeObserved && this.challengeAccepted && this.guessPlaced && this.guessSubmitted,
      "camera-location-media": this.capabilities.camera && this.capabilities.geolocation && this.challengeUploaded,
      "refresh-and-reconnect": this.refreshed,
      "leaderboard-and-progression": this.leaderboardObserved,
      "profile-and-settings": this.profileVisited && this.settingsVisited,
      "responsive-mobile": this.mobile,
    };
    const required = full ? releaseJourneys : [];
    const evidenced = required.filter(({ id }) => checks[id]).map(({ id, label }) => ({ id, label }));
    const missing = required.filter(({ id }) => !checks[id]).map(({ id, label }) => ({ id, label }));
    return { required, evidenced, missing, release_ready: missing.length === 0 };
  }
}
