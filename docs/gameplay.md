# Gameplay

## Challenge lifecycle

A photo or video challenge moves through these states:

```text
ready → accepted → viewing window → guessable → expired → removed
```

1. **ready** — The media is uploaded. No group member can see it yet.
2. **accepted** — A member calls `POST /api/v1/challenges/{photoID}/accept`.
   This opens a per-member viewing window (default 10 seconds).
3. **viewing window** — The media is available at
   `GET /api/v1/challenges/{photoID}/media` only during this window. The window
   is bounded by `PHOTO_VIEW_WINDOW` and `CHALLENGE_TTL` (whichever is shorter).
4. **guessable** — After the viewing window expires, the member may submit one
   guess via `POST /api/v1/challenges/{photoID}/guess`. Guesses are idempotent:
   a duplicate returns the original result.
5. **expired** — The challenge reaches `expires_at` (created_at +
   CHALLENGE_TTL). No more guesses can be submitted.
6. **removed** — After `retention_at` (created_at + PHOTO_RETENTION), the
   cleanup worker marks the challenge `removed`, nulls the storage key, and
   enqueues a durable object-deletion job. The media bytes are no longer
   available.

## Timing

| Parameter           | Default    | Description                           |
| ------------------- | ---------- | ------------------------------------- |
| `CHALLENGE_TTL`     | 24h        | Lifetime of the challenge from upload |
| `PHOTO_VIEW_WINDOW` | 10s        | Per-member window to view the photo   |
| `PHOTO_RETENTION`   | 720h (30d) | Media retention period from upload    |

The view window is capped at `CHALLENGE_TTL` even if `PHOTO_VIEW_WINDOW` is
longer. A re-acceptance does not extend access beyond the original window.

## Capture media

The camera shutter captures a photo when tapped. Holding the same shutter for
300 ms starts a video recording; releasing it stops the clip and opens the
normal preview before sending. The application requests microphone access only
for a held video capture. Recorded MP4 and WebM clips share the configured
`UPLOAD_MAX_BYTES` limit (10 MiB by default); selected files remain photo-only
so that video uploads originate from the privacy-preserving camera flow.

Profile photos use the separate `AVATAR_MAX_BYTES` limit (25 MiB by default)
because they are resized to a small thumbnail before storage. The shared pixel
limit still protects image decoding from oversized inputs.

## Scoring

**Formula**: Scoring uses exponential decay based on distance:

- Distance < 50 m → **5000 points** (perfect)
- Distance ≥ 50 m → `5000 × e^(-distance / 20000)` (rounded to integer)

**Scale**: 0–5000. Points decrease rapidly for the first kilometres, then taper
off.

After a guess is recorded, the frontend presents a short, animated tier label
from **Cartographic Catastrophe** through **Masterstroke**. These labels are
presentation-only; the score returned by the server remains authoritative.

The group leaderboard ranks each member by one of three metrics, each scoped to
the selected calendar week, month, or all-time period:

- **Total** — the sum of all of the member's guess scores in the period.
- **Average** — the member's average guess score in the period.
- **Elo** — a rating computed from the period's challenges. Every photo that at
  least two group members guessed is a challenge; each pair of guessers is
  compared by raw score (win/draw/loss) and both ratings move by the standard
  `K = 32` Elo update. Ratings are recomputed from guess history on every read,
  so a late guess on an old challenge retroactively moves everyone who played it
  — this is what makes Elo measure guessing skill independently of who posts the
  harder challenges.

A player who never compared against another guesser on a shared challenge has
Elo 0, sorts below rated players, and has no numbered place. Profiles use the
same Elo model across all groups and all-time challenges for the global rating
and rank.

## Result visibility

| Who                      | When                                     |
| ------------------------ | ---------------------------------------- |
| Uploader                 | Immediately after upload                 |
| Member who guessed       | Immediately after submitting their guess |
| Any current group member | After `CHALLENGE_TTL` expires            |

The result endpoint (`GET /api/v1/challenges/{photoID}/results`) returns
`actual_lat`, `actual_long`, all guesses with `username`, `score`, `distance`,
and `media_url` plus `media_type` (with `?result=1`) if the media is still
available. Result photos can be opened full screen; videos retain playback
controls in the result panel.

## Group join codes

Groups are joined via a 6-character alphanumeric code (uppercase A–Z, 0–9).
Codes are generated randomly at creation. An unauthenticated invite preserves
the code through login or signup, automatically joins the account, and opens the
group. Replaying an invite for an existing member is safe and simply opens the
group again.

The chat message history includes a viewer-specific `challenge_status`:
`available`, `accepted`, `guessed`, `results`, or `expired`. The frontend uses
this to show Accept, Continue, or View Results without exposing result data to
unauthorized members. Results show every submitted score visible to the current
authorized viewer, with the current player highlighted.
