package rcsnooper

import (
	"errors"
	"fmt"

	"github.com/rclone/rclone/backend/crypt"
	"github.com/rclone/rclone/fs"
)

const (
	rcloneConfigCryptRemoteKey = "remote"
)

func (c *Controller) initCrypt(cryptBackend, driveBackend string) (err error) {
	// Get backend info from config
	fsInfo, _, _, config, err := fs.ConfigFs(cryptBackend + ":")
	if err != nil {
		return fmt.Errorf("can not get config for backend '%s': %w", cryptBackend, err)
	}
	// Checks
	if fsInfo.Name != "crypt" {
		return errors.New("the backend needs to be of type \"crypt\"")
	}
	if cryptRemote, found := config.Get(rcloneConfigCryptRemoteKey); found {
		if cryptRemote != driveBackend+":" {
			return fmt.Errorf("the crypt backend '%s' should have as remote: '%s:' (currently: '%s')",
				cryptBackend, driveBackend, cryptRemote)
		}
	} else {
		return fmt.Errorf("the crypt backend '%s' does not have a remote declared", cryptBackend)
	}
	// Init the crypt cipher with config
	if c.CryptCipher, err = crypt.NewCipher(config); err != nil {
		c.CryptCipher = nil // just be safe, not our package here
		return fmt.Errorf("failed to init rclone crypt cipher for backend '%s': %w", cryptBackend, err)
	}
	return
}
