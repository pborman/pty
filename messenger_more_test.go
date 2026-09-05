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
	"bytes"
	"io"
	"io/ioutil"
	"testing"
)

func TestMessengerRoundTrip(t *testing.T) {
	defer func() { oneK = 1024 }()
	oneK = 8

	type step struct {
		kind messageKind
		msg  string
	}
	for _, tt := range []struct {
		name  string
		steps []step
		raw   string
		text  string
	}{
		{
			name:  "plain",
			steps: []step{{msg: "abc"}},
			raw:   "abc",
			text:  "abc",
		},
		{
			name:  "typed",
			steps: []step{{kind: 42, msg: "abc"}},
			raw:   string([]byte{0, 42, 0, 0, 0, 3}) + "abc",
		},
		{
			name:  "typed-exact-onek",
			steps: []step{{kind: 42, msg: "abcdefgh"}},
			raw:   string([]byte{0, 42, 0, 0, 0, 8}) + "abcdefgh",
		},
		{
			name:  "typed-over-onek",
			steps: []step{{kind: 42, msg: "abcdefghijklmnopqrstuvwxyz"}},
			raw:   string([]byte{0, 42, 0, 0, 0, 26}) + "abcdefghijklmnopqrstuvwxyz",
		},
		{
			name: "multi",
			steps: []step{
				{msg: "abc"},
				{kind: 1, msg: ""},
				{msg: "def"},
				{kind: 2, msg: "12345678"},
				{msg: "ghi"},
			},
			raw: "abc" +
				string([]byte{0, 1, 0, 0, 0, 0}) +
				"def" +
				string([]byte{0, 2, 0, 0, 0, 8}) + "12345678" +
				"ghi",
			text: "abcdefghi",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			mw := NewMessengerWriter(&buf)
			for i, st := range tt.steps {
				n, err := mw.Send(st.kind, []byte(st.msg))
				if n != len(st.msg) {
					t.Errorf("step %d: Write returned %d, want %d", i, n, len(st.msg))
				}
				if err != nil {
					t.Errorf("step %d: %v", i, err)
				}
			}
			if buf.String() != tt.raw {
				t.Errorf("raw got %q want %q", buf.String(), tt.raw)
			}
			seen := []messageKind{}
			mr := NewMessengerReader(&buf, func(kind messageKind, data []byte) {
				seen = append(seen, kind)
			})
			data, err := ioutil.ReadAll(mr)
			if err != nil {
				t.Errorf("read: %v", err)
			}
			if string(data) != tt.text {
				t.Errorf("text got %q want %q", data, tt.text)
			}
		})
	}
}

func TestMessengerSendfAndClose(t *testing.T) {
	cb := &closeBuffer{}
	mw := NewMessengerWriter(cb)
	n, err := mw.Sendf(7, "hello %s", "world")
	if err != nil {
		t.Fatal(err)
	}
	if n != len("hello world") {
		t.Errorf("Sendf n = %d", n)
	}
	if err := mw.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestMessengerWriteError(t *testing.T) {
	mw := NewMessengerWriter(errWriter{err: io.ErrClosedPipe})
	if _, err := mw.Write([]byte("abc")); err == nil {
		t.Error("Write expected error")
	}
	if _, err := mw.Write([]byte("a\x00b")); err == nil {
		t.Error("Write with NUL expected error")
	}
	if _, err := mw.Send(3, []byte("xyz")); err == nil {
		t.Error("Send expected error")
	}
}

func TestMessengerReaderEmptyRead(t *testing.T) {
	mr := NewMessengerReader(bytes.NewReader(nil), nil)
	n, err := mr.Read(nil)
	if n != 0 || err != nil {
		t.Errorf("Read(nil) = %d, %v", n, err)
	}
}

func TestMessengerReaderCallbackAndPartial(t *testing.T) {
	var raw bytes.Buffer
	mw := NewMessengerWriter(&raw)
	mw.Write([]byte("ab"))
	mw.Send(5, []byte("MSG"))
	mw.Write([]byte("cd"))

	var kinds []messageKind
	var payloads []string
	mr := NewMessengerReader(&raw, func(kind messageKind, data []byte) {
		kinds = append(kinds, kind)
		payloads = append(payloads, string(data))
	})
	got, err := ioutil.ReadAll(mr)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "abcd" {
		t.Errorf("text = %q, want abcd", got)
	}
	if len(kinds) != 1 || kinds[0] != 5 || payloads[0] != "MSG" {
		t.Errorf("callback kinds=%v payloads=%q", kinds, payloads)
	}
}

func TestMessengerReaderLargeMessage(t *testing.T) {
	payload := bytes.Repeat([]byte("x"), 40000)
	var raw bytes.Buffer
	mw := NewMessengerWriter(&raw)
	if _, err := mw.Send(9, payload); err != nil {
		t.Fatal(err)
	}
	var got []byte
	mr := NewMessengerReader(&raw, func(kind messageKind, data []byte) {
		got = append([]byte(nil), data...)
	})
	if _, err := ioutil.ReadAll(mr); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		n := len(got)
		if n > len(payload) {
			n = len(payload)
		}
		diff := n
		for i := 0; i < n; i++ {
			if got[i] != payload[i] {
				diff = i
				break
			}
		}
		t.Errorf("large payload len=%d, want %d, first diff at %d (got[:8]=%q want[:8]=%q)",
			len(got), len(payload), diff, got[:min(8, len(got))], payload[:min(8, len(payload))])
	}
}

func TestMessengerReaderChunked(t *testing.T) {
	var raw bytes.Buffer
	mw := NewMessengerWriter(&raw)
	mw.Write([]byte("hello world"))
	mr := NewMessengerReader(&raw, nil)
	buf := make([]byte, 3)
	var out []byte
	for {
		n, err := mr.Read(buf)
		out = append(out, buf[:n]...)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			break
		}
	}
	if string(out) != "hello world" {
		t.Errorf("chunked = %q", out)
	}
}

func TestMessageKindString(t *testing.T) {
	if dataMessage.String() != "dataMessage" {
		t.Errorf("dataMessage.String = %q", dataMessage.String())
	}
	if messageKind(999).String() != "message-999" {
		t.Errorf("unknown kind = %q", messageKind(999).String())
	}
}
