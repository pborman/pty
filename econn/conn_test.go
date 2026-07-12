package econn

import (
	"bytes"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

// loopbackPair returns a client/server pair of raw TCP connections over the
// loopback interface.
func loopbackPair(t *testing.T) (client, server net.Conn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	var wg sync.WaitGroup
	var srvErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		server, srvErr = ln.Accept()
	}()

	client, err = net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	if srvErr != nil {
		t.Fatal(srvErr)
	}
	t.Cleanup(func() {
		client.Close()
		server.Close()
	})
	return client, server
}

// handshakePair runs Client and Server concurrently over the given raw
// connections and returns both results.
func handshakePair(rawClient, rawServer net.Conn, clientSecret, serverSecret []byte) (client, server *Conn, clientErr, serverErr error) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		server, serverErr = Server(rawServer, serverSecret)
	}()
	client, clientErr = Client(rawClient, clientSecret)
	wg.Wait()
	return client, server, clientErr, serverErr
}

func encryptedPair(t *testing.T, secret []byte) (client, server *Conn) {
	t.Helper()
	rawClient, rawServer := loopbackPair(t)
	client, server, clientErr, serverErr := handshakePair(rawClient, rawServer, secret, secret)
	if clientErr != nil {
		t.Fatalf("Client: %v", clientErr)
	}
	if serverErr != nil {
		t.Fatalf("Server: %v", serverErr)
	}
	return client, server
}

func TestRoundTrip(t *testing.T) {
	client, server := encryptedPair(t, []byte("hunter2"))

	msg := []byte("hello from client")
	if _, err := client.Write(msg); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(msg))
	if _, err := io.ReadFull(server, got); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, msg) {
		t.Errorf("server read %q, want %q", got, msg)
	}

	reply := []byte("hello from server")
	if _, err := server.Write(reply); err != nil {
		t.Fatal(err)
	}
	got = make([]byte, len(reply))
	if _, err := io.ReadFull(client, got); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, reply) {
		t.Errorf("client read %q, want %q", got, reply)
	}
}

func TestManySmallWrites(t *testing.T) {
	client, server := encryptedPair(t, []byte("secret"))

	var want bytes.Buffer
	go func() {
		for i := 0; i < 100; i++ {
			client.Write([]byte{byte(i)})
		}
		client.Close()
	}()
	for i := 0; i < 100; i++ {
		want.WriteByte(byte(i))
	}

	got, err := io.ReadAll(server)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want.Bytes()) {
		t.Errorf("got %x, want %x", got, want.Bytes())
	}
}

func TestLargeTransfer(t *testing.T) {
	client, server := encryptedPair(t, []byte("secret"))

	payload := make([]byte, 1<<20)
	if _, err := rand.Read(payload); err != nil {
		t.Fatal(err)
	}

	go func() {
		client.Write(payload)
		client.Close()
	}()

	got, err := io.ReadAll(server)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Error("large transfer corrupted")
	}
}

func TestBidirectionalConcurrent(t *testing.T) {
	client, server := encryptedPair(t, []byte("secret"))

	c2s := make([]byte, 1<<18)
	s2c := make([]byte, 1<<18)
	rand.Read(c2s)
	rand.Read(s2c)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		client.Write(c2s)
		if cw, ok := client.conn.(*net.TCPConn); ok {
			cw.CloseWrite()
		}
	}()
	go func() {
		defer wg.Done()
		server.Write(s2c)
		if sw, ok := server.conn.(*net.TCPConn); ok {
			sw.CloseWrite()
		}
	}()

	var gotC2S, gotS2C []byte
	var errC2S, errS2C error
	wg.Add(2)
	go func() {
		defer wg.Done()
		gotC2S, errC2S = io.ReadAll(server)
	}()
	go func() {
		defer wg.Done()
		gotS2C, errS2C = io.ReadAll(client)
	}()
	wg.Wait()

	if errC2S != nil || !bytes.Equal(gotC2S, c2s) {
		t.Errorf("client->server: err=%v equal=%v", errC2S, bytes.Equal(gotC2S, c2s))
	}
	if errS2C != nil || !bytes.Equal(gotS2C, s2c) {
		t.Errorf("server->client: err=%v equal=%v", errS2C, bytes.Equal(gotS2C, s2c))
	}
}

// snoopConn records everything written through it.
type snoopConn struct {
	net.Conn
	mu      sync.Mutex
	written bytes.Buffer
}

