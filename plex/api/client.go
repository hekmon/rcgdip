package api

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"runtime"
)

type Config struct {
	// Base config
	BaseURL string
	Token   string
	// Advanced config
	ProductName    string // Plex application name, eg Laika, Plex Media Server, Media Link
	ProductVersion string // Plex application version number
	ClientID       string // UUID, serial number, or other number unique per device
	CustomClient   *http.Client
}

type Client struct {
	// User config
	baseURL *url.URL
	token   string
	// Default headers
	defaultHeaders func() http.Header
	// Controller
	http *http.Client
}

func New(conf Config) (c *Client, err error) {
	defer func() {
		if err != nil {
			c = nil
		}
	}()
	// Base init
	if conf.CustomClient == nil {
		conf.CustomClient = http.DefaultClient
	}
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "generic " + runtime.GOOS + "/" + runtime.GOARCH
	}
	c = &Client{
		token: conf.Token,
		// https://github.com/Arcanemagus/plex-api/wiki/Plex-Web-API-Overview#request-headers
		defaultHeaders: func() http.Header {
			return http.Header{
				"X-Plex-Platform": []string{runtime.GOOS},
				// "X-Plex-Platform-Version":  []string{},
				"X-Plex-Provides":          []string{"controller"},
				"X-Plex-Client-Identifier": []string{conf.ClientID},
				"X-Plex-Product":           []string{conf.ProductName},
				"X-Plex-Version":           []string{conf.ProductVersion},
				"X-Plex-Device":            []string{hostname},
				"Accept":                   []string{"application/json"},
			}
		},
		http: conf.CustomClient,
	}
	// Validate base URL
	if c.baseURL, err = url.Parse(conf.BaseURL); err != nil {
		err = fmt.Errorf("failed to parse plex base URL: %w", err)
		return
	}
	if len(c.baseURL.Path) > 0 && c.baseURL.Path[len(c.baseURL.Path)-1] == '/' {
		c.baseURL.Path = c.baseURL.Path[:len(c.baseURL.Path)-1]
	}
	return
}
