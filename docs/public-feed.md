# Public challenge feed

Signed-in players can open **Explore feed** from My Groups, or visit `/feed`, to
discover photo challenges from the whole community. Posts appear newest first,
with a stable cursor for loading older posts. Each post has a shareable
`/feed/{id}` link. Signing in is required to follow that link.

The profile page also shows a bounded feed leaderboard for the profile owner. It
ranks other players by the sum of their immutable guesses on that owner's public
and viewer-visible friends challenges, never including the owner, and includes
the same avatar marker used by other player lists. The legacy community
leaderboard remains available for compatibility. Opening a username still uses
the existing profile visibility rule: full profile details require a shared
group unless the viewer is looking at their own profile.

## Publish and play

Choose **Post a challenge**, take a photo with the camera, add an optional
description of up to 280 characters, and choose **Everyone** or **Friends in my
groups**. The browser attaches the device's current location when the photo is
sent. Feed capture does not accept an old photo, a manually pinned map point, or
hand-entered coordinates. Friends posts are visible to users sharing a group
with the author. The author can optionally narrow a friends post to selected
groups; the author must belong to every selected group. Public posts remain
visible to every signed-in user. Publication is explicit: private group
challenges never appear in the feed automatically. Public uploads use the
configured image byte and pixel limits and strip metadata during normalization.
Videos continue to use the private group challenge flow.

The feed camera reuses the private group capture workflow for its live preview,
shutter, processing, and resource cleanup. Camera and location permission errors
remain recoverable, while the feed composer intentionally has no file picker or
manual location controls. Server-side validation and normalization remain
authoritative.

An unresolved viewer receives a small, reduced-detail preview from the server.
Removing the visual blur cannot recover the original pixels. The timed feed
challenge uses the same server-authoritative lifecycle as a group challenge:
**Play challenge** accepts a session, streams the original, and confirms
complete media delivery before the view and guess windows start. Late guesses
persist a zero-point timeout, and the author is never allowed to guess their own
post. Legacy untimed `/play` and `/guess` endpoints remain available while
clients migrate to the timed routes. The author always sees their own original.
Other viewers' resolution state is independent.

Each player gets one immutable guess per public post. Repeated submissions
return the first result, including concurrent submissions. Timed results show
the distance, the actual point, and the time-adjusted score from 0 to 5000 in a
map-ready response. Exact challenge and guess coordinates are returned only
after the owner, a resolved viewer, or an expired timed session is authorized to
see results. Once a challenge has multiple guesses, every guess is available in
score order with a stable rank and a signed all-time Elo delta. The delta is
replayed from the same combined private/public history as the global ladder; a
single guess still has no Elo comparison. Public score totals remain separate
from private group challenge leaderboards and profile progression. Authors
cannot guess their own posts. Feed and comment payloads never contain answer
coordinates.

## Reactions and comments

Players can add or remove one heart reaction per post. The feed returns the
aggregate count and whether the viewer reacted. Repeated like requests are
idempotent.

Comments accept 1–1000 characters after trimming whitespace. Threads are
collapsed by default and carry a spoiler notice for unresolved players. Opening
a thread shows the newest comments first, with pagination for older comments.
Comment authors can delete their own comments; a post author can remove any
comment on their post. Only the author can delete a post.

**Share challenge** opens the device's share sheet when available, otherwise
copies the post link. If clipboard access is unavailable, a selectable link
remains available. Recipients sign in before viewing the challenge. Author names
remain plain text because existing player profiles are restricted to shared
group members; public posting does not widen those profile permissions.

## Interaction and accessibility

The feed uses the app's existing colors, buttons, artwork, and camera. A compact
header brings the first photo into view sooner, and the camera action at the
bottom of the feed keeps posting discoverable on mobile. **Play public
challenges** gives new players a route from an empty group list. **Play
challenge** opens the photo; **Guess & reveal** commits the one attempt. The
score and distance lead back to the same post for reactions and conversation.

Dialogs retain keyboard focus and restore it on close. Publication and guesses
keep their dialog open during saving to prevent an accidental dismissal from
hiding the outcome. Failed saves preserve entered data. Photos load near the
viewport and release their blob URLs on departure; feed and comment pages use
bounded cursor requests with indexable seek predicates.

Feed mutations use the existing authenticated rate limit. Reading the feed,
photos, results, or comments does not consume that write allowance, so browsing
cannot prevent a subsequent guess or comment. Every read still requires an
active authenticated account and the applicable media authorization.

Publication fan-out is capped at 20 selected groups. A submission stores one
feed object and one private object per selected group; the database destination
rows commit transactionally, while object storage uses compensation because it
cannot participate in that transaction. Failed cleanup is attempted for up to 10
seconds per object and then sent to the durable deletion worker. Both
publication and cleanup serialize on the canonical storage key; the worker
rechecks live references before deleting, so a retry cannot remove a later
successful upload that reused a key.

The idempotency reservation also stores the normalized byte size, MIME type,
SHA-256 digest, location-visibility flag, and a random per-attempt publication
token. Replaying a key with different immutable metadata returns a conflict.
Rows created before migration 033 retain safe defaults (location visible, zero
size, and empty digest/token) and are not silently treated as equivalent to a
newly uploaded capture; operators should use a fresh idempotency key for those
legacy rows.

