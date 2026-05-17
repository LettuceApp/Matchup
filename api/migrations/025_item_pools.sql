CREATE TABLE item_pools (
    id SERIAL PRIMARY KEY,
    public_id VARCHAR(32) UNIQUE NOT NULL,
    short_id VARCHAR(8) UNIQUE,
    title VARCHAR(255) NOT NULL,
    description TEXT DEFAULT '',
    author_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
    size INTEGER NOT NULL DEFAULT 0,
    visibility VARCHAR(32) NOT NULL DEFAULT 'public',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_item_pools_author ON item_pools(author_id);
CREATE INDEX idx_item_pools_visibility ON item_pools(visibility);

CREATE TABLE item_pool_items (
    id SERIAL PRIMARY KEY,
    item_pool_id INTEGER REFERENCES item_pools(id) ON DELETE CASCADE,
    item VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_item_pool_items_pool ON item_pool_items(item_pool_id);
