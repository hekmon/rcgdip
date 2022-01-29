package rcsnooper

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/rclone/rclone/fs"
	"golang.org/x/oauth2"
)

const (
	rcloneConfigGDriveClientIDKey     = "client_id"
	rcloneConfigGDriveClientSecretKey = "client_secret"
	rcloneConfigGDriveScopeKey        = "scope"
	rcloneConfigGDriveTokenKey        = "token"
	rcloneConfigGDriveSAFileKey       = "service_account_file"
	rcloneConfigGDriveRootFolderIDKey = "root_folder_id"
	rcloneConfigGDriveTeameDriveKey   = "team_drive"
)

type DriveBackend struct {
	ClientID     string
	ClientSecret string
	Scope        string
	Token        *oauth2.Token
	RootFolderID string
	TeamDrive    string
}

func (c *Controller) extractDriveBackend(backend string) (err error) {
	// Load backend from config if it exists
	fsInfo, _, _, config, err := fs.ConfigFs(backend + ":")
	if err != nil {
		return fmt.Errorf("can not get config for backend '%s': %w", backend, err)
	}
	if fsInfo.Name != "drive" {
		return errors.New("the backend needs to be of type \"drive\"")
	}
	// Extract values we need
	var (
		found    bool
		tokenRaw string
	)
	if c.Drive.ClientID, found = config.Get(rcloneConfigGDriveClientIDKey); !found {
		return fmt.Errorf("key %s not found", rcloneConfigGDriveClientIDKey)
	}
	if c.Drive.ClientSecret, found = config.Get(rcloneConfigGDriveClientSecretKey); !found {
		return fmt.Errorf("key %s not found", rcloneConfigGDriveClientSecretKey)
	}
	if c.Drive.Scope, found = config.Get(rcloneConfigGDriveScopeKey); !found {
		return fmt.Errorf("key %s not found", rcloneConfigGDriveScopeKey)
	}
	if tokenRaw, found = config.Get(rcloneConfigGDriveTokenKey); found {
		if err = json.Unmarshal([]byte(tokenRaw), &c.Drive.Token); err != nil {
			return fmt.Errorf("failed to parse oauth2 token: %w", err)
		}
	} else if _, found = config.Get(rcloneConfigGDriveSAFileKey); found {
		return errors.New("authentification with service account not yet implemented")
	} else {
		return errors.New("no suitable authentification found (oauth2 or service account)")
	}
	if c.Drive.RootFolderID, found = config.Get(rcloneConfigGDriveRootFolderIDKey); !found {
		fmt.Println("no custom root folder id found")
	}
	if c.Drive.TeamDrive, found = config.Get(rcloneConfigGDriveTeameDriveKey); !found {
		fmt.Println("no team drive found")
	}
	return
}
