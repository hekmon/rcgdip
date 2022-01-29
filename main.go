package main

import "fmt"

func main() {
	// // Initialize RClone snooper
	// rc, err := rcsnooper.New(rcsnooper.Config{
	// 	RCloneConfigPath: devrcloneconfigpath,
	// 	DriveBackendName: devdrivebackendname,
	// })
	// if err != nil {
	// 	panic(err)
	// }
	// // Initialize GDrive controller
	// gd, err := gdrivewatcher.New(context.TODO(), rc.Drive)
	// if err != nil {
	// 	panic(err)
	// }
	// if err = gd.FakeRun(); err != nil {
	// 	panic(err)
	// }

	rclone()

	// err := CryptInit("DMECrypt:", []string{"Films/Rip 2160p", "Animes"})
	// if err != nil {
	// 	panic(fmt.Sprintf("cryptinit: %v", err))
	// }

	err := DriveInit("DME")
	if err != nil {
		panic(fmt.Sprintf("driveinit: %v", err))
	}
}