## Storage, deployment, and rollback

migrations **027_public_feed**, **028_group_inbox_reads**,
**029_feed_audiences**, **030_public_feed_results**, and
**031_public_feed_leaderboard**, and **032_public_feed_timed_games** add
independent public challenge data, durable group inbox read boundaries,
audience/selected-group records, ranked results and feed-score indexes, and a
separate timed-session/timeout lifecycle for public feed games. Migration
**033_feed_publication_metadata** is applied after the timed-game migration; its
`IF NOT EXISTS` clauses also allow a clean deployment of this branch when 032 is
absent. Apply migrations with the existing deployment migration job before
starting the new application revision; local operators use `make migrate-up`. No
environment variables or services are added.

Public posts remain available until the author deletes the post or account; the
private challenge TTL and retention settings do not apply. Preview bytes live
with the database row. Original photos remain in private object storage and are
streamed through authenticated endpoints with `private, no-store`.

A database delete trigger atomically enqueues the original photo for the
existing durable deletion worker, including account-deletion cascades. The post
becomes inaccessible immediately while physical deletion is retried by that
worker. Monitor the existing storage cleanup backlog and request-error metrics.
Include the public tables and objects in the existing database/storage backups.

The migration is additive and forward-only. Rolling back the application leaves
the public tables intact; the previous application ignores them. The deletion
trigger uses the existing queue and its supported `manual` source, so older
cleanup workers can process public deletion jobs. Do not remove the tables or
trigger during an application rollback.

## Validation

Handler, composition, and repository tests cover authentication on every feed
route, validation, visibility, immutable guesses, storage cleanup timeouts,
errors, and pagination. The integration fixture in
`backend/integration_test/flow_test.go` exercises real publication, viewer
isolation, independent concurrent guesses, duplicate attempts, reactions,
comment authorization, and migration cascade cleanup. The deterministic browser
journey is in `frontend/e2e/feed/public-feed.spec.ts`. Run it in both the
Chromium desktop and Pixel 5 mobile projects; it asserts that no file picker or
manual coordinate controls are present and that the multipart publication
carries the deterministic device coordinates.

The Playwright journey captures the empty feed, composer, audience controls,
authored post, blurred challenge, guess dialog, result, revealed challenge, and
reaction/comment thread. A generated landscape exercises real image decoding,
preview reduction, and blur. Assertions check decoded images, blur removal,
horizontal overflow, and dialog bounds before screenshots are attached to the
HTML report. These are visual-review artifacts, not pixel-baseline comparisons.
The deliberately inaccurate browser guess pins reveal-on-completion behavior.
The journey also requires a loaded group inbox without alerts and checks compact
audience controls inside touch targets of at least 44 pixels. The composition
regression verifies that the group API receives its inbox-capable repository.
Scrollable dialogs also capture their lower controls. The existing desktop and
mobile projects both exercise this journey.

The shared map observes container size changes so opening a previously hidden
dialog fills the entire map with tiles. The screenshot journey verifies tile
coverage before capture; unit coverage checks resize handling and observer
cleanup. Deterministic test tiles remove dependency on an external map server;
Leaflet and all application API, database, and media interactions remain real.

Run the focused screenshot journey through the Dockerized interface:

```sh
GEOGUESSME_E2E_PROJECTS=desktop,mobile GEOGUESSME_E2E_SPEC=feed/public-feed.spec.ts make test-e2e
```

Images stay in ignored `frontend/test-results/` and the Playwright HTML report.
PR CI retains its desktop report and screenshots for seven days, including
successful runs; download the `geoguessme-e2e-<run>-<shard>` artifact in
Actions.

## Roadmap and adjacent maintenance

- **Frontend ownership — keyboard location selection in private games.** The
  bounded scan found that the existing group map selects locations by click. Add
  labeled coordinate inputs to private gameplay, matching the keyboard access
  provided by the public feed. Verify that keyboard players can select and
  submit a location without a pointer.
- **Tooling ownership — Dockerize the maintenance report.** The existing
  `make maintenance-report` target invokes host Bash and fails on macOS Bash 3
  with `declare: -A: invalid option`. Move its reporting script into the
  Dockerized toolchain and verify the target with the supported host
  prerequisites. The feed change uses direct module inspection and the structure
  gate for its bounded scan.
- **Tooling ownership — isolate installed dependencies per worktree.** Tool
  containers share the `frontend-node-modules` volume across checkouts. A
  concurrent installation removed a Vitest preload module during verification.
  Scope the installed dependency volume to the worktree or lockfile so one
  task's install cannot invalidate another's tests. The feed verification uses
  the existing `COMPOSE_TOOLS` Make override with a separate named dependency
  volume; its commands are unchanged. Align cache ownership between the root
  test runner and the user-owned build runner too: a root-owned `.vite-temp`
  directory blocked the local build until the standard bootstrap reset it.
- **Tooling ownership — check TypeScript project references in preflight.** The
  existing `make type-check` runs `tsc --noEmit` on a root configuration with
  `files: []`, so it does not check the referenced frontend projects. Switch
  that gate to check both projects and add a failing-type fixture in a separate
  tooling change. The public-feed review exposed this gap through two untyped
  dialog mocks; `make build-frontend` checks the actual projects and validates
  their fix.
