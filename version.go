//   Copyright 2026 Paul Borman
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
	"strconv"
	"sync"
)

// The protocol version lets a client and a server built at different times
// find out what the other understands.  A server runs for weeks, so a newly
// built client talking to an older server is the normal case.
//
// On every connection the server first sends a versionMessage holding its
// version, before anything else.  A client replies with its own version, but
// only after it has received the server's, so a server that predates
// versions never sees a kind it does not know.  A peer that never sends a
// version is version 0.  A client knows the server is version 0 when
// startMessage arrives with no version before it.
//
// The payload of a versionMessage is the version in decimal.  Anything after
// the digits is ignored, so fields can be added later, and a payload that
// does not start with a number is version 0.
//
// protocolVersion must be bumped whenever a message kind is added or the
// meaning of one changes, and a constant naming the first version with the
// change added below it, for code that needs to know whether the peer has it.
const protocolVersion = 1

// The first protocol version with each feature.  Supports(versionX) is true
// if the peer has feature X.
const (
	// versionClear: the server handles clearMessage.  (Servers built
	// between 2026-11-25 and version 1 also handled it, but cannot be
	// told apart from older ones, so they are not sent it.)
	versionClear = 1
)

// versionPayload is the payload of our versionMessage.
func versionPayload() []byte {
	return []byte(fmt.Sprintf("%d", protocolVersion))
}

// parseVersion returns the version in msg, the payload of a versionMessage.
func parseVersion(msg []byte) int {
	n := 0
	for n < len(msg) && msg[n] >= '0' && msg[n] <= '9' {
		n++
	}
	v, err := strconv.Atoi(string(msg[:n]))
	if err != nil {
		return 0
	}
	return v
}

// A peerVersion is what one side of a connection knows of the other's
// protocol version.  It is safe for concurrent use.  Its lock is taken last,
// with nothing called while it is held, so it may be used with any other
// lock held.
type peerVersion struct {
	mu      sync.Mutex
	version int
	known   bool
}

// set records v as the peer's version.  It returns true the first time the
// version becomes known.
func (p *peerVersion) set(v int) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	first := !p.known
	p.version = v
	p.known = true
	return first
}

// setDefault records the peer as version 0 unless its version is already
// known.
func (p *peerVersion) setDefault() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.known = true
}

// Version returns the peer's version and whether it is known yet.  Until it
// is known the version is 0.
func (p *peerVersion) Version() (int, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.version, p.known
}

// Supports reports whether the peer is known to have protocol version v or
// later.  It is false while the peer's version is not yet known.
//
// The level of a connection is the lower of the two ends' versions.  Code
// that asks about a feature is itself at that feature's version or later, so
// asking whether the peer supports it is asking whether the connection does.
// The level belongs to the connection: a server can have clients at
// different levels attached at once, and asks each client's peerVersion
// separately.
func (p *peerVersion) Supports(v int) bool {
	version, known := p.Version()
	return known && version >= v
}
