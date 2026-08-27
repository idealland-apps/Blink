package miniflux

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/idealland-apps/Blink/internal/config"
	"github.com/idealland-apps/Blink/internal/model"
)

type Client struct {
	config config.Config
	http   *http.Client
}

type ListOptions struct {
	UnreadOnly bool
	Limit      int
	Category   string
	Freshness  time.Duration
}

func New(c config.Config) *Client {
	return &Client{config: c, http: &http.Client{Timeout: 15 * time.Second}}
}

func (c *Client) ListEntries(ctx context.Context, options ListOptions) ([]model.Entry, error) {
	query := url.Values{}
	if options.UnreadOnly {
		query.Set("status", "unread")
	}
	limit := options.Limit
	if limit == 0 {
		limit = 200
	}
	query.Set("limit", strconv.Itoa(limit))
	query.Set("order", "published_at")
	query.Set("direction", "desc")
	if options.Freshness > 0 {
		query.Set("published_after", strconv.FormatInt(time.Now().Add(-options.Freshness).Unix(), 10))
	}
	var response struct {
		Entries []model.Entry `json:"entries"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/v1/entries", query, nil, &response); err != nil {
		return nil, err
	}
	if options.Category == "" {
		return response.Entries, nil
	}
	entries := response.Entries[:0]
	for _, entry := range response.Entries {
		if strings.EqualFold(entry.Feed.Category.Title, options.Category) {
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

func (c *Client) MarkRead(ctx context.Context, id int64) error {
	return c.update(ctx, id, map[string]any{"status": "read"})
}

func (c *Client) MarkUnread(ctx context.Context, id int64) error {
	return c.update(ctx, id, map[string]any{"status": "unread"})
}
func (c *Client) SetStarred(ctx context.Context, id int64, starred bool) error {
	return c.update(ctx, id, map[string]any{"starred": starred})
}

func (c *Client) update(ctx context.Context, id int64, payload map[string]any) error {
	payload["entry_ids"] = []int64{id}
	return c.doJSON(ctx, http.MethodPut, "/v1/entries", nil, payload, nil)
}

func (c *Client) doJSON(ctx context.Context, method, endpoint string, query url.Values, requestBody, responseBody any) error {
	base, err := url.Parse(c.config.URL)
	if err != nil {
		return fmt.Errorf("parse Miniflux URL: %w", err)
	}
	basePath := strings.TrimSuffix(base.Path, "/")
	if strings.HasSuffix(basePath, "/v1") && strings.HasPrefix(endpoint, "/v1/") {
		endpoint = strings.TrimPrefix(endpoint, "/v1")
	}
	base.Path = path.Join(basePath, endpoint)
	base.RawQuery = query.Encode()
	var body *bytes.Reader
	if requestBody != nil {
		data, err := json.Marshal(requestBody)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	} else {
		body = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, base.String(), body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if requestBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.config.Token != "" {
		req.Header.Set("X-Auth-Token", c.config.Token)
	} else {
		req.SetBasicAuth(c.config.Username, c.config.Password)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("Miniflux request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Miniflux %s %s failed: HTTP %d", method, endpoint, resp.StatusCode)
	}
	if responseBody != nil {
		if err := json.NewDecoder(resp.Body).Decode(responseBody); err != nil {
			return fmt.Errorf("decode Miniflux response: %w", err)
		}
	}
	return nil
}
