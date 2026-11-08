CREATE TABLE client (
    client_id TEXT PRIMARY KEY,
    display_name TEXT NOT NULL,
    reconnect_token_hash BLOB,
    created_at TEXT NOT NULL,
    last_seen_at TEXT
);

CREATE TABLE session (
    session_id TEXT PRIMARY KEY,
    game_id TEXT NOT NULL,
    created_at TEXT NOT NULL,
    closed_at TEXT
);

CREATE TABLE "match" (
    match_id INTEGER PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES session(session_id),
    match_number INTEGER NOT NULL CHECK (match_number > 0),
    game_id TEXT NOT NULL,
    seed BLOB,
    started_at TEXT NOT NULL,
    ended_at TEXT NOT NULL,
    termination_reason TEXT NOT NULL,
    is_draw INTEGER NOT NULL CHECK (is_draw IN (0, 1)),
    action_count INTEGER NOT NULL CHECK (action_count >= 0),
    UNIQUE (session_id, match_number)
);


CREATE TABLE match_participant (
    match_id INTEGER NOT NULL REFERENCES "match"(match_id) ON DELETE CASCADE,
    client_id TEXT NOT NULL REFERENCES client(client_id),
    role TEXT NOT NULL,
    seat INTEGER NOT NULL CHECK (seat >= 0),
    is_winner INTEGER NOT NULL CHECK (is_winner IN (0, 1)),
    score REAL,
    actions_accepted INTEGER NOT NULL DEFAULT 0 CHECK (actions_accepted >= 0),
    actions_rejected INTEGER NOT NULL DEFAULT 0 CHECK (actions_rejected >= 0),
    PRIMARY KEY (match_id, client_id),
    UNIQUE (match_id, seat)
);


CREATE TABLE action_log (
    match_id INTEGER NOT NULL REFERENCES "match"(match_id) ON DELETE CASCADE,
    seq INTEGER NOT NULL CHECK (seq > 0),
    client_id TEXT NOT NULL REFERENCES client(client_id),
    received_at TEXT NOT NULL,
    payload BLOB NOT NULL,
    outcome TEXT NOT NULL CHECK (outcome IN ('ACCEPTED', 'REJECTED')),
    rejection_code TEXT,
    rejection_reason TEXT,
    state_version INTEGER,
    PRIMARY KEY (match_id, seq)
);
