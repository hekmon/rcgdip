package rcsnooper

import (
	"fmt"

	"github.com/rclone/rclone/backend/crypt"
	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/config"
	"github.com/rclone/rclone/fs/config/configfile"
)

type Config struct {
	RCloneConfigPath string
	DriveBackendName string
	CryptBackendName string
}

type Controller struct {
	// rclone config
	Drive       DriveBackend
	cryptCipher *crypt.Cipher
}

func New(conf Config) (rcsnooper *Controller, err error) {
	rcsnooper = new(Controller)
	// Load RClone config into rclone modules
	if err = rcsnooper.loadRCloneConfig(conf.RCloneConfigPath); err != nil {
		err = fmt.Errorf("can not get RClone configuration: %w", err)
		return
	}
	// Extract drive configuration
	if err = rcsnooper.extractDriveBackend(conf.DriveBackendName); err != nil {
		err = fmt.Errorf("can not extract drive backend '%s' from RClone configuration: %w",
			conf.DriveBackendName, err)
		return
	}
	// Initialize crypt cypher for path decryption
	if conf.CryptBackendName != "" {
		if err = rcsnooper.initCrypt(conf.CryptBackendName, conf.DriveBackendName); err != nil {
			err = fmt.Errorf("failed to initialize the crypt backend: %w", err)
			return
		}
	}
	return
}

func (c *Controller) loadRCloneConfig(configPath string) (err error) {
	// Initialize rclone config
	err = config.SetConfigPath(configPath)
	if err != nil {
		err = fmt.Errorf("failed to set config path in rclone config module: %w", err)
		return
	}
	storageConfig := &configfile.Storage{}
	if err = storageConfig.Load(); err != nil {
		err = fmt.Errorf("failed to load rclone config: %w", err)
	}
	config.SetData(storageConfig)
	// Make further call to fs.ConfigFs() for drive happy
	fs.Register(&fs.RegInfo{
		Name: "drive",
	})
	return
}
