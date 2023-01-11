package main

import (
	"bytes"
	"testing"
)

func TestMessengerWriter(t *testing.T) {
	oneK = 8

	bigbytes := make([]byte, 0x01020304)
	for i := range bigbytes {
		bigbytes[i] = 'a' + byte(i % 26)
	}
	bigstring := string(bigbytes)

	defer func() { oneK = 1024 }()
	for _, tt := range []struct {
		kind    int
		message string
		want    string
	}{
		{
			kind:    0,
			message: "abc",
			want:    string([]byte{0, 0, 0, 3, 0}) + "abc",
		},
		{
			kind:    42,
			message: "abc",
			want:    string([]byte{0, 0, 0, 3, 42}) + "abc",
		},
		{
			kind:    42,
			message: "abcdefgh",
			want:    string([]byte{0, 0, 0, 8, 42}) + "abcdefgh",
		},
		{
			kind:    42,
			message: "abcdefghijklmnopqrstuvwxyz",
			want:    string([]byte{0, 0, 0, 26, 42}) + "abcdefghijklmnopqrstuvwxyz",
		},
		{
			kind:    5,
			message: bigstring,
			want:    string([]byte{1,2,3,4,5}) + bigstring,
		},
	} {
		var buf bytes.Buffer
		m := NewMessengerWriter(&buf)
		n, err := m.WriteMessage(tt.kind, []byte(tt.message))
		got := buf.String()
		if n != len(tt.message) {
			t.Errorf("%d:%.16s: Write returned %d, want %d", tt.kind, tt.message, n, len(tt.message))
		}
		if err != nil {
			t.Errorf("%d:%.16s: Write returned unexpected error %v", tt.kind, tt.message, err)
		}
		if got != tt.want {
			t.Errorf("%d:%.16s: got %.16q, want %.16q", tt.kind, tt.message, got, tt.want)
		}
	}
}
