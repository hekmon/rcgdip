package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

func (c *Client) GetLibraries(ctx context.Context) (libraries []Library, headers http.Header, err error) {
	var answerPayload librariesAnswerPayload
	// Execute request
	if headers, err = c.request(ctx, "GET", "/library/sections", nil, &answerPayload); err != nil {
		err = fmt.Errorf("failed to execute libraries listing query: %w", err)
		return
	}
	// Extract libraries listing
	libraries = answerPayload.MediaContainer.Directory
	return
}

type librariesAnswerPayload struct {
	MediaContainer struct {
		Size            int       `json:"size"`
		AllowSync       bool      `json:"allowSync"`
		Identifier      string    `json:"identifier"`
		MediaTagPrefix  string    `json:"mediaTagPrefix"`
		MediaTagVersion int       `json:"mediaTagVersion"`
		Title1          string    `json:"title1"`
		Directory       []Library `json:"Directory"`
	} `json:"MediaContainer"`
}

type Library struct {
	AllowSync        bool           `json:"allowSync"`
	Art              string         `json:"art"`
	Composite        string         `json:"composite"`
	Filters          bool           `json:"filters"`
	Refreshing       bool           `json:"refreshing"`
	Thumb            string         `json:"thumb"`
	Key              string         `json:"key"`
	Type             string         `json:"type"`
	Title            string         `json:"title"`
	Agent            string         `json:"agent"`
	Scanner          string         `json:"scanner"`
	Language         string         `json:"language"`
	UUID             string         `json:"uuid"`
	UpdatedAt        time.Time      `json:"-"`
	CreatedAt        time.Time      `json:"-"`
	ScannedAt        time.Time      `json:"-"`
	Content          bool           `json:"content"`
	Directory        bool           `json:"directory"`
	ContentChangedAt int            `json:"contentChangedAt"`
	Hidden           int            `json:"hidden"`
	Locations        map[int]string `json:"-"`
}

type locationRaw struct {
	ID   int    `json:"id"`
	Path string `json:"path"`
}

func (l *Library) MarshalJSON() ([]byte, error) {
	// Prepare locations
	locations := make([]locationRaw, len(l.Locations))
	index := 0
	for id, path := range l.Locations {
		locations[index] = locationRaw{
			ID:   id,
			Path: path,
		}
	}
	// Convert
	type Alias Library
	return json.Marshal(struct {
		UpdatedAt int64         `json:"updatedAt"`
		CreatedAt int64         `json:"createdAt"`
		ScannedAt int64         `json:"scannedAt"`
		Location  []locationRaw `json:"Location"`
		*Alias
	}{
		UpdatedAt: l.UpdatedAt.Unix(),
		CreatedAt: l.CreatedAt.Unix(),
		ScannedAt: l.ScannedAt.Unix(),
		Location:  locations,
		Alias:     (*Alias)(l),
	})
}

func (l *Library) UnmarshalJSON(data []byte) error {
	type Alias Library
	shadow := struct {
		UpdatedAt int64         `json:"updatedAt"`
		CreatedAt int64         `json:"createdAt"`
		ScannedAt int64         `json:"scannedAt"`
		Location  []locationRaw `json:"Location"`
		*Alias
	}{
		Alias: (*Alias)(l),
	}
	if err := json.Unmarshal(data, &shadow); err != nil {
		return err
	}
	l.UpdatedAt = time.Unix(shadow.UpdatedAt, 0)
	l.CreatedAt = time.Unix(shadow.CreatedAt, 0)
	l.ScannedAt = time.Unix(shadow.ScannedAt, 0)
	l.Locations = make(map[int]string, len(shadow.Location))
	for _, lRaw := range shadow.Location {
		l.Locations[lRaw.ID] = lRaw.Path
	}
	return nil
}

func (c *Client) ScanLibrary(ctx context.Context, key, specificPath string) (headers http.Header, err error) {
	// Prepare request
	endpoint := fmt.Sprintf("/library/sections/%s/refresh", key)
	var query url.Values
	if specificPath != "" {
		query = url.Values{
			"path": []string{
				specificPath,
			},
		}
	}
	// Execute request
	if headers, err = c.request(ctx, "GET", endpoint, query, nil); err != nil {
		err = fmt.Errorf("failed to execute library scan query: %w", err)
		return
	}
	return
}
