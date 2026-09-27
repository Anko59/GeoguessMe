# Personalized map pins

Players select an unlocked pin in **Settings → Profile**. The chosen artwork
appears beside that player's locations on the group globe and in challenge
results. The profile card names the challenge credited for the equipped pin.
Clearing the selection restores the standard marker and keeps unlock history.

Opening the settings pin picker runs the authenticated progression action. It
checks recorded history and awards newly completed challenges once, including
eligible history from before the catalog was introduced. Weekly group winners
are credited after their UTC week closes. Newly completed challenges appear the
next time the player opens the picker.

## Pin catalog

All 30 launch pins and their server-checked unlock rules are listed below.
Timed-out rounds do not count toward score, distance, streak, or guess totals.

|   # | Pin                      | Unlock rule                                                                                                                    |
| --: | ------------------------ | ------------------------------------------------------------------------------------------------------------------------------ |
|   1 | **Bullseye**             | Score exactly 5,000 points once. A group guess doubled to 10,000 by Party Time counts as the underlying perfect score.         |
|   2 | **Weekly Champion**      | Finish first on a group's Monday-to-Monday UTC weekly leaderboard after the week closes. Ties for first count.                 |
|   3 | **20K Club**             | Earn more than 20,000 points across completed group and public rounds in one UTC calendar week. Party Time bonus points count. |
|   4 | **Field Guide**          | Publish 20 public-feed locations at distinct exact coordinates. Friends-only posts and duplicate coordinates do not count.     |
|   5 | **Passport Builder**     | Score at least 4,000 points on locations in 10 different Natural Earth Admin 0 areas.                                          |
|   6 | **On a Roll**            | Score at least 4,000 points on five consecutive guesses. A lower score or timeout breaks the streak.                           |
|   7 | **Close Call**           | Place within 1 kilometre of the correct location on 10 guesses.                                                                |
|   8 | **Group Explorer**       | Complete a scored guess in five different groups.                                                                              |
|   9 | **Beat the Clock**       | Complete 10 timed public-feed guesses before their deadlines.                                                                  |
|  10 | **Community Regular**    | Complete public-feed guesses on posts from 10 different creators.                                                              |
|  11 | **Globe Trotter**        | Score at least 2,500 points on locations in 20 different countries.                                                            |
|  12 | **High Scorer**          | Score at least 4,500 points on 25 completed guesses.                                                                           |
|  13 | **Streak Master**        | Score at least 4,500 points on 10 consecutive guesses. A lower score or timeout breaks the streak.                             |
|  14 | **Perfect Five**         | Score exactly 5,000 points on five guesses. A doubled 10,000 point Party Time score counts.                                    |
|  15 | **Atlas Scholar**        | Score at least 4,500 points on locations in 15 different countries.                                                            |
|  16 | **Fan Favorite**         | Receive 100 completed guesses from other players on your public posts.                                                         |
|  17 | **Buzz Magnet**          | Receive 100 reactions from other players on your public posts.                                                                 |
|  18 | **Conversation Starter** | Write 50 comments on public posts.                                                                                             |
|  19 | **Welcome Mat**          | Have 20 different players complete guesses on your public posts.                                                               |
|  20 | **Trailblazer**          | Complete a scored guess in 10 different groups.                                                                                |
|  21 | **Night Owl**            | Earn at least 10,000 points from public-feed guesses recorded between 00:00 and 04:59 UTC.                                     |
|  22 | **Weekend Wanderer**     | Earn at least 20,000 points from Saturday and Sunday guesses within one UTC calendar week.                                     |
|  23 | **Quick Draw**           | Complete 10 timed public-feed guesses within 30 seconds after each guess window opens.                                         |
|  24 | **Long Haul**            | Earn at least 100,000 total points across completed group and public rounds.                                                   |
|  25 | **Long Shot**            | Score at least 3,000 points on five guesses made from at least 5 kilometres away.                                              |
|  26 | **Double Dedication**    | Score at least 4,000 points on 10 group guesses and 10 public-feed guesses.                                                    |
|  27 | **Century Club**         | Complete 100 guesses on public-feed posts.                                                                                     |
|  28 | **Art Curator**          | Publish 50 distinct public-feed locations.                                                                                     |
|  29 | **Social Butterfly**     | React to 50 different public posts from other players.                                                                         |
|  30 | **Trusted Reviewer**     | Comment on 25 different public posts.                                                                                          |

## Offline country boundaries

Passport Builder, Globe Trotter, and Atlas Scholar use the bundled Natural Earth
10m Admin 0 Countries 5.1.1 dataset. The source data is public domain and has
258 uniquely coded Admin 0 areas; its default boundaries reflect de facto
control. No geocoding request or other runtime network access is made. A point
in water or outside the data does not count. A coordinate on a shared or
disputed border resolves deterministically to the first matching Admin 0 code in
lexical order.

The dataset treats some territories as separate Admin 0 areas. This is the
country-count convention used by these challenges, so locations in Greenland and
Denmark resolve to separate codes. Boundary and country definitions follow the
bundled dataset rather than a live political registry. See the
[Natural Earth country dataset](https://www.naturalearthdata.com/downloads/10m-cultural-vectors/10m-admin-0-countries/)
and the [backend asset provenance note](../backend/internal/geography/README.md)
for the version and source file.

## Progression behavior

The server evaluates each rule from recorded gameplay and public-feed activity
and stores the challenge that granted the pin. Unlock records are idempotent and
cannot be granted by client requests. Public-post rules only count posts whose
audience is public; friends-only posts do not contribute.
