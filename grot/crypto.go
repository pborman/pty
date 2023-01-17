package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"os"

	"golang.org/x/crypto/nacl/box"
)

func EncryptedDial(net, addr string) (*Channel, error) {
	switch net {
	case "unix":
		return EncryptedUnixDial(addr)
	case "tcp":
		return encryptedDial(net, addr)
	default:
		return nil, fmt.Errorf("unsupported network: %s", net)
	}
}

func EncryptedServer(net, addr string, handle func(*Channel, error)) error {
	switch net {
	case "unix":
		return EncryptedUnixServer(addr, handle)
	case "tcp":
		return encryptedServer(net, addr, handle)
	default:
		return fmt.Errorf("unsupported network: %s", net)
	}
}

func EncryptedUnixDial(socket string) (*Channel, error) {
	client, err := net.DialUnix("unix", nil, &net.UnixAddr{
		Name: socket,
		Net:  "unix",
	})
	if err != nil {
		return nil, err
	}
	return NewEncryptedChannel(client, false)
}

func CheckSession(socket string) bool {
	client, err := net.DialUnix("unix", nil, &net.UnixAddr{
		Name: socket,
		Net:  "unix",
	})
	if err == nil {
		client.Close()
		return true
	}
	os.Remove(socket)
	return false
}

func EncryptedUnixServer(socket string, handle func(*Channel, error)) error {
	os.Remove(socket)
	conn, err := net.ListenUnix("unix", &net.UnixAddr{
		Name: socket,
		Net:  "unix",
	})
	if err != nil {
		return err
	}
	for {
		c, err := conn.Accept()
		if err != nil {
			return err
		}
		go func() {
			handle(NewEncryptedChannel(c, true))
			c.Close()
		}()
	}
}

func encryptedDial(network, addr string) (*Channel, error) {
	client, err := net.Dial(network, addr)
	if err != nil {
		return nil, err
	}
	return NewEncryptedChannel(client, false)
}

func encryptedServer(network, addr string, handle func(*Channel, error)) error {
	conn, err := net.Listen(network, addr)
	if err != nil {
		return err
	}
	for {
		c, err := conn.Accept()
		if err != nil {
			return err
		}
		go func() {
			handle(NewEncryptedChannel(c, true))
			c.Close()
		}()
	}
}

func sendShared(c io.ReadWriter) ([32]byte, error) {
	var key [32]byte

	private, err := sendKey(c)
	if err != nil {
		return key, err
	}
	public, err := getKey(c)
	if err != nil {
		return key, err
	}

	box.Precompute(&key, public, private)
	return key, nil
}

func getShared(c io.ReadWriter) ([32]byte, error) {
	var key [32]byte

	public, err := getKey(c)
	if err != nil {
		return key, err
	}
	private, err := sendKey(c)
	if err != nil {
		return key, err
	}

	box.Precompute(&key, public, private)
	return key, nil
}

const (
	publicTag = "PUBLIC: "
	nonceTag  = "NONCE:  "
)

func sendKey(conn io.ReadWriter) (*[32]byte, error) {
	public, private, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}

	pub := *public
	if err := writeFull(conn, pub[:]); err != nil {
		return nil, err
	}
	return private, nil
}

func getKey(conn io.ReadWriter) (*[32]byte, error) {
	var key [32]byte
	if err := readFull(conn, key[:]); err != nil {
		return nil, err
	}
	return &key, nil
}

func readFull(conn io.ReadWriter, msg []byte) error {
	_, err := conn.Write(msg)
	return err
}

func writeFull(conn io.ReadWriter, buf []byte) error {
	_, err := io.ReadFull(conn, buf[:])
	return err
}

type Channel struct {
	rw  io.ReadWriteCloser
	in  cipher.StreamReader
	out cipher.StreamWriter
}

// NewEncryptedChannel returns an encrypted io.ReadWriter over the provided
// channel.  The server should send true as the server and the client
// should send false.  The server flag simply determines which side
// starts sending first.
//
// The channel is encrypted using cipher feed back(CFB) with an AES-256 block
// cipher.  A shared key is computed using public key encryption.  Each
// direction starts with a unique random initialization vector (IV).
//
// The channel is not buffered.
//
// This protocol suffers from man-in-the-middle attacks.
func NewEncryptedChannel(c io.ReadWriteCloser, server bool) (*Channel, error) {
	var key [32]byte
	var err error
	if server {
		key, err = getShared(c)
	} else {
		key, err = sendShared(c)
	}
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	block1, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	iniv := make([]byte, block.BlockSize())
	outiv := make([]byte, block.BlockSize())

	if _, err := io.ReadFull(rand.Reader, outiv); err != nil {
		return nil, err
	}
	if server {
		if _, err := io.ReadFull(c, iniv); err != nil {
			return nil, err
		}
		if _, err := c.Write(outiv); err != nil {
			return nil, err
		}
	} else {
		if _, err := c.Write(outiv); err != nil {
			return nil, err
		}
		if _, err := io.ReadFull(c, iniv); err != nil {
			return nil, err
		}
	}

	return &Channel{
		rw: c,
		in: cipher.StreamReader{
			S: cipher.NewCFBDecrypter(block, iniv),
			R: c,
		},
		out: cipher.StreamWriter{
			S: cipher.NewCFBEncrypter(block1, outiv),
			W: c,
		},
	}, nil
}

func (c *Channel) Write(buf []byte) (int, error) { return c.out.Write(buf) }
func (c *Channel) Read(buf []byte) (int, error)  { return c.in.Read(buf) }
func (c *Channel) Close() error                  { return c.rw.Close() }
func (c *Channel) ReadFull(buf []byte) error {
	_, err := io.ReadFull(c, buf)
	return err
}
