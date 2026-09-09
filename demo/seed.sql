CREATE EXTENSION IF NOT EXISTS pg_stat_statements;

CREATE TABLE users (
    id serial PRIMARY KEY,
    email text NOT NULL UNIQUE,
    plan text NOT NULL DEFAULT 'free',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE orders (
    id serial PRIMARY KEY,
    user_id integer NOT NULL REFERENCES users (id),
    amount_cents integer NOT NULL,
    status text NOT NULL DEFAULT 'pending',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX orders_user_id_idx ON orders (user_id);
CREATE INDEX orders_status_idx ON orders (status);

INSERT INTO users (email, plan) VALUES
    ('ada@example.com', 'enterprise'),
    ('grace@example.com', 'pro'),
    ('linus@example.com', 'free'),
    ('margaret@example.com', 'pro'),
    ('alan@example.com', 'enterprise');

INSERT INTO orders (user_id, amount_cents, status) VALUES
    (1, 49900, 'paid'),
    (1, 19900, 'refunded'),
    (2, 9900, 'paid'),
    (2, 14900, 'paid'),
    (3, 0, 'pending'),
    (4, 29900, 'paid'),
    (5, 99900, 'paid'),
    (5, 49900, 'pending');
