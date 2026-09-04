-- +goose Up
CREATE TABLE seed_runs (
    name text PRIMARY KEY,
    completed_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE seed_runs;
