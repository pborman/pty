package main

import (
	"bytes"
	"io/ioutil"
	"testing"
)

func TestMessengerWriter(t *testing.T) {
	oneK = 8

	bigbytes := make([]byte, 0x01020304)
	for i := range bigbytes {
		bigbytes[i] = 'a' + byte(i%26)
	}
	bigstring := string(bigbytes)
	_ = bigstring

	type mbuf struct {
		kind int
		msg  string
	}

	defer func() { oneK = 1024 }()
	for _, tt := range []struct {
		name     string
		messages []mbuf
		want     string
		text     string
	}{
		{
			name: "zero bug",
			messages: []mbuf{
				{msg: "\000\r\n"},
				{msg: "\000\r\n"},
				{msg: "\000\r\n"},
			},
			want: "\000\000\r\n\000\000\r\n\000\000\r\n",
			text: "\000\r\n\000\r\n\000\r\n",
		},
		/*
			{
				name: "abc",
				messages: []mbuf{
					{msg: "abc"},
				},
				want: "abc",
				text: "abc",
			},
			{
				name: "42-abc",
				messages: []mbuf{
					{kind: 42, msg: "abc"},
				},
				want: string([]byte{0, 42, 0, 0, 0, 3}) + "abc",
			},
			{
				name: "42-abcdefgh",
				messages: []mbuf{
					{kind: 42, msg: "abcdefgh"},
				},
				want: string([]byte{0, 42, 0, 0, 0, 8}) + "abcdefgh",
			},
			{
				name: "42-abcxyz",
				messages: []mbuf{
					{kind: 42, msg: "abcdefghijklmnopqrstuvwxyz"},
				},
				want: string([]byte{0, 42, 0, 0, 0, 26}) + "abcdefghijklmnopqrstuvwxyz",
			},
			{
				name: "42-big",
				messages: []mbuf{
					{kind: 5, msg: bigstring},
				},
				want: string([]byte{0, 5, 1, 2, 3, 4}) + bigstring,
			},
			{
				name: "big",
				messages: []mbuf{
					{kind: 1, msg: "abc"},
					{msg: bigstring},
					{kind: 1, msg: "xyz"},
				},
				want: string([]byte{0, 1, 0, 0, 0, 3}) + "abc" +
					bigstring +
					string([]byte{0, 1, 0, 0, 0, 3}) + "xyz",
				text: bigstring,
			},
			{
				name: "multi-message",
				messages: []mbuf{
					{msg: "abc"},
					{kind: 1, msg: ""},
					{msg: "def"},
					{kind: 2, msg: "12345678"},
					{msg: "ghi"},
				},
				want: "abc" +
					string([]byte{0, 1, 0, 0, 0, 0}) +
					"def" +
					string([]byte{0, 2, 0, 0, 0, 8}) + "12345678" +
					"ghi",
				text: "abc" + "def" + "ghi",
			},
		*/
	} {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			mw := NewMessengerWriter(&buf)
			for i, mb := range tt.messages {
				n, err := mw.Send(mb.kind, []byte(mb.msg))
				if n != len(mb.msg) {
					t.Errorf("%d:%.16s: Write returned %d, want %d", i, mb.msg, n, len(mb.msg))
				}
				if err != nil {
					t.Errorf("%d:%.16s: Write returned unexpected error %v", i, mb.msg, err)
				}
			}
			got := buf.String()
			if got != tt.want {
				t.Errorf("got raw/want raw:\n%.32q\n%.32q", got, tt.want)
			}
			mr := NewMessengerReader(&buf, func(kind int, data []byte) {})
			data, err := ioutil.ReadAll(mr)
			if err != nil {
				t.Errorf("read error:%v", err)
			}
			got = string(data)
			if got != tt.text {
				t.Errorf("got/want:\n%.32q\n%.32q", got, tt.text)
			}
		})
	}
}
