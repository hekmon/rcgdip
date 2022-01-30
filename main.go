package main

import (
	"context"

	"github.com/hekmon/rcgdip/gdrivewatcher"
	"github.com/hekmon/rcgdip/gdrivewatcher/rcsnooper"
)

func main() {
	// Initialize GDrive controller
	gd, err := gdrivewatcher.New(context.TODO(), gdrivewatcher.Config{
		RClone: rcsnooper.Config{
			RCloneConfigPath: devrcloneconfigpath,
			DriveBackendName: devdrivebackendname,
			CryptBackendName: devcryptbackendname,
		},
	})
	if err != nil {
		panic(err)
	}
	if err = gd.FakeRun(); err != nil {
		panic(err)
	}
}
