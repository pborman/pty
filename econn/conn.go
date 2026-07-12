// Package econn provides an encrypted net.Conn wrapper keyed by a shared
// secret. A *Conn is created with Client on one end of an existing net.Conn
// and Server on the other; all bytes written are encrypted with AES-256-CTR
// and decrypted on the far side.
//
// Each direction uses its own key, derived from the shared secret with
// HMAC-SHA256, and its own random IV. A side sends its 16-byte IV in front
// of its first encrypted write; the receiver consumes that IV before
// decrypting.
//
// The constructors perform a handshake that proves each peer holds the same
// secret: each side sends a random encrypted challenge, decrypts the peer's
// challenge, echoes it back encrypted, and verifies the echo of its own. A
// peer started with a different secret decrypts garbage at every step, so
// its echo cannot match and the constructor returns ErrSecretMismatch.
// Because the handshake is a round trip, Client and Server block until the
// other end runs or Timeout passes.  Timeout is a package global and should
// only be changed in package main prior to using the econn package.
//
// The stream is encrypted but not authenticated (no MAC), so an active
// attacker could flip bits in transit undetected. This matches its intended
// use on a loopback connection where the transport itself is not hostile.
package econn

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

const (
	ivSize        = aes.BlockSize
	challengeSize = 32
)

// Timeout is how long to wait for the initial handshake.
var Timeout = time.Minute * 5

// ErrTimeout is returned by Client and Server when the handshake
// does not complete within the Timeout.
var ErrTimeout = errors.New("econn: handshake timed out")

// ErrSecretMismatch is returned by Client and Server when the handshake
// shows the peer was started with a different shared secret.
var ErrSecretMismatch = errors.New("econn: handshake failed: bad secret")

// Conn is an encrypted connection over an underlying net.Conn. It implements
// net.Conn. As with any net.Conn, one concurrent Read and one concurrent
// Write are safe; multiple concurrent Reads (or Writes) are serialized.
type Conn struct {
	conn net.Conn

	writeMu     sync.Mutex
	writeStream cipher.Stream
	writeIV     []byte // sent before the first write, then nil
	writeBuf    []byte // scratch so callers' buffers are never modified

	readMu     sync.Mutex
	readBlock  cipher.Block
	readStream cipher.Stream // nil until the peer's IV has been read
}

var _ net.Conn = (*Conn)(nil)

// Client wraps c in an encrypted connection using the shared secret. The
// peer on the other end of c must be created with Server and the same
// secret. It blocks until the verification handshake with the peer
// completes; on any handshake error c is closed. It panics only if
// crypto/rand fails.
func Client(c net.Conn, secret []byte) (*Conn, error) {
	return newConn(c, secret, "econn client write", "econn server write")
}

// Server wraps c in an encrypted connection using the shared secret. The
// peer on the other end of c must be created with Client and the same
// secret. It blocks until the verification handshake with the peer
// completes; on any handshake error c is closed. It panics only if
// crypto/rand fails.
func Server(c net.Conn, secret []byte) (*Conn, error) {
	return newConn(c, secret, "econn server write", "econn client write")
}

func newConn(c net.Conn, secret []byte, writeLabel, readLabel string) (*Conn, error) {
	t := time.NewTimer(Timeout)
	defer t.Stop()
	ch := make(chan struct{})
	var ec *Conn
	var err error
	go func() {
		ec, err = newConn2(c, secret, writeLabel, readLabel)
		close(ch)
	}()
	select {
	case <-t.C:
		c.Close()
		return nil, ErrTimeout
	case <-ch:
		return ec, err
	}
}

func newConn2(c net.Conn, secret []byte, writeLabel, readLabel string) (*Conn, error) {
	writeBlock, err := aes.NewCipher(deriveKey(secret, writeLabel))
	if err != nil {
		panic("econn: " + err.Error())
	}
	readBlock, err := aes.NewCipher(deriveKey(secret, readLabel))
	if err != nil {
		panic("econn: " + err.Error())
	}
	iv := make([]byte, ivSize)
	if _, err := rand.Read(iv); err != nil {
		panic("econn: reading random IV: " + err.Error())
	}
	conn := &Conn{
		conn:        c,
		writeStream: cipher.NewCTR(writeBlock, iv),
		writeIV:     iv,
		readBlock:   readBlock,
	}
	if err := conn.handshake(); err != nil {
		c.Close()
		return nil, err
	}
	return conn, nil
}

