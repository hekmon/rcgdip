package main

import (
	"github.com/hekmon/rcgdip/rcsnooper"
)

func main() {
	rc, err := rcsnooper.New(rcsnooper.Config{
		RCloneConfigPath: devrcloneconfigpath,
		DriveBackendName: devdrivebackendname,
	})

	if err != nil {
		panic(err)
	}

	driveTest(rc.GetDriveInfos())

}
