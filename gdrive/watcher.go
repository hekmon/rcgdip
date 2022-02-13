package gdrive

import (
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/hekmon/rcgdip/drivechange"
)

func (c *Controller) watcher(interval time.Duration) {
	// Prepare
	defer c.workers.Done()
	// Has the rclone backend changed ?
	var (
		sameDrive bool
		err       error
	)
	if sameDrive, err = c.validateStateAgainstRemoteDrive(); err != nil {
		c.logger.Errorf("[Drive] failed to validate if remote drive has changed: %s", err)
		if c.ctx.Err() == nil {
			c.killSwitch()
		}
		return
	}
	// Fresh start ? (or reset)
	if err := c.initState(!sameDrive); err != nil {
		c.logger.Errorf("[Drive] failed to initialize local state: %s", err)
		if c.ctx.Err() == nil {
			c.killSwitch()
		}
		return
	}
	// Start the watch
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	c.logger.Infof("[Drive] will check for changes every %v", interval)
	for {
		select {
		case <-ticker.C:
			c.workerPass()
		case <-c.ctx.Done():
			c.logger.Debug("[Drive] stopping watcher as main context has been cancelled")
			return
		}
	}
}

func (c *Controller) workerPass() {
	c.logger.Debug("[Drive] checking changes...")
	// Compute the paths containing changes
	changesFiles, err := c.getFilesChanges()
	if err != nil {
		c.logger.Errorf("failed to retreived changed files: %s", err)
		return
	}
	if len(changesFiles) == 0 {
		return
	}
	// Walk thru results to log and decrypt if needed (and remove paths not part of crypt prefix)
	if c.rc.Crypt.Cipher != nil {
		oldNum := len(changesFiles)
		changesFiles = c.processCryptChanges(changesFiles)
		c.logger.Infof("[Drive] crypt process of changes removed %d paths, remaining: %d", oldNum-len(changesFiles), len(changesFiles))
		if len(changesFiles) == 0 {
			return
		}
	}
	// Print valid final changes
	if c.logger.IsInfoShown() {
		var (
			fileType      string
			deletedSuffix string
		)
		for _, change := range changesFiles {
			for _, path := range change.Paths {
				if change.Folder {
					fileType = "directory"
				} else {
					fileType = "file"
				}
				if change.Deleted {
					deletedSuffix = " (removed)"
				} else {
					deletedSuffix = ""
				}
				c.logger.Infof("[Drive] %s change detected: %s%s", fileType, path, deletedSuffix)
			}
		}
	}
	// Send the collection to the consumer
	c.logger.Debug("[Drive] sending change(s)...")
	c.output <- changesFiles
	c.logger.Debugf("[Drive] sent %d change(s)", len(changesFiles))
}

func (c *Controller) processCryptChanges(changesFiles []drivechange.File) (validCryptChangesFiles []drivechange.File) {
	// Prepare
	var (
		err           error
		decryptedPath string
		partOfPrefix  bool
		validPaths    []string
	)
	validCryptChangesFiles = make([]drivechange.File, 0, len(changesFiles))
	// Process each path
	for _, change := range changesFiles {
		validPaths = make([]string, 0, len(change.Paths))
		for _, path := range change.Paths {
			// decrypt what needs to be decrypted
			if decryptedPath, partOfPrefix, err = c.handleCryptPath(path, change.Folder); err != nil {
				c.logger.Errorf("[Drive] can not decrypt path '%s': %s", path, err)
				continue
			}
			// this path may be in the scope of the drive backend, it is not within the crypt backend prefix, skipping
			if !partOfPrefix {
				c.logger.Debugf("[Drive] path '%s' is not part of the crypt prefix '%s': skipping", path, c.rc.Crypt.PathPrefix)
				continue
			}
			// if everything is ok add it as valid path
			validPaths = append(validPaths, decryptedPath)
		}
		if len(validPaths) > 0 {
			change.Paths = validPaths
			validCryptChangesFiles = append(validCryptChangesFiles, change)
		}
	}
	return
}

func (c *Controller) handleCryptPath(path2process string, directory bool) (decryptedPath string, partOfPrefix bool, err error) {
	// If no crypt backend
	if c.rc.Crypt.Cipher == nil {
		err = errors.New("can not decrypt with a nil cipher")
		return
	}
	// Check if path is within the crypt remote prefix
	if partOfPrefix = strings.HasPrefix(path2process, c.rc.Crypt.PathPrefix); !partOfPrefix {
		return
	}
	// Remove prefix
	strippedPath := path2process[len(c.rc.Crypt.PathPrefix):]
	if path.IsAbs(strippedPath) {
		strippedPath = strippedPath[1:]
	}
	// Decrypt
	if directory {
		if decryptedPath, err = c.rc.Crypt.Cipher.DecryptDirName(strippedPath); err != nil {
			err = fmt.Errorf("failed to decrypt base path: %w", err)
			return
		}
	} else {
		if decryptedPath, err = c.rc.Crypt.Cipher.DecryptFileName(strippedPath); err != nil {
			err = fmt.Errorf("failed to decrypt filename: %w", err)
			return
		}
	}
	c.logger.Debugf("[Drive] path decrypted using '%s' rclone backend configuration: %s  -->  %s",
		c.rc.Conf.CryptBackendName, path2process, decryptedPath)
	return
}
