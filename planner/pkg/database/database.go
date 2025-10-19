package database

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Client represents a database client
type Client struct {
	pool *pgxpool.Pool
}

// New creates a new database client
func New(connStr string) (*Client, error) {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		return nil, fmt.Errorf("pgxpool: %w", err)
	}

	return &Client{pool: pool}, nil
}

// Close closes the database connection
func (c *Client) Close() {
	c.pool.Close()
}

// TemplateMeta represents template metadata from database
type TemplateMeta struct {
	TemplateID   string
	TemplateText string
	Regex        string
	Labels       map[string]string
	Count24h     uint64
	FirstSeen    *time.Time
	LastSeen     *time.Time
}

// FetchTemplates fetches template metadata by IDs for a given tenant
func (c *Client) FetchTemplates(ctx context.Context, tenant string, ids []string) (map[string]TemplateMeta, error) {
	if len(ids) == 0 {
		return map[string]TemplateMeta{}, nil
	}

	// Build IN clause safely
	params := make([]string, len(ids))
	args := make([]any, 0, len(ids)+1)
	for i, id := range ids {
		params[i] = fmt.Sprintf("$%d", i+1)
		args = append(args, id)
	}
	// Always scope by tenant
	params = append(params, fmt.Sprintf("$%d", len(ids)+1))
	args = append(args, tenant)

	q := `
SELECT template_id, template_text, regex, labels, count_24h, first_seen, last_seen
FROM templates
WHERE template_id IN (` + strings.Join(params[:len(ids)], ",") + `)
  AND tenant = ` + params[len(params)-1]

	rows, err := c.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]TemplateMeta, len(ids))
	for rows.Next() {
		var (
			id, text, regex string
			labelsJSON      []byte
			count24         int64
			first, last     *time.Time
		)
		if err := rows.Scan(&id, &text, &regex, &labelsJSON, &count24, &first, &last); err != nil {
			return nil, err
		}
		lbls := map[string]string{}
		_ = json.Unmarshal(labelsJSON, &lbls)
		if count24 < 0 {
			count24 = 0
		}
		out[id] = TemplateMeta{
			TemplateID:   id,
			TemplateText: text,
			Regex:        regex,
			Labels:       lbls,
			Count24h:     uint64(count24),
			FirstSeen:    first,
			LastSeen:     last,
		}
	}
	return out, rows.Err()
}
