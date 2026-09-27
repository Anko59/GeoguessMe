INSERT INTO map_pins (pin_key, name, description, image_url, display_order)
VALUES
    ('bullseye', 'Bullseye', 'A marker for your first perfect guess.', '/assets/map-pins/bullseye-v1.png', 10),
    ('weekly-champion', 'Weekly Champion', 'A marker for finishing first in a group week.', '/assets/map-pins/weekly-champion-v1.png', 20),
    ('20k-club', '20K Club', 'A marker for a week of standout scores.', '/assets/map-pins/20k-club-v1.png', 30),
    ('field-guide', 'Field Guide', 'A marker for sharing locations with other players.', '/assets/map-pins/field-guide-v1.png', 40),
    ('passport-builder', 'Passport Builder', 'A marker for strong guesses across ten countries.', '/assets/map-pins/passport-builder-v1.png', 50),
    ('on-a-roll', 'On a Roll', 'A marker for a run of consistently sharp guesses.', '/assets/map-pins/on-a-roll-v1.png', 60),
    ('close-call', 'Close Call', 'A marker for landing close to the real location.', '/assets/map-pins/close-call-v1.png', 70),
    ('group-explorer', 'Group Explorer', 'A marker for playing across several groups.', '/assets/map-pins/group-explorer-v1.png', 80),
    ('beat-the-clock', 'Beat the Clock', 'A marker for completing timed public guesses.', '/assets/map-pins/beat-the-clock-v1.png', 90),
    ('community-regular', 'Community Regular', 'A marker for exploring the public feed.', '/assets/map-pins/community-regular-v1.png', 100),
    ('globe-trotter', 'Globe Trotter', 'A marker for accurate guesses around the world.', '/assets/map-pins/globe-trotter-v1.svg', 110),
    ('high-scorer', 'High Scorer', 'A marker for a long record of strong scores.', '/assets/map-pins/high-scorer-v1.svg', 120),
    ('streak-master', 'Streak Master', 'A marker for keeping a high scoring streak alive.', '/assets/map-pins/streak-master-v1.svg', 130),
    ('perfect-five', 'Perfect Five', 'A marker for five perfect scores.', '/assets/map-pins/perfect-five-v1.svg', 140),
    ('atlas-scholar', 'Atlas Scholar', 'A marker for high scores across fifteen countries.', '/assets/map-pins/atlas-scholar-v1.svg', 150),
    ('fan-favorite', 'Fan Favorite', 'A marker for drawing a crowd to your public posts.', '/assets/map-pins/fan-favorite-v1.svg', 160),
    ('buzz-magnet', 'Buzz Magnet', 'A marker for earning plenty of public reactions.', '/assets/map-pins/buzz-magnet-v1.svg', 170),
    ('conversation-starter', 'Conversation Starter', 'A marker for joining the public feed conversation.', '/assets/map-pins/conversation-starter-v1.svg', 180),
    ('welcome-mat', 'Welcome Mat', 'A marker for welcoming many players to your locations.', '/assets/map-pins/welcome-mat-v1.svg', 190),
    ('trailblazer', 'Trailblazer', 'A marker for making your way through many groups.', '/assets/map-pins/trailblazer-v1.svg', 200),
    ('night-owl', 'Night Owl', 'A marker for strong public-feed scores before dawn UTC.', '/assets/map-pins/night-owl-v1.svg', 210),
    ('weekend-wanderer', 'Weekend Wanderer', 'A marker for scoring big on a weekend.', '/assets/map-pins/weekend-wanderer-v1.svg', 220),
    ('quick-draw', 'Quick Draw', 'A marker for fast guesses in timed public games.', '/assets/map-pins/quick-draw-v1.svg', 230),
    ('long-haul', 'Long Haul', 'A marker for an impressive lifetime score.', '/assets/map-pins/long-haul-v1.svg', 240),
    ('long-shot', 'Long Shot', 'A marker for accurate guesses from far away.', '/assets/map-pins/long-shot-v1.svg', 250),
    ('double-dedication', 'Double Dedication', 'A marker for scoring well in groups and the public feed.', '/assets/map-pins/double-dedication-v1.svg', 260),
    ('century-club', 'Century Club', 'A marker for a hundred public-feed guesses.', '/assets/map-pins/century-club-v1.svg', 270),
    ('art-curator', 'Art Curator', 'A marker for sharing a wide collection of locations.', '/assets/map-pins/art-curator-v1.svg', 280),
    ('social-butterfly', 'Social Butterfly', 'A marker for supporting other public posts.', '/assets/map-pins/social-butterfly-v1.svg', 290),
    ('trusted-reviewer', 'Trusted Reviewer', 'A marker for commenting across the public feed.', '/assets/map-pins/trusted-reviewer-v1.svg', 300)
