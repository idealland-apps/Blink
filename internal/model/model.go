package model

import "time"

type Feed struct {
	ID       int64  `json:"id"`
	Title    string `json:"title"`
	SiteURL  string `json:"site_url"`
	Category struct {
		ID    int64  `json:"id"`
		Title string `json:"title"`
	} `json:"category"`
}

type Entry struct {
	ID          int64     `json:"id"`
	FeedID      int64     `json:"feed_id"`
	Title       string    `json:"title"`
	URL         string    `json:"url"`
	Content     string    `json:"content"`
	Status      string    `json:"status"`
	Starred     bool      `json:"starred"`
	ReadingTime int       `json:"reading_time"`
	PublishedAt time.Time `json:"published_at"`
	Feed        Feed      `json:"feed"`
}
