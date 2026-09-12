# Public challenge feed

Signed-in players can open **Explore feed** from My Groups, or visit `/feed`, to
discover photo challenges from the whole community. Posts appear newest first,
with a stable cursor for loading older posts. Each post has a shareable
`/feed/{id}` link. Signing in is required to follow that link.

## Publish and play

Choose **Post a challenge**, upload a JPG, PNG, or WebP photo, add an optional
caption of up to 500 characters, and select the photo's location on the map or
enter its coordinates. Publication is explicit: private group challenges never
appear in the public feed automatically. Public uploads use the configured image
byte and pixel limits and strip metadata during normalization. Videos continue
to use the private group challenge flow.

The composer accepts only JPG, PNG, and WebP files and renders decoded pixels on
a bounded canvas, without exposing a URL for the raw upload. Publishing stays
disabled until the preview succeeds; unsupported or unreadable files show an
accessible error. Server-side validation and normalization remain authoritative.

An unresolved viewer receives a small, reduced-detail preview from the server.
Removing the visual blur cannot recover the original pixels. Choosing **Play
challenge** opens the original photo for an untimed attempt; submitting **Guess
& reveal** with one valid guess reveals the photo in that player's feed
permanently. The author always sees their own original. Other viewers'
resolution state is independent.

Each player gets one immutable guess per public post. Repeated submissions
return the first result, including concurrent submissions. Results show the
distance, the actual point, and the existing distance-based score from 0
to 5000. **View your result** reopens the stored result. Public scores are
separate from private group leaderboards and profile progression. Authors cannot
guess their own posts. Feed and comment payloads never contain answer
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

The feed uses the app's existing colors, buttons, artwork, and map. A compact
header brings the first photo into view sooner, and **Play public challenges**
gives new players a route from an empty group list. **Play challenge** opens the
photo; **Guess & reveal** commits the one attempt. The score and distance lead
back to the same post for reactions and conversation.

Dialogs retain keyboard focus and restore it on close. Publication and guesses
keep their dialog open during saving to prevent an accidental dismissal from
hiding the outcome. Failed saves preserve entered data. Both location pickers
have labeled coordinate fields for keyboard use. Photos load near the viewport
and release their blob URLs on departure; feed and comment pages use bounded
cursor requests with indexable seek predicates.

## Storage, deployment, and rollback

Migration **026_public_feed** adds independent public challenge, guess,
reaction, and comment tables. Apply migrations with the existing deployment
migration job before starting the new application revision; local operators use
`make migrate-up`. No environment variables or services are added.

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
comment authorization, and migration cascade cleanup. The browser journey is in
`frontend/e2e/feed/public-feed.spec.ts`.

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
