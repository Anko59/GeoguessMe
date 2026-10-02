CREATE TABLE map_pins (
    pin_key TEXT PRIMARY KEY CHECK (pin_key ~ '^[a-z0-9][a-z0-9_-]{0,63}$'),
    name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 80),
    description TEXT NOT NULL CHECK (length(description) BETWEEN 1 AND 280),
    image_url TEXT NOT NULL CHECK (
        image_url ~ '^/[a-zA-Z0-9][a-zA-Z0-9_./-]*$'
        AND image_url !~ '(^|/)\.\.(/|$)'
    ),
    display_order INTEGER NOT NULL DEFAULT 0,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE map_pin_challenges (
    challenge_key TEXT PRIMARY KEY CHECK (challenge_key ~ '^[a-z0-9][a-z0-9_-]{0,63}$'),
    pin_key TEXT NOT NULL REFERENCES map_pins(pin_key) ON DELETE RESTRICT,
    name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 100),
    description TEXT NOT NULL CHECK (length(description) BETWEEN 1 AND 280),
    criteria JSONB NOT NULL DEFAULT '{}'::JSONB CHECK (jsonb_typeof(criteria) = 'object'),
    display_order INTEGER NOT NULL DEFAULT 0,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (challenge_key, pin_key)
);

CREATE TABLE user_map_pin_unlocks (
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    pin_key TEXT NOT NULL REFERENCES map_pins(pin_key) ON DELETE RESTRICT,
    challenge_key TEXT NOT NULL,
    unlocked_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (user_id, challenge_key),
    UNIQUE (user_id, pin_key, challenge_key),
    FOREIGN KEY (challenge_key, pin_key)
        REFERENCES map_pin_challenges(challenge_key, pin_key) ON DELETE RESTRICT
);

CREATE INDEX user_map_pin_unlocks_by_pin_idx
    ON user_map_pin_unlocks (user_id, pin_key, unlocked_at, challenge_key);

CREATE TABLE user_equipped_map_pins (
    user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    pin_key TEXT NOT NULL,
    challenge_key TEXT NOT NULL,
    FOREIGN KEY (user_id, pin_key, challenge_key)
        REFERENCES user_map_pin_unlocks(user_id, pin_key, challenge_key) ON DELETE CASCADE
);
