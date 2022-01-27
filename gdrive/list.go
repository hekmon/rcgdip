package gdrive

import (
	"fmt"
)

func (c *Controller) TestList() (err error) {
	r, err := c.driveClient.Files.List().PageSize(10).
		Fields("nextPageToken, files(id, name)").Do()
	if err != nil {
		return fmt.Errorf("unable to retrieve files: %w", err)
	}
	fmt.Println("Files:")
	if len(r.Files) == 0 {
		fmt.Println("No files found.")
	} else {
		for _, i := range r.Files {
			fmt.Printf("%s (%s)\n", i.Name, i.Id)
		}
	}
	return
}
