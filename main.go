package main

import (
	"context"

	"github.com/hekmon/rcgdip/gdrive"
	"github.com/hekmon/rcgdip/rcsnooper"
)

func main() {
	// Initialize RClone snooper
	rc, err := rcsnooper.New(rcsnooper.Config{
		RCloneConfigPath: devrcloneconfigpath,
		DriveBackendName: devdrivebackendname,
	})
	if err != nil {
		panic(err)
	}
	// Initialize GDrive controller
	gd, err := gdrive.New(context.TODO(), rc.Drive)
	if err != nil {
		panic(err)
	}

	gd.TestList()
}
