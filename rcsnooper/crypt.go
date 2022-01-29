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
			return fmt.Errorf("the crypt backend '%s' should have the declared drive backend as remote: '%s:'",
				cryptBackend, driveBackend)
		}
	} else {
		return fmt.Errorf("the crypt backend '%s' does not have a remote declared", cryptBackend)
	}
	// Init the crypt cipher with config
	c.cryptCipher, err = crypt.NewCipher(config)
	if err != nil {
		c.cryptCipher = nil // just be safe, not our package there
		return fmt.Errorf("failed to build crypt cipher for backend '%s': %w", cryptBackend, err)
	}
	return
}

func (c *Controller) CryptDecode(encrypted string) (decrypted string, err error) {
	if c.cryptCipher == nil {
		decrypted = encrypted
		return
	}
	return c.cryptCipher.DecryptFileName(encrypted)
}
