package econn

import (
	"io"
	"testing"
	"time"
)

func TestConnNetMethods(t *testing.T) {
	client, server := encryptedPair(t, []byte("addr-secret"))

	if client.LocalAddr() == nil {
		t.Error("client LocalAddr is nil")
	}
	if client.RemoteAddr() == nil {
		t.Error("client RemoteAddr is nil")
	}
	if server.LocalAddr() == nil {
		t.Error("server LocalAddr is nil")
	}
	if server.RemoteAddr() == nil {
		t.Error("server RemoteAddr is nil")
	}

	deadline := time.Now().Add(time.Second)
	if err := client.SetDeadline(deadline); err != nil {
		t.Errorf("SetDeadline: %v", err)
	}
	if err := client.SetReadDeadline(deadline); err != nil {
		t.Errorf("SetReadDeadline: %v", err)
	}
	if err := client.SetWriteDeadline(deadline); err != nil {
		t.Errorf("SetWriteDeadline: %v", err)
	}

	go io.Copy(io.Discard, server)
	if _, err := client.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestWriteEmpty(t *testing.T) {
	client, server := encryptedPair(t, []byte("empty"))
	go io.Copy(io.Discard, server)
	n, err := client.Write(nil)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("Write(nil) = %d, want 0", n)
	}
}

func TestDeriveKeyStable(t *testing.T) {
	a := deriveKey([]byte("s"), "label")
	b := deriveKey([]byte("s"), "label")
	c := deriveKey([]byte("s"), "other")
	if len(a) != 32 {
		t.Fatalf("key len %d, want 32", len(a))
	}
	if string(a) != string(b) {
		t.Error("deriveKey is not stable")
	}
	if string(a) == string(c) {
		t.Error("different labels produced the same key")
	}
}
