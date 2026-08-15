import assert from "node:assert/strict";
import { CoverageTracker } from "./coverage.mjs";

const incomplete = new CoverageTracker();
const blocked = incomplete.snapshot("full");
assert.equal(blocked.release_ready, false);
assert.equal(blocked.missing.length, 10);

const complete = new CoverageTracker();
complete.role("owner-session", "owner");
complete.role("member-session", "member");
complete.role("outsider-session", "outsider");
complete.emailAccount();
complete.mailboxCreated("mailbox-1");
complete.mailboxSearched("mailbox-1");
complete.linkOpened("verification");
complete.linkOpened("password-reset");
complete.transferCaptured();
complete.transferOpened();
complete.capabilitiesObserved({ camera: { usable: true }, geolocation: { usable: true } });
complete.challengeUploaded = true;
complete.challengeObserved = true;
complete.challengeAccepted = true;
complete.guessPlaced = true;
complete.guessSubmitted = true;
complete.chatAction = true;
complete.chatObservationSessions.add("owner-session");
complete.chatObservationSessions.add("member-session");
complete.leaderboardObserved = true;
complete.profileVisited = true;
complete.settingsVisited = true;
complete.refreshed = true;
complete.mobile = true;
complete.outsiderGroupAttempt = true;
const ready = complete.snapshot("full");
assert.equal(ready.release_ready, true);
assert.deepEqual(ready.missing, []);

console.log("QA coverage gate contract PASSED");
