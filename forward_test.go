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
	"net"
	"testing"
)

func TestSetForwarderMissing(t *testing.T) {
	if err := SetForwarder("no-such-socket", "remote"); err == nil {
		t.Error("SetForwarder of missing name succeeded")
	}
}

func TestNewForwarderAndSet(t *testing.T) {
	defer withTempHome(t)()
	if err := NewForwarder("testfwd", "fwdsock"); err != nil {
		t.Fatal(err)
	}
	if err := SetForwarder("testfwd", "fwdsock"); err != nil {
		t.Fatal(err)
	}
	if err := SetForwarder("testfwd", "other"); err != nil {
		t.Fatal(err)
	}
}

func TestForwarderSessionError(t *testing.T) {
	defer withTempHome(t)()
	f := &forwarder{}
	remote := MakeSession("no-listen", "")
	defer remote.Remove()
	if err := remote.SetAddr("not-an-addr"); err != nil {
		t.Fatal(err)
	}
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	if err := f.session(a, remote); err == nil {
		t.Error("session to a session with a bad addr should fail")
	}
}