ON CONFLICT (pin_key) DO UPDATE
SET name = excluded.name,
    description = excluded.description,
    image_url = excluded.image_url,
    display_order = excluded.display_order;

INSERT INTO map_pin_challenges (challenge_key, pin_key, name, description, criteria, display_order)
VALUES
    (
        'first-perfect-5000', 'bullseye', 'First perfect guess',
        'Score exactly 5,000 points on a single guess for the first time. A doubled 10,000 point Party Time score counts.',
        '{"kind":"single_guess_score","score":5000,"count":1}'::JSONB, 10
    ),
    (
        'weekly-group-champion', 'weekly-champion', 'Weekly Champion',
        'Finish first on a group weekly leaderboard when the week closes. A tie for the top score counts.',
        '{"kind":"group_weekly_rank","rank":1,"tie_policy":"shared_first","week_start":"monday_utc"}'::JSONB, 20
    ),
    (
        'over-20000-week', '20k-club', '20K Club',
        'Earn more than 20,000 points across completed group and public rounds in one UTC calendar week.',
        '{"kind":"weekly_points","threshold_exclusive":20000,"week_start":"monday_utc"}'::JSONB, 30
    ),
    (
        'published-20-locations', 'field-guide', 'Field Guide',
        'Publish 20 distinct public locations for other players to guess.',
        '{"kind":"published_public_locations","count":20}'::JSONB, 40
    ),
    (
        'country-collector-10', 'passport-builder', 'Passport Builder',
        'Score at least 4,000 points on locations in 10 different countries.',
        '{"kind":"country_score_count","minimum_score":4000,"countries":10}'::JSONB, 50
    ),
    (
        'five-strong-in-a-row', 'on-a-roll', 'On a Roll',
        'Score at least 4,000 points on five consecutive guesses. A lower score or timeout breaks the streak.',
        '{"kind":"score_streak","minimum_score":4000,"count":5}'::JSONB, 60
    ),
    (
        'ten-close-guesses', 'close-call', 'Close Call',
        'Place within 1 kilometre of the correct location on 10 guesses.',
        '{"kind":"distance_count","maximum_distance_meters":1000,"count":10}'::JSONB, 70
    ),
    (
        'play-five-groups', 'group-explorer', 'Group Explorer',
        'Complete a scored guess in five different groups.',
        '{"kind":"unique_groups","count":5}'::JSONB, 80
    ),
    (
        'ten-timed-public-guesses', 'beat-the-clock', 'Beat the Clock',
        'Complete 10 timed public-feed guesses before their deadlines.',
        '{"kind":"timed_public_count","count":10}'::JSONB, 90
    ),
    (
        'guess-ten-creators', 'community-regular', 'Community Regular',
        'Complete a public-feed guess on posts from 10 different creators.',
        '{"kind":"public_author_count","count":10}'::JSONB, 100
    ),
    (
        'score-in-twenty-countries', 'globe-trotter', 'Globe Trotter',
        'Score at least 2,500 points on locations in 20 different countries.',
        '{"kind":"country_score_count","minimum_score":2500,"countries":20}'::JSONB, 110
    ),
    (
        'twenty-five-high-scores', 'high-scorer', 'High Scorer',
        'Score at least 4,500 points on 25 completed guesses.',
        '{"kind":"score_count","minimum_score":4500,"count":25}'::JSONB, 120
    ),
    (
        'ten-high-scores-in-a-row', 'streak-master', 'Streak Master',
        'Score at least 4,500 points on 10 consecutive guesses. A lower score or timeout breaks the streak.',
        '{"kind":"score_streak","minimum_score":4500,"count":10}'::JSONB, 130
    ),
    (
        'five-perfect-scores', 'perfect-five', 'Perfect Five',
        'Score exactly 5,000 points on five guesses. A doubled 10,000 point Party Time score counts.',
        '{"kind":"score_count","score":5000,"count":5}'::JSONB, 140
    ),
    (
        'score-in-fifteen-countries', 'atlas-scholar', 'Atlas Scholar',
        'Score at least 4,500 points on locations in 15 different countries.',
        '{"kind":"country_score_count","minimum_score":4500,"countries":15}'::JSONB, 150
    ),
    (
        'receive-one-hundred-guesses', 'fan-favorite', 'Fan Favorite',
        'Receive 100 completed guesses on your public posts.',
        '{"kind":"received_public_guesses","count":100}'::JSONB, 160
    ),
    (
        'receive-one-hundred-reactions', 'buzz-magnet', 'Buzz Magnet',
        'Receive 100 reactions from other players on your public posts.',
        '{"kind":"received_public_reactions","count":100}'::JSONB, 170
    ),
    (
        'write-fifty-public-comments', 'conversation-starter', 'Conversation Starter',
        'Write 50 comments on public posts.',
        '{"kind":"authored_public_comments","count":50}'::JSONB, 180
    ),
    (
        'welcome-twenty-players', 'welcome-mat', 'Welcome Mat',
        'Have 20 different players complete guesses on your public posts.',
        '{"kind":"received_public_guessers","count":20}'::JSONB, 190
    ),
    (
        'play-ten-groups', 'trailblazer', 'Trailblazer',
        'Complete a scored guess in 10 different groups.',
        '{"kind":"unique_groups","count":10}'::JSONB, 200
    ),
    (
        'ten-thousand-before-dawn', 'night-owl', 'Night Owl',
        'Earn at least 10,000 points from public-feed guesses recorded between 00:00 and 04:59 UTC.',
        '{"kind":"utc_hour_points","start_utc_hour":0,"end_utc_hour":5,"threshold_exclusive":9999}'::JSONB, 210
    ),
    (
        'twenty-thousand-weekend', 'weekend-wanderer', 'Weekend Wanderer',
        'Earn at least 20,000 points from Saturday and Sunday guesses within one UTC calendar week.',
        '{"kind":"weekend_points","threshold_exclusive":20000,"week_start":"monday_utc"}'::JSONB, 220
    ),
    (
        'ten-quick-timed-guesses', 'quick-draw', 'Quick Draw',
        'Complete 10 timed public-feed guesses within 30 seconds after each guess window opens.',
        '{"kind":"timed_public_speed","count":10,"seconds":30}'::JSONB, 230
    ),
    (
        'one-hundred-thousand-points', 'long-haul', 'Long Haul',
        'Earn at least 100,000 total points across completed group and public rounds.',
        '{"kind":"total_points","threshold_exclusive":100000}'::JSONB, 240
    ),
    (
        'five-long-shot-guesses', 'long-shot', 'Long Shot',
        'Score at least 3,000 points on five guesses made from at least 5 kilometres away.',
        '{"kind":"distance_score_count","minimum_distance_meters":5000,"minimum_score":3000,"count":5}'::JSONB, 250
    ),
    (
        'strong-in-groups-and-public', 'double-dedication', 'Double Dedication',
        'Score at least 4,000 points on 10 group guesses and 10 public-feed guesses.',
        '{"kind":"source_score_count","minimum_score":4000,"group_count":10,"public_count":10}'::JSONB, 260
    ),
    (
        'one-hundred-public-guesses', 'century-club', 'Century Club',
        'Complete 100 guesses on public-feed posts.',
        '{"kind":"public_guess_count","count":100}'::JSONB, 270
    ),
    (
        'publish-fifty-locations', 'art-curator', 'Art Curator',
        'Publish 50 distinct public locations for other players to guess.',
        '{"kind":"published_public_locations","count":50}'::JSONB, 280
    ),
    (
        'react-to-fifty-public-posts', 'social-butterfly', 'Social Butterfly',
        'React to 50 different public posts from other players.',
        '{"kind":"public_reaction_count","count":50}'::JSONB, 290
    ),
    (
        'comment-on-twenty-five-posts', 'trusted-reviewer', 'Trusted Reviewer',
        'Comment on 25 different public posts.',
        '{"kind":"public_commented_challenges","count":25}'::JSONB, 300
    )
ON CONFLICT (challenge_key) DO UPDATE
SET pin_key = excluded.pin_key,
    name = excluded.name,
    description = excluded.description,
    criteria = excluded.criteria,
    display_order = excluded.display_order;
