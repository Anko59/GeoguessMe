# Rank badges

This page documents the visual identity of the lifetime progression ranks and
how badge artwork is produced and displayed. It accompanies the profile page and
leaderboard changes that surface rank everywhere a rank name is shown.

## Badge set

Every one of the 30 progression ranks has its own badge artwork. The set follows
a step-up in both material and form so the ordering stays readable at a glance:
the lowest ranks are plain map pins, then pins grow a distinguishing emblem,
compasses appear, maps, globes, ornate globes, and finally the ultimate golden
instruments. Each badge matches its rank's name while the tier material (gray →
bronze → silver → gold → gold with laurels) reflects how high the rank is.

| Level | Rank                | XP required | Badge                                              |
| ----- | ------------------- | ----------: | -------------------------------------------------- |
| 1     | Completely Lost     |           0 | Plain gray map pin                                 |
| 2     | Lost Tourist        |       5,000 | Gray map pin with tiny camera                      |
| 3     | Clueless Wanderer   |      15,000 | Gray map pin with winding dashed trail             |
| 4     | Rookie Guesser      |      30,000 | Bronze map pin with arrow                          |
| 5     | Geography Beginner  |      50,000 | Bronze map pin with star                           |
| 6     | Geography Student   |      75,000 | Bronze compass with open book                      |
| 7     | Map Reader          |     105,000 | Silver compass with folded map                     |
| 8     | Landmark Spotter    |     140,000 | Silver compass with landmark tower                 |
| 9     | Local Guide         |     180,000 | Silver compass with flag                           |
| 10    | Seasoned Traveler   |     225,000 | Silver compass with airplane                       |
| 11    | Explorer            |     275,000 | Gold folded map with compass needle                |
| 12    | Scout               |     335,000 | Gold map with tent                                 |
| 13    | Wayfinder           |     405,000 | Gold map with winding trail and arrow              |
| 14    | Road Reader         |     485,000 | Gold map with highway road                         |
| 15    | Navigator           |     575,000 | Gold compass with star                             |
| 16    | Surveyor            |     675,000 | Globe with measuring calipers                      |
| 17    | Cartographer        |     800,000 | Globe with compass rose                            |
| 18    | Geo Detective       |     950,000 | Globe with magnifying glass                        |
| 19    | Geo Analyst         |   1,125,000 | Globe with bar chart                               |
| 20    | Expert Navigator    |   1,325,000 | Globe with golden compass                          |
| 21    | Master Cartographer |   1,550,000 | Ornate globe with scroll                           |
| 22    | Master Wayfinder    |   1,800,000 | Ornate globe with star                             |
| 23    | World Expert        |   2,100,000 | Globe with golden orbit rings                      |
| 24    | Geo Savant          |   2,450,000 | Globe with laurel wreath                           |
| 25    | Globe Master        |   2,850,000 | Golden globe with orbit ring                       |
| 26    | Earth Master        |   3,300,000 | Golden globe with laurel crown                     |
| 27    | Human Compass       |   3,800,000 | Golden compass with laurels and stars              |
| 28    | Human GPS           |   4,400,000 | Golden satellite with orbit ring                   |
| 29    | World Sage          |   5,100,000 | Wise owl perched on a golden globe                 |
| 30    | Living Atlas        |   6,000,000 | Grand open atlas book with a golden globe above it |

Badges intentionally carry no number or text: AI-generated artwork does not
render legible glyphs at small sizes, and the rank name is always displayed next
to the badge. The rank number in roman numerals (I to XXX) is rendered beneath
the badge by the frontend.

## Artwork generation

Badge artwork is generated with the `minimax/image-01` model on Replicate using
a single style recipe so the thirty badges look like one consistent set:

- flat vector badge emblem, centered, symmetric, clean edges
- one form per tier (gray and bronze map pins, bronze and silver compasses, gold
  maps and compasses, globes, ornate globes, then golden instruments), with one
  explicit emblem variation per rank
- material step-up carries the rank height: gray → bronze → silver → gold, and
  the top tiers add laurels and orbit rings
- isolated on a pure white background
- no text, no letters, no numbers, no watermark
- no border, no frame, no drop shadow

The Replicate token is read from `REPLICATE_API_TOKEN` in the developer's shell
environment. Generation is a one-time manual step, not part of the build: the
finished transparent PNGs are committed under `frontend/public/rank-badges/` as
`{trophy_key}.png` (for example `lost-tourist.png`), one per rank.

Because `image-01` outputs JPEG (no alpha channel), generated images are
post-processed once before being committed: a two-pass border flood fill removes
the white background and the JPEG compression ring around the emblem, near-white
pixels adjacent to transparency are snapped to alpha 0, enclosed near-white
regions above a size threshold are removed (leftover background inside emblem
outlines), and the result is trimmed to its solid content, padded with a uniform
margin, centered, and resized to 256×256. Rarely the model draws a gray vignette
instead of a white background; such images must be regenerated and the same
post-processing applied. The committed PNGs are final assets: regeneration is a
one-off manual step, never part of the build.

To regenerate a badge, run the same prompt recipe for that rank through the
model, post-process with the flood-fill script, verify transparency and margins
programmatically, and review the result before committing.

## Display rules

- A small badge (16–20 px) is shown next to the rank name everywhere a rank name
  is displayed: the profile hero, the profile stat card, and every leaderboard
  row.
- The rank number in roman numerals (I to XXX) is shown beneath the badge
  everywhere the badge is displayed, so the ordering is obvious even when only
  the badge is in view.
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
