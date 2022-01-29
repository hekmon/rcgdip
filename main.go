package main

import (
	"context"

	"github.com/hekmon/rcgdip/gdrivewatcher"
	"github.com/hekmon/rcgdip/rcsnooper"
)

func main() {
	// Initialize RClone snooper
	rc, err := rcsnooper.New(rcsnooper.Config{
		RCloneConfigPath: devrcloneconfigpath,
		DriveBackendName: devdrivebackendname,
		CryptBackendName: devcryptbackendname,
	})
	if err != nil {
		panic(err)
	}
	// Initialize GDrive controller
	gd, err := gdrivewatcher.New(context.TODO(), gdrivewatcher.Config{
		Drive:     rc.Drive,
		DecryptFx: rc.CryptDecode,
	})
	if err != nil {
		panic(err)
	}
	if err = gd.FakeRun(); err != nil {
		panic(err)
	}
}
