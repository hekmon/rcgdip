package gdrive

import (
	"errors"
	"fmt"
	"path"
	"reflect"
	"strings"
	"time"

	"github.com/hekmon/rcgdip/drivechange"
)

func (c *Controller) watcher(interval time.Duration) {
	// Prepare
	defer c.workers.Done()
	// Validate state locally and against remote drive
	if err := c.validateState(); err != nil {
		c.logger.Errorf("[Drive] failed to validate local state: %s", err)
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
		changesFiles = c.processChangesThruCrypt(changesFiles)
		c.logger.Infof("[Drive] crypt process of changes removed %d paths, remaining: %d", oldNum-len(changesFiles), len(changesFiles))
		if len(changesFiles) == 0 {
			return
		}
	}
	// Rewrited files can generate 2 events: a deletion event followed by a new file event: transform them to a single change event
	oldLen := len(changesFiles)
	changesFiles = c.detectRewrites(changesFiles)
	if oldLen != len(changesFiles) {
		c.logger.Debugf("[Drive] removed %d deletion event(s) because matching path(s) change event were along (rewritten file(s))", oldLen-len(changesFiles))
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

func (c *Controller) processChangesThruCrypt(changesFiles []drivechange.File) (validCryptChangesFiles []drivechange.File) {
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
			if decryptedPath, partOfPrefix, err = c.decryptPath(path, change.Folder); err != nil {
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

func (c *Controller) decryptPath(encryptedPath string, directory bool) (decryptedPath string, partOfPrefix bool, err error) {
	// If no crypt backend
	if c.rc.Crypt.Cipher == nil {
		err = errors.New("can not decrypt with a nil cipher")
		return
	}
	// Check if path is within the crypt remote prefix
	if partOfPrefix = strings.HasPrefix(encryptedPath, c.rc.Crypt.PathPrefix); !partOfPrefix {
		return
	}
	// Remove prefix
	strippedPath := encryptedPath[len(c.rc.Crypt.PathPrefix):]
	if len(strippedPath) == 0 {
		// relative root
		decryptedPath = "/"
		return
	}
	if path.IsAbs(strippedPath) {
		if len(strippedPath) == 1 {
			// relative root
			decryptedPath = strippedPath
			return
		}
		strippedPath = strippedPath[1:]
	}
	// Decrypt
	if directory {
		if decryptedPath, err = c.rc.Crypt.Cipher.DecryptDirName(strippedPath); err != nil {
			err = fmt.Errorf("failed to decrypt dir path: %w", err)
			return
		}
	} else {
		if decryptedPath, err = c.rc.Crypt.Cipher.DecryptFileName(strippedPath); err != nil {
			err = fmt.Errorf("failed to decrypt filename: %w", err)
			return
		}
	}
	c.logger.Debugf("[Drive] path decrypted using '%s' rclone backend configuration: %s  -->  %s",
		c.rc.Conf.CryptBackendName, encryptedPath, decryptedPath)
	return
}

func (c *Controller) detectRewrites(changesFiles []drivechange.File) (cleanedChangesFiles []drivechange.File) {
	cleanedChangesFiles = make([]drivechange.File, 0, len(changesFiles))
candidates:
	for index, changeFile := range changesFiles {
		if changeFile.Deleted {
			// search if there is another change matching theses paths that is not a deletion to avoid adding a delete event for a newly/rewritten file
			for searchIndex, searchChangeFile := range changesFiles {
				// do not compare against self
				if index == searchIndex {
					continue
				}
				// is this event the exact same file (path) ?
				if reflect.DeepEqual(changeFile.Paths, searchChangeFile.Paths) {
					// bingo, files has been rewritten, no need to keep the deletion event
					c.logger.Debugf("[Drive] skipping deletion event of '%s' (index %d) because another change event target the same path(s) (index %d)",
						changeFile.Paths, index, searchIndex)
					continue candidates
				}
			}
		}
		// If we reached here, keep the event
		cleanedChangesFiles = append(cleanedChangesFiles, changeFile)
	}
	return
}
