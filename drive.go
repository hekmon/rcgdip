package main

import (
	"context"
	"fmt"
	"log"

	"github.com/rclone/rclone/lib/oauthutil"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

const (
	scopePrefix = "https://www.googleapis.com/auth/"
)

func driveTest(clientID, clientSecret string, token *oauth2.Token) {
	// https://developers.google.com/drive/api/v3/quickstart/go
	ctx := context.Background()

	// If modifying these scopes, delete your previously saved token.json.
	driveConfig := &oauth2.Config{
		Scopes:       []string{scopePrefix + "drive"},
		Endpoint:     google.Endpoint,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  oauthutil.TitleBarRedirectURL,
	}
	client := driveConfig.Client(ctx, token)

	srv, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		log.Fatalf("Unable to retrieve Drive client: %v", err)
	}

	r, err := srv.Files.List().PageSize(10).
		Fields("nextPageToken, files(id, name)").Do()
	if err != nil {
		log.Fatalf("Unable to retrieve files: %v", err)
	}
	fmt.Println("Files:")
	if len(r.Files) == 0 {
		fmt.Println("No files found.")
	} else {
		for _, i := range r.Files {
			fmt.Printf("%s (%s)\n", i.Name, i.Id)
		}
	}
}
