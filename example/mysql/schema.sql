CREATE TABLE filter_items (
    id   BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    kind VARCHAR(255) NOT NULL,
    a    VARCHAR(255) NOT NULL,
    b    VARCHAR(255) NOT NULL,
    c    VARCHAR(255) NOT NULL
);

CREATE INDEX filter_items_a_c_idx ON filter_items (a, c);

CREATE TABLE users (
    id         BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    name       VARCHAR(255) NOT NULL,
    email      VARCHAR(255) NOT NULL UNIQUE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    phone      VARCHAR(255)
);

CREATE TABLE orders (
    id         BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    user_id    BIGINT NOT NULL,
    amount     DECIMAL(10,2) NOT NULL,
    status     VARCHAR(255) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT orders_user_id_fk FOREIGN KEY (user_id) REFERENCES users (id)
);

CREATE TABLE products (
    id         BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    name       VARCHAR(255),
    price      DECIMAL(10,2) NOT NULL,
    stock      INT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
