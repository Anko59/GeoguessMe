# Rank badges

This page documents the visual identity of the lifetime progression ranks and
how badge artwork is produced and displayed. It accompanies the profile page and
leaderboard changes that surface rank everywhere a rank name is shown.

## Badge tiers

The 20 progression ranks are grouped into 5 visual tiers. Every rank in a tier
shares the same badge artwork, so the ordering is readable at a glance: the
badge material and form step up clearly from one tier to the next, and the rank
name next to the badge disambiguates within a tier.

| Tier | Levels | Ranks                                      | Badge                    |
| ---- | ------ | ------------------------------------------ | ------------------------ |
| 1    | 1–4    | Page, Squire, Yeoman, Herald               | Bronze shield            |
| 2    | 5–8    | Knight Errant, Knight, Banneret, Castellan | Silver shield            |
| 3    | 9–12   | Baron, Viscount, Count, Earl               | Gold shield              |
| 4    | 13–16  | Marquess, Duke, Grand Duke, Prince         | Royal crown              |
| 5    | 17–20  | Regent, Sovereign, High King, Emperor      | Imperial crown with gems |

Badges intentionally carry no number or text: AI-generated artwork does not
render legible glyphs at small sizes, and the rank name is always displayed next
to the badge.

## Artwork generation

Badge artwork is generated with the `minimax/image-01` model on Replicate using
a single style recipe so the five badges look like one consistent set:

- flat vector badge emblem, centered, symmetric, clean edges
- one specific heraldic form and material per tier (see the table above)
- isolated on a pure white background
- no text, no letters, no numbers, no watermark
- no border, no frame, no drop shadow

The Replicate token is read from `REPLICATE_API_TOKEN` in the developer's shell
environment. Generation is a one-time manual step, not part of the build: the
finished transparent PNGs are committed under `frontend/public/rank-badges/` as
`tier-1.png` through `tier-5.png`.

Because `image-01` outputs JPEG (no alpha channel), generated images are
post-processed once with a white-to-transparent flood fill before being
committed. The flood fill removes background connected to the image border
(threshold 242) and fades near-white edge pixels to remove JPEG halos; the
results are trimmed, padded, and resized to 256×256. The committed PNGs are
final assets: regeneration is a one-off manual step, never part of the build.

To regenerate a tier, run the same prompt recipe for that tier through the
model, post-process with the flood-fill script, verify transparency and margins
programmatically, and review the result before committing.

## Display rules

- A small tier badge (16–20 px) is shown next to the rank name everywhere a rank
  name is displayed: the profile hero, the profile stat card, and every
  leaderboard row.
- The profile hero shows a large version (96–144 px) of the player's tier badge
  as the progression artwork.
- The tier badge is chosen from the rank's `level` via a small lookup table in
  the frontend; the server keeps sending the existing `trophy_key` for backward
  compatibility.

## Global rank

The profile response gains a `global_rank` object with two fields:

- `rank`: the player's position among all players who have guessed at least
  once, ordered by lifetime points using standard competition ranking (equal
  totals share a rank, the next rank is skipped accordingly). Zero while the
  player has no guesses of their own.
- `total_players`: the number of players who have guessed at least once.

The profile page renders a ranked player as a "Global rank" card showing `#3`
over "of 1,943 players"; a player who never guessed sees "Unranked" with a hint
to guess a location to enter the ranking.
