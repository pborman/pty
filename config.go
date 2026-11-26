//   Copyright 2023 Paul Borman
//
//   Licensed under the Apache License, Version 2.0 (the "License");
//   you may not use this file except in compliance with the License.
//   You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
//   Unless required by applicable law or agreed to in writing, software
//   distributed under the License is distributed on an "AS IS" BASIS,
//   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//   See the License for the specific language governing permissions and
//   limitations under the License.

package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	yaml "gopkg.in/yaml.v2"
)

var config = struct {
	Forward   []string
	Sequences []SequenceConfig // see sequences.go
	Shells    []string

	// CloseTimeout is how long the server waits for a client that is
	// being closed to take its queued output (see clientCloseTimeout).
	// Zero, or leaving it out, keeps the default.
	CloseTimeout time.Duration `yaml:"close_timeout"`
}{}

func ReadConfig() error {
	data, err := ioutil.ReadFile(filepath.Join(user.HomeDir, rcdir, "config.yaml"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return err
	}
	rules, err := compileSequences(append(append([]SequenceConfig{}, builtinSequences...), config.Sequences...))
	if err != nil {
		return err
	}
	seqRules = rules
	switch {
	case config.CloseTimeout < 0:
		return fmt.Errorf("close_timeout: %v is negative", config.CloseTimeout)
	case config.CloseTimeout > 0:
		clientCloseTimeout.Store(int64(config.CloseTimeout))
	}
	return nil
}
