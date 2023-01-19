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

package log

import (
	"os"
	"testing"
	"time"
)

func TestLog(t *testing.T) {
	Dir = "/tmp/log.test"
	os.RemoveAll(Dir)
	Duration = 1 * time.Second
	Age = 4 * time.Second
	if err := Init("tlog"); err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(10 * time.Second)
		logger.quit = true
	}()
	tick := time.Tick(time.Second / 4)
	for i := 0; ; i++ {
		select {
		case <-tick:
			Infof("message %d", i)
			continue
		case <-logger.done:
		}
		break
	}
	// We should look in Dir to see if we have the right
	// number of files.
}
