CREATE TABLE filter_items (
    id INTEGER PRIMARY KEY,
    a TEXT NOT NULL,
    b TEXT NOT NULL,
    c TEXT NOT NULL
);

CREATE INDEX filter_items_a_c_idx ON filter_items (a, c);
