package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

const (
	tokenHeader = "X-Plex-Token"
)

func (c *Client) request(ctx context.Context, method, endpoint string, addQueryParameters url.Values, output interface{}) (headers http.Header, err error) {
	// Prepare URL
	requestURL := *c.baseURL
	requestURL.Path += endpoint
	if addQueryParameters != nil {
		queryParameters := requestURL.Query()
		for queryKey, queryValues := range addQueryParameters {
			for _, queryValue := range queryValues {
				queryParameters.Add(queryKey, queryValue)
			}
		}
		requestURL.RawQuery = queryParameters.Encode()
	}
	// fmt.Println(requestURL.String())
	// Build HTTP request
	req, err := http.NewRequestWithContext(ctx, method, requestURL.String(), nil)
	if err != nil {
		err = fmt.Errorf("failed to build HTTP query: %w", err)
		return
	}
	req.Header = c.defaultHeaders()
	req.Header.Set(tokenHeader, c.token)
	// Execute HTTP request
	resp, err := c.http.Do(req)
	if err != nil {
		err = fmt.Errorf("failed to execute HTTP query: %w", err)
		return
	}
	defer resp.Body.Close()
	headers = resp.Header
	// Check status code
	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("request status error: %s", resp.Status)
		return
	}
	// Unmarshall
	if output != nil {
		if err = json.NewDecoder(resp.Body).Decode(output); err != nil {
			err = fmt.Errorf("failed to decode response payload as JSON: %w", err)
			return
		}
	}
	return
}
