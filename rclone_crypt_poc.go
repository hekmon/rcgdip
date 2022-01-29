package main

import (
	"errors"
	"fmt"

	"github.com/rclone/rclone/backend/crypt"
	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/config"
	"github.com/rclone/rclone/fs/config/configfile"
)

func rclone() {

	err := config.SetConfigPath(devrcloneconfigpath)
	if err != nil {
		panic(fmt.Sprintf("setconfigpath: %v", err))
	}

	storageConfig := &configfile.Storage{}
	err = storageConfig.Load()
	if err != nil {
		panic(fmt.Sprintf("storageconfigload: %v", err))
	}

	config.SetData(storageConfig)

	err = CryptInit("DMECrypt:", []string{"Films/Rip 2160p", "Animes"})
	if err != nil {
		panic(fmt.Sprintf("cryptinit: %v", err))
	}
}

func CryptInit(path string, files []string) (err error) {
	fsInfo, _, _, config, err := fs.ConfigFs(path)
	if err != nil {
		return err
	}
	if fsInfo.Name != "crypt" {
		return errors.New("the remote needs to be of type \"crypt\"")
	}
	cipher, err := crypt.NewCipher(config)
	if err != nil {
		return err
	}
	return cryptEncode(cipher, files)
}

// cryptDecode returns the unencrypted file name
func cryptDecode(cipher *crypt.Cipher, files []string) error {

	output := ""

	for _, encryptedFileName := range files {
		fileName, err := cipher.DecryptFileName(encryptedFileName)
		if err != nil {
			output += fmt.Sprintln(encryptedFileName, "\t", "Failed to decrypt")
		} else {
			output += fmt.Sprintln(encryptedFileName, "\t", fileName)
		}
	}

	fmt.Printf(output)

	return nil
}

// cryptEncode returns the encrypted file name
func cryptEncode(cipher *crypt.Cipher, args []string) error {
	output := ""

	for _, fileName := range args {
		encryptedFileName := cipher.EncryptFileName(fileName)
		output += fmt.Sprintln(fileName, "\t", encryptedFileName)
	}

	fmt.Printf(output)

	return nil
}
