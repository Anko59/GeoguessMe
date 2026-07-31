# Rank badges

This page documents the visual identity of the lifetime progression ranks and
how badge artwork is produced and displayed. It accompanies the profile page and
leaderboard changes that surface rank everywhere a rank name is shown.

## Badge set

Every one of the 20 progression ranks has its own badge artwork. The set follows
a heraldic step-up so the ordering stays readable at a glance: the first twelve
ranks are shields in bronze, silver, then gold with a distinct emblem per rank,
and the final eight are crowns with increasing ornamentation.

| Level | Rank          | Badge                                                                           |
| ----- | ------------- | ------------------------------------------------------------------------------- |
| 1     | Page          | Bronze shield, plain                                                            |
| 2     | Squire        | Bronze shield, sword                                                            |
| 3     | Yeoman        | Bronze shield, crossed arrows                                                   |
| 4     | Herald        | Bronze shield, trumpet                                                          |
| 5     | Knight Errant | Silver shield, lance                                                            |
| 6     | Knight        | Silver shield, sword and star                                                   |
| 7     | Banneret      | Silver shield, war banner                                                       |
| 8     | Castellan     | Silver shield, castle tower                                                     |
| 9     | Baron         | Gold shield, coronet                                                            |
| 10    | Viscount      | Gold shield, coronet with pearls                                                |
| 11    | Count         | Gold shield, coronet with leaves                                                |
| 12    | Earl          | Gold shield, ermine spots                                                       |
| 13    | Marquess      | Royal crown with ermine trim and pearls                                         |
| 14    | Duke          | Royal crown with strawberry leaves                                              |
| 15    | Grand Duke    | Royal crown with arches and pearls                                              |
| 16    | Prince        | Royal crown with three arches and a jewel                                       |
| 17    | Regent        | Imperial crown with sapphires                                                   |
| 18    | Sovereign     | Imperial crown with rubies                                                      |
| 19    | High King     | Imperial crown with many gems and a laurel wreath                               |
| 20    | Emperor       | Ornate imperial crown with gold, sapphires, rubies, emeralds, and purple velvet |

Badges intentionally carry no number or text: AI-generated artwork does not
render legible glyphs at small sizes, and the rank name is always displayed next
to the badge.

## Artwork generation

Badge artwork is generated with the `minimax/image-01` model on Replicate using
a single style recipe so the twenty badges look like one consistent set:

- flat vector badge emblem, centered, symmetric, clean edges
- one heraldic form per tier (bronze, silver, gold shields, then royal and
  imperial crowns), with one explicit emblem variation per rank
- isolated on a pure white background
- no text, no letters, no numbers, no watermark
- no border, no frame, no drop shadow

The Replicate token is read from `REPLICATE_API_TOKEN` in the developer's shell
environment. Generation is a one-time manual step, not part of the build: the
finished transparent PNGs are committed under `frontend/public/rank-badges/` as
`{trophy_key}.png` (for example `knight-errant.png`), one per rank.

Because `image-01` outputs JPEG (no alpha channel), generated images are
post-processed once with a white-to-transparent flood fill before being
committed. The flood fill removes background connected to the image border
(threshold 242) and fades near-white edge pixels to remove JPEG halos; the
results are trimmed, padded, and resized to 256×256. Rarely the model draws a
gray vignette instead of a white background; such images must be regenerated and
the same post-processing applied. The committed PNGs are final assets:
regeneration is a one-off manual step, never part of the build.

To regenerate a badge, run the same prompt recipe for that rank through the
model, post-process with the flood-fill script, verify transparency and margins
programmatically, and review the result before committing.

## Display rules

- A small badge (16–20 px) is shown next to the rank name everywhere a rank name
  is displayed: the profile hero, the profile stat card, and every leaderboard
  row.
- The profile hero shows a large version (96–144 px) of the player's badge as
  the progression artwork.
- The badge is chosen from the rank's `trophy_key`, which the server already
  sends in the rank object of both the profile and the leaderboard entries.

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
