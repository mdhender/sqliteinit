-- sqliteinit infrastructure schema
-- This script is owned by the sqliteinit package and creates the
-- schema_migrations and config tables used for migration tracking.

PRAGMA foreign_keys = ON;
PRAGMA busy_timeout = 5000;

-- All timestamps are stored as Unix seconds in UTC.

CREATE TABLE schema_migrations (
    id         INTEGER NOT NULL PRIMARY KEY,
    comment    TEXT    NOT NULL,
    path       TEXT    NOT NULL UNIQUE,
    applied_at INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE config (
    key        TEXT    NOT NULL PRIMARY KEY,
    value      TEXT    NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

-- Initial config rows. schema.version is updated by each migration.
-- app.version and db.created_at are populated after init if AppVersion is set.
INSERT INTO config (key, value, created_at, updated_at)
VALUES ('schema.version', '0', 0, 0),
       ('app.version', '', 0, 0),
       ('db.created_at', '', 0, 0);
