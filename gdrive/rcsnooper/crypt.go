package rcsnooper

import (
	"fmt"
	"strings"

	"github.com/rclone/rclone/backend/crypt"
	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/config/configstruct"
)

type CryptBackend struct {
	Options    crypt.Options
	RemoteName string
	PathPrefix string
	Cipher     *crypt.Cipher
}

func (c *Controller) initCrypt(cryptBackend, driveBackend string) (err error) {
	// Get backend info from config
	fsInfo, _, _, config, err := fs.ConfigFs(cryptBackend + ":")
	if err != nil {
		return fmt.Errorf("can not get config for backend '%s': %w", cryptBackend, err)
	}
	if fsInfo.Name != "crypt" {
		return fmt.Errorf("backend '%s' should have \"crypt\" type, currently have: %s", cryptBackend, fsInfo.Name)
	}
	if err = configstruct.Set(config, &c.Crypt.Options); err != nil {
		return fmt.Errorf("can not extract config of the backend '%s' as crypt options: %w", cryptBackend, err)
	}
	// Process remote
	splittedRemote := strings.Split(c.Crypt.Options.Remote, ":")
	if len(splittedRemote) != 2 {
		return fmt.Errorf("crypt remote has invalid format, expecting '<backend>:[<optionnal/path>]': %s", c.Crypt.Options.Remote)
	}
	c.Crypt.RemoteName, c.Crypt.PathPrefix = splittedRemote[0], splittedRemote[1]
	// Check the crypt remote is targeting the right drive backend
	if c.Crypt.RemoteName != driveBackend {
		return fmt.Errorf("the crypt backend '%s' should have as remote: '%s' (currently: '%s')",
			cryptBackend, driveBackend, c.Crypt.RemoteName)
	}
	// Init the crypt cipher with config
	if c.Crypt.Cipher, err = crypt.NewCipher(config); err != nil {
		c.Crypt.Cipher = nil // just be safe, not our package here
		return fmt.Errorf("failed to init rclone crypt cipher for backend '%s': %w", cryptBackend, err)
	}
	return
}
