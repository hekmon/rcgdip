package rcsnooper

import (
	"errors"
	"fmt"

	"github.com/rclone/rclone/backend/crypt"
	"github.com/rclone/rclone/fs"
)

func (c *Controller) initCrypt(backend string) (err error) {
	fsInfo, _, _, config, err := fs.ConfigFs(backend + ":")
	if err != nil {
		return fmt.Errorf("can not get config for backend '%s': %w", backend, err)
	}
	if fsInfo.Name != "crypt" {
		return errors.New("the backend needs to be of type \"crypt\"")
	}
	c.cryptCipher, err = crypt.NewCipher(config)
	if err != nil {
		c.cryptCipher = nil // just be safe, not our package there
		return fmt.Errorf("failed to build crypt cipher for backend '%s': %w", backend, err)
	}
	return
}

func (c *Controller) CryptDecode(encrypted string) (decrypted string, err error) {
	if c.cryptCipher == nil {
		err = errors.New("crypt cipher not initialized")
		return
	}
	return c.cryptCipher.DecryptFileName(encrypted)
}

func (c *Controller) CryptEncode(plain string) (encrypted string, err error) {
	if c.cryptCipher == nil {
		err = errors.New("crypt cipher not initialized")
		return
	}
	encrypted = c.cryptCipher.EncryptFileName(plain)
	return
}
