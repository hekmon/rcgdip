package gdrive

import (
	"context"
	"fmt"

	"github.com/hekmon/rcgdip/rcsnooper"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

const (
	scopePrefix = "https://www.googleapis.com/auth/"
)

func New(ctx context.Context, driveConfig rcsnooper.DriveBackend) (c *Controller, err error) {
	c = new(Controller)
	// OAuth2 configuration
	oauthConf := &oauth2.Config{
		Scopes:       []string{scopePrefix + driveConfig.Scope},
		Endpoint:     google.Endpoint,
		ClientID:     driveConfig.ClientID,
		ClientSecret: driveConfig.ClientSecret,
		// RedirectURL:  oauthutil.TitleBarRedirectURL,
	}
	client := oauthConf.Client(ctx, driveConfig.Token)
	// Init Drive client
	if c.driveClient, err = drive.NewService(ctx, option.WithHTTPClient(client)); err != nil {
		err = fmt.Errorf("unable to initialize Drive client: %w", err)
		return
	}
	return
}

type Controller struct {
	driveClient *drive.Service
}