func (s *snoopConn) Write(b []byte) (int, error) {
	s.mu.Lock()
	s.written.Write(b)
	s.mu.Unlock()
	return s.Conn.Write(b)
}

// TestWireIsEncrypted checks that neither the challenge exchange nor the
// application data appears on the raw connection in the clear.
func TestWireIsEncrypted(t *testing.T) {
	rawClient, rawServer := loopbackPair(t)
	snoop := &snoopConn{Conn: rawClient}

	client, server, clientErr, serverErr := handshakePair(snoop, rawServer, []byte("secret"), []byte("secret"))
	if clientErr != nil || serverErr != nil {
		t.Fatalf("handshake: client=%v server=%v", clientErr, serverErr)
	}
	go io.Copy(io.Discard, server)

	msg := []byte("this text must not appear on the wire in the clear")
	if _, err := client.Write(msg); err != nil {
		t.Fatal(err)
	}

	snoop.mu.Lock()
	wire := snoop.written.Bytes()
	snoop.mu.Unlock()
	if bytes.Contains(wire, msg) {
		t.Error("plaintext found on the wire")
	}
}

// TestWrongSecret checks that mismatched secrets make the handshake fail on
// both sides.
func TestWrongSecret(t *testing.T) {
	rawClient, rawServer := loopbackPair(t)
	client, server, clientErr, serverErr := handshakePair(rawClient, rawServer, []byte("right secret"), []byte("wrong secret"))

	if clientErr == nil || serverErr == nil {
		t.Fatalf("handshake succeeded with mismatched secrets: client=%v server=%v", clientErr, serverErr)
	}
	if client != nil || server != nil {
		t.Error("got non-nil Conn from failed handshake")
	}
	// At least one side must reach the comparison and report the mismatch
	// (the other may see the conn close under it first).
	if !errors.Is(clientErr, ErrSecretMismatch) && !errors.Is(serverErr, ErrSecretMismatch) {
		t.Errorf("neither side reported ErrSecretMismatch: client=%v server=%v", clientErr, serverErr)
	}
}

// TestMismatchedRoles checks that two Clients with the same secret fail the
// handshake, since each direction has its own key.
func TestMismatchedRoles(t *testing.T) {
	rawA, rawB := loopbackPair(t)

	var wg sync.WaitGroup
	var b *Conn
	var errB error
	wg.Add(1)
	go func() {
		defer wg.Done()
		b, errB = Client(rawB, []byte("secret"))
	}()
	a, errA := Client(rawA, []byte("secret"))
	wg.Wait()

	if errA == nil || errB == nil {
		t.Fatalf("handshake succeeded between two Clients: a=%v b=%v", errA, errB)
	}
	if a != nil || b != nil {
		t.Error("got non-nil Conn from failed handshake")
	}
}

// TestWriteDoesNotModifyCaller checks that Write leaves the caller's buffer
// untouched.
func TestWriteDoesNotModifyCaller(t *testing.T) {
	client, server := encryptedPair(t, []byte("secret"))
	go io.Copy(io.Discard, server)

	msg := []byte("do not touch")
	orig := bytes.Clone(msg)
	if _, err := client.Write(msg); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(msg, orig) {
		t.Error("Write modified the caller's buffer")
	}
}

// TestHandshakeTimeout checks that a constructor gives up with ErrTimeout
// when the peer never handshakes, and closes the underlying conn.
func TestHandshakeTimeout(t *testing.T) {
	oldTimeout := Timeout
	Timeout = 50 * time.Millisecond
	defer func() { Timeout = oldTimeout }()

	rawClient, _ := loopbackPair(t) // server side never handshakes
	start := time.Now()
	conn, err := Client(rawClient, []byte("secret"))
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("got err %v, want ErrTimeout", err)
	}
	if conn != nil {
		t.Error("got non-nil Conn from timed-out handshake")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("timeout took %v with Timeout=50ms", elapsed)
	}
	if _, err := rawClient.Write([]byte("x")); err == nil {
		t.Error("underlying conn still open after timeout")
	}
}

// TestEOF checks that closing one side surfaces EOF on the other.
func TestEOF(t *testing.T) {
	client, server := encryptedPair(t, []byte("secret"))

	if _, err := client.Write([]byte("bye")); err != nil {
		t.Fatal(err)
	}
	client.Close()

	got, err := io.ReadAll(server)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "bye" {
		t.Errorf("read %q, want %q", got, "bye")
	}
}
