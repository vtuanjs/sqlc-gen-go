CREATE TABLE filter_items (
    id   INTEGER PRIMARY KEY,
    kind TEXT NOT NULL,
    a    TEXT NOT NULL,
    b    TEXT NOT NULL,
    c    TEXT NOT NULL
);

CREATE INDEX filter_items_a_c_idx ON filter_items (a, c);

CREATE TABLE users (
    id         INTEGER PRIMARY KEY,
    name       TEXT NOT NULL,
    email      TEXT NOT NULL UNIQUE,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    phone      TEXT
);

CREATE TABLE orders (
    id         INTEGER PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users (id),
    amount     REAL NOT NULL,
    status     TEXT NOT NULL DEFAULT 'pending',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE products (
    id         INTEGER PRIMARY KEY,
    name       TEXT,
    price      REAL NOT NULL,
    stock      INTEGER,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
