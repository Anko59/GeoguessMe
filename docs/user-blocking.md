# Player blocking

## Player controls

Open another player's profile and choose **Block player**. Confirming hides
content and interactions between the two accounts in both directions. A failed
request leaves the profile and relationship unchanged. You cannot block
yourself.

Your **Settings → Blocked users** list contains only blocks you created. Choose
**Unblock** there, or **Unblock player** on a blocked profile, to remove your
outgoing block. Unblocking does not remove a block the other player created;
access may therefore remain unavailable. Incoming blocks are never disclosed.
The manager shows usernames without requesting blocked private avatar media.

## Visibility and retained identity

Blocks suppress chat history and live messages, feed content and interactions,
profile access, and private media between the pair. Direct blocked-resource
requests return the ordinary not-found response, including when the other player
created the block. Group membership, group member identities, scores, ranking
identities, and existing gameplay records remain intact. Blocking is not group
removal and does not delete another player's contributions.

A successful block or unblock clears session/message hints, avatar/group-photo
blob caches, and leaderboard caches. Open views remount and refetch
authoritative server visibility; their media and socket owners run normal
cleanup. A same-origin storage event invalidates other open tabs without
transmitting player IDs or block relationship details. When browser storage is
disabled, local invalidation still works but cross-tab signaling is unavailable;
refresh other tabs. Blocking does not retract media already downloaded or
content saved outside the app.

Blocking and [content reporting](api.md#content-reports) are independent: a
block is a personal visibility control, not a moderation notice. Submit a report
before blocking if the content requires operator review.

## API and rollout

The [API reference](api.md#player-blocking) and [OpenAPI contract](openapi.yaml)
define the authenticated list and idempotent mutation endpoints. The server is
authoritative; clients never receive incoming relationship metadata.

Deploy the forward-only `038_user_blocks` migration with the backend before
serving the updated frontend. The table stores blocker ID, blocked ID, and
creation time; account deletion removes relationships through foreign-key
cascades. No new environment variables or operator-managed secrets are required.
The supported Compose topology runs one backend process, which also owns the
in-memory chat hub. Every socket and queued push delivery rechecks database
visibility; a process-local read/write barrier additionally orders in-flight
writes against a successful block response. Do not scale backend replicas while
claiming that response-ordering guarantee: a distributed delivery barrier and
shared realtime fanout are required first. Already delivered bytes cannot be
recalled, and ordinary in-flight HTTP/media responses may have been authorized
before the preference changed.

For rollback, retain the migration and block data. Do not roll back to a backend
that lacks visibility enforcement while representing blocking as active to
users; restore the enforcing revision or pause affected interactions. Operators
should use the ordinary error and request monitoring to investigate failed
mutations; do not log relationship lists or their contents.
