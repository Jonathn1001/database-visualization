CREATE TABLE users (
    id UUID PRIMARY KEY,
    email VARCHAR(255) UNIQUE NOT NULL,
    name VARCHAR(255),
    created_at TIMESTAMP DEFAULT NOW()
);
CREATE INDEX idx_users_email ON users(email);

CREATE TABLE addresses (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    street TEXT,
    city VARCHAR(100)
);
CREATE INDEX idx_addresses_city ON addresses(city);

CREATE TABLE categories (
    id UUID PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    parent_id UUID REFERENCES categories(id) ON DELETE SET NULL
);

CREATE TABLE products (
    id UUID PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    price DECIMAL(10,2),
    category_id UUID REFERENCES categories(id) ON DELETE RESTRICT
);
CREATE INDEX idx_products_name ON products(name);

CREATE TABLE orders (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status VARCHAR(50),
    total DECIMAL(10,2),
    created_at TIMESTAMP DEFAULT NOW()
);
CREATE INDEX idx_orders_status ON orders(status);
CREATE INDEX idx_orders_created_at ON orders(created_at);

CREATE TABLE order_items (
    id UUID PRIMARY KEY,
    order_id UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    qty INT NOT NULL,
    price DECIMAL(10,2)
);

CREATE TABLE reviews (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    rating INT CHECK (rating BETWEEN 1 AND 5),
    comment TEXT
);

CREATE TABLE payments (
    id UUID PRIMARY KEY,
    order_id UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    method VARCHAR(50),
    status VARCHAR(50)
);
CREATE INDEX idx_payments_status ON payments(status);

-- Seed rows so sample-data and PII-masking behavior can be exercised.
INSERT INTO users (id, email, name) VALUES
    (gen_random_uuid(), 'alice@example.com', 'Alice'),
    (gen_random_uuid(), 'bob@example.com', 'Bob');

-- A least-privilege read-only role so the adapter does not connect as superuser.
CREATE ROLE dbviz_reader LOGIN PASSWORD 'readonly';
GRANT CONNECT ON DATABASE testdb TO dbviz_reader;
GRANT USAGE ON SCHEMA public TO dbviz_reader;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO dbviz_reader;
GRANT pg_read_all_stats TO dbviz_reader;
