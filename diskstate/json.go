package diskstate

import (
	"encoding/json"
	"fmt"
	"os"
)

func SaveJSON(file string, input interface{}, withIndent bool) (err error) {
	// Open file
	fd, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0640)
	if err != nil {
		return fmt.Errorf("failed to open '%s' file for writing: %w", file, err)
	}
	defer func() {
		if errClose := fd.Close(); errClose != nil {
			if err == nil {
				err = errClose
			} else {
				err = fmt.Errorf("2 errors encountered: %w | %s", err, errClose)
			}
		}
	}()
	// Serialize data
	encoder := json.NewEncoder(fd)
	if withIndent {
		encoder.SetIndent("", "    ")
	}
	if err = encoder.Encode(input); err != nil {
		return fmt.Errorf("failed to encode the index as JSON: %w", err)
	}
	return
}

func LoadJSON(file string, output interface{}) (err error) {
	// Open file
	fd, err := os.Open(file)
	if err != nil {
		return fmt.Errorf("failed to open '%s' file for reading: %w", file, err)
	}
	defer func() {
		if errClose := fd.Close(); errClose != nil {
			if err == nil {
				err = errClose
			} else {
				err = fmt.Errorf("2 errors encountered: %w | %s", err, errClose)
			}
		}
	}()
	// Deserialize data
	return json.NewDecoder(fd).Decode(output)
}