// handshake proves both sides hold the same secret. Each side sends a random
// challenge, echoes back the peer's decrypted challenge, and verifies the
// echo of its own. It runs on the ordinary encrypted streams, so it also
// exchanges the IVs as a side effect of the first Write and Read.
func (c *Conn) handshake() error {
	challenge := make([]byte, challengeSize)
	if _, err := rand.Read(challenge); err != nil {
		panic("econn: reading random challenge: " + err.Error())
	}
	if _, err := c.Write(challenge); err != nil {
		return fmt.Errorf("econn: handshake send: %w", err)
	}
	peer := make([]byte, challengeSize)
	if _, err := io.ReadFull(c, peer); err != nil {
		return fmt.Errorf("econn: handshake receive: %w", err)
	}
	if _, err := c.Write(peer); err != nil {
		return fmt.Errorf("econn: handshake echo send: %w", err)
	}
	echo := make([]byte, challengeSize)
	if _, err := io.ReadFull(c, echo); err != nil {
		return fmt.Errorf("econn: handshake echo receive: %w", err)
	}
	if subtle.ConstantTimeCompare(echo, challenge) != 1 {
		return ErrSecretMismatch
	}
	return nil
}

// deriveKey turns the shared secret into a 32-byte AES key bound to one
// direction, so client->server and server->client never share a key stream.
func deriveKey(secret []byte, label string) []byte {
	m := hmac.New(sha256.New, secret)
	m.Write([]byte(label))
	return m.Sum(nil)
}

// Read reads decrypted data from the connection. The first Read blocks until
// the peer's IV (sent with the peer's first write) has arrived.
func (c *Conn) Read(b []byte) (int, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()
	if c.readStream == nil {
		iv := make([]byte, ivSize)
		if _, err := io.ReadFull(c.conn, iv); err != nil {
			return 0, err
		}
		c.readStream = cipher.NewCTR(c.readBlock, iv)
	}
	n, err := c.conn.Read(b)
	if n > 0 {
		c.readStream.XORKeyStream(b[:n], b[:n])
	}
	return n, err
}

// Write encrypts b and writes it to the connection. The first Write also
// sends this side's IV. b itself is never modified.
func (c *Conn) Write(b []byte) (int, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	pre := len(c.writeIV)
	if cap(c.writeBuf) < pre+len(b) {
		c.writeBuf = make([]byte, pre+len(b))
	}
	buf := c.writeBuf[:pre+len(b)]
	copy(buf, c.writeIV)
	c.writeStream.XORKeyStream(buf[pre:], b)
	// The IV is consumed even if the write fails: the key stream has
	// advanced, so a retry could not resend the same prefix anyway.
	c.writeIV = nil

	n, err := c.conn.Write(buf)
	n -= pre
	if n < 0 {
		n = 0
	}
	return n, err
}

// Close closes the underlying connection.
func (c *Conn) Close() error { return c.conn.Close() }

// LocalAddr returns the local address of the underlying connection.
func (c *Conn) LocalAddr() net.Addr { return c.conn.LocalAddr() }

// RemoteAddr returns the remote address of the underlying connection.
func (c *Conn) RemoteAddr() net.Addr { return c.conn.RemoteAddr() }

// SetDeadline sets the read and write deadlines of the underlying connection.
func (c *Conn) SetDeadline(t time.Time) error { return c.conn.SetDeadline(t) }

// SetReadDeadline sets the read deadline of the underlying connection.
func (c *Conn) SetReadDeadline(t time.Time) error { return c.conn.SetReadDeadline(t) }

// SetWriteDeadline sets the write deadline of the underlying connection.
func (c *Conn) SetWriteDeadline(t time.Time) error { return c.conn.SetWriteDeadline(t) }
