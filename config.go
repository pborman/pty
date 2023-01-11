package main

import (
	"io/ioutil"
	"os"
	"path/filepath"

	yaml "gopkg.in/yaml.v2"
)

var config = struct {
	Forward []string
}{}

func ReadConfig() error {
	data, err := ioutil.ReadFile(filepath.Join(user.HomeDir, rcdir, "config.yaml"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return yaml.Unmarshal(data, &config)
}
