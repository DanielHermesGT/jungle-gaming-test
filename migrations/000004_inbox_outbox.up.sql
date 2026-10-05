-- Inbox (SQS dedup) + transactional outbox (README §6.5 / §11).

CREATE TABLE outbox_events (
    id               TEXT        PRIMARY KEY,
    event_type       TEXT        NOT NULL,
    aggregate_id     TEXT        NOT NULL,
    aggregate_type   TEXT        NOT NULL,
    payload          JSONB       NOT NULL,
    headers          JSONB       NOT NULL DEFAULT '{}'::jsonb,
    occurred_at      TIMESTAMPTZ NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL,
    published_at     TIMESTAMPTZ NULL,
    attempts         INT         NOT NULL DEFAULT 0,
    next_attempt_at  TIMESTAMPTZ NOT NULL,
    last_error       TEXT        NULL,
    locked_at        TIMESTAMPTZ NULL,
    locked_by        TEXT        NULL
);

CREATE INDEX outbox_events_pending_idx
    ON outbox_events (next_attempt_at, id)
    WHERE published_at IS NULL;

CREATE INDEX outbox_events_aggregate_idx
    ON outbox_events (aggregate_id);

CREATE TABLE inbox_messages (
    consumer_name  TEXT        NOT NULL,
    message_id     TEXT        NOT NULL,
    payload_hash   TEXT        NOT NULL,
    received_at    TIMESTAMPTZ NOT NULL,
    completed_at   TIMESTAMPTZ NULL,
    PRIMARY KEY (consumer_name, message_id)
);
