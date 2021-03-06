package gdrive

import (
	"fmt"
	"path"
)

type driveFilePath []driveFilePathElem

func (dfp *driveFilePath) CutAt(elemID string) bool {
	for index, elem := range *dfp {
		if elem.ID == elemID {
			*dfp = (*dfp)[:index]
			return true
		}
	}
	return false
}

func (dfp driveFilePath) Path() string {
	names := make([]string, len(dfp))
	for index, elem := range dfp {
		names[index] = elem.Name
	}
	return path.Join(names...)
}

func (dfp driveFilePath) Reverse() (reversed driveFilePath) {
	reversed = make(driveFilePath, len(dfp))
	for index, elem := range dfp {
		reversed[len(dfp)-1-index] = elem
	}
	return
}

type driveFilePathElem struct {
	ID   string
	Name string
}

func (c *Controller) generateReversePaths(fileID string) (buildedPaths []driveFilePath, err error) {
	// Obtain infos for current fileID
	var fileInfos driveFileBasicInfo
	found, err := c.index.Get(fileID, &fileInfos)
	if err != nil {
		err = fmt.Errorf("failed to query the index for fileID '%s': %w", fileID, err)
		return
	}
	if !found {
		err = fmt.Errorf("fileID '%s' not found in the local index", fileID)
		return
	}
	// Stop if no parent, we have reached root folder
	if len(fileInfos.Parents) == 0 {
		return
	}
	// Follow the white rabbit
	buildedPaths = make([]driveFilePath, len(fileInfos.Parents))
	var (
		parentPaths []driveFilePath
		currentPath driveFilePath
	)
	for parentIndex, parent := range fileInfos.Parents {
		// Get paths for this parent
		if parentPaths, err = c.generateReversePaths(parent); err != nil {
			err = fmt.Errorf("failed to lookup parent path for folderID '%s': %w", parent, err)
			return
		}
		// If parent is root folder, just add ourself in this path
		if parentPaths == nil {
			buildedPaths[parentIndex] = driveFilePath{
				{
					ID:   fileID,
					Name: fileInfos.Name,
				},
			}
			continue
		}
		// Else add paths to final return while prefixing with current file/folder name
		for _, parentPath := range parentPaths {
			currentPath = make(driveFilePath, len(parentPath)+1)
			// prefix ourself
			currentPath[0] = driveFilePathElem{
				ID:   fileID,
				Name: fileInfos.Name,
			}
			// add expanded parents
			for parentPathElemIndex, parentPathElem := range parentPath {
				currentPath[parentPathElemIndex+1] = parentPathElem
			}
			// save this new builded path with parents for return
			buildedPaths[parentIndex] = currentPath
		}
	}
	// All parents paths explored
	return
}
