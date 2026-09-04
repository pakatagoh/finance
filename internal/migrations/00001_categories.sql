-- +goose Up
CREATE TABLE categories (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    slug text NOT NULL,
    name text NOT NULL,
    archived_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX categories_slug_ci_uidx ON categories (lower(slug));
CREATE UNIQUE INDEX categories_name_ci_uidx ON categories (lower(name));

-- +goose Down
DROP TABLE categories;
