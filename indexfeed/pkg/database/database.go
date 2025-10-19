package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/lib/pq"
)

// Schema SQL statements
const (
	CreateTemplatesSQL = `
CREATE TABLE IF NOT EXISTS templates (
  tenant           TEXT NOT NULL,
  template_id      TEXT NOT NULL,
  template_text    TEXT NOT NULL,
  regex            TEXT NOT NULL,
  labels           JSONB,
  first_seen       TIMESTAMPTZ,
  last_seen        TIMESTAMPTZ,
  template_version TEXT NOT NULL,
  PRIMARY KEY (tenant, template_id)
);
CREATE INDEX IF NOT EXISTS templates_service_idx ON templates ((labels->>'service'));
CREATE INDEX IF NOT EXISTS templates_env_idx     ON templates ((labels->>'env'));
CREATE INDEX IF NOT EXISTS templates_last_seen   ON templates (last_seen);
`

	CreateTemplateStatsSQL = `
CREATE TABLE IF NOT EXISTS template_stats (
  tenant      TEXT NOT NULL,
  template_id TEXT NOT NULL,
  "window"    TEXT NOT NULL,  -- '10m'|'1h'|'24h'
  count       BIGINT NOT NULL,
  updated_at  TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (tenant, template_id, "window")
);
`

	UpsertTemplateSQL = `
INSERT INTO templates (tenant, template_id, template_text, regex, labels, first_seen, last_seen, template_version)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT (tenant, template_id) DO UPDATE SET
  template_text=EXCLUDED.template_text,
  regex=EXCLUDED.regex,
  labels=EXCLUDED.labels,
  first_seen=LEAST(templates.first_seen, EXCLUDED.first_seen),
  last_seen=GREATEST(templates.last_seen, EXCLUDED.last_seen),
  template_version=EXCLUDED.template_version
WHERE templates.template_version <> EXCLUDED.template_version
   OR templates.last_seen < EXCLUDED.last_seen;
`

	UpsertStatSQL = `
INSERT INTO template_stats (tenant, template_id, "window", count, updated_at)
VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (tenant, template_id, "window") DO UPDATE SET
  count=EXCLUDED.count,
  updated_at=EXCLUDED.updated_at;
`
)

// Client represents a database client
type Client struct {
	db *sql.DB
}

// New creates a new database client
func New(connStr string) (*Client, error) {
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &Client{db: db}, nil
}

// Close closes the database connection
func (c *Client) Close() error {
	return c.db.Close()
}

// InitSchema initializes the database schema
func (c *Client) InitSchema(ctx context.Context) error {
	if _, err := c.db.ExecContext(ctx, CreateTemplatesSQL); err != nil {
		return fmt.Errorf("failed to create templates table: %w", err)
	}

	if _, err := c.db.ExecContext(ctx, CreateTemplateStatsSQL); err != nil {
		return fmt.Errorf("failed to create template_stats table: %w", err)
	}

	return nil
}

// UpsertTemplate upserts a template into the database
func (c *Client) UpsertTemplate(ctx context.Context, tenant, templateID, templateText, regex string, labels map[string]string, firstSeen, lastSeen time.Time, templateVersion string) error {
	labelsJSON, err := json.Marshal(labels)
	if err != nil {
		return fmt.Errorf("failed to marshal labels: %w", err)
	}

	_, err = c.db.ExecContext(ctx, UpsertTemplateSQL,
		tenant, templateID, templateText, regex, labelsJSON, firstSeen, lastSeen, templateVersion)
	if err != nil {
		return fmt.Errorf("failed to upsert template: %w", err)
	}

	return nil
}

// UpsertStat upserts a stat into the database
func (c *Client) UpsertStat(ctx context.Context, tenant, templateID, window string, count int64, updatedAt time.Time) error {
	_, err := c.db.ExecContext(ctx, UpsertStatSQL,
		tenant, templateID, window, count, updatedAt)
	if err != nil {
		return fmt.Errorf("failed to upsert stat: %w", err)
	}

	return nil
}
