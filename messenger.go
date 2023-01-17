package main

import (
	"bytes"
	"fmt"
	"io"

	"github.com/pborman/pty/log"
	"github.com/pborman/pty/mutex"
)

type MessengerWriter struct {
	mu *mutex.Mutex
	w  io.Writer
}

func NewMessengerWriter(w io.Writer) *MessengerWriter {
	m := &MessengerWriter{
		mu: mutex.New("NewMessengerWriter"),
		w:  w,
	}
	return m
}

func (m *MessengerWriter) Close() error {
	return checkClose(m.w)
}

func (m *MessengerWriter) Write(buf []byte) (int, error) {
	x := bytes.IndexByte(buf, 0)
	cnt := 0
	for x >= 0 {
		n, err := m.w.Write(buf[:x+1])
		if err != nil {
			log.Infof("%v", err)
		}
		cnt += n
		if err != nil {
			return cnt, err
		}
		cnt--         // don't count the double send of NUL
		buf = buf[x:] // repeating the NUL
		x = bytes.IndexByte(buf[1:], 0)
		if x < 0 {
			break
		}
		x++ // we indexed from buf+1
	}
	n, err := m.w.Write(buf)
	if err != nil {
		log.Infof("%v", err)
	}
	cnt += n
	return cnt, err
}

var oneK = 1024 // so tests can change it

func (m *MessengerWriter) Sendf(kind messageKind, format string, v ...interface{}) (int, error) {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, format, v...)
	return m.Send(kind, buf.Bytes())
}

func (m *MessengerWriter) Send(kind messageKind, buf []byte) (int, error) {
	if kind == 0 {
		return m.Write(buf)
	}
	defer m.mu.Lock("Send")()

	// We copy up to 1k into the buffer than includes are header.
	// If buf is longer than that, we do a second write.  This is
	// as opposed doing a 6 byte write always followed by a second
	// write.
	msg := make([]byte, oneK)

	count := len(buf)
	msg[0] = 0
	msg[1] = byte(kind)
	msg[2] = byte(count >> 24)
	msg[3] = byte(count >> 16)
	msg[4] = byte(count >> 8)
	msg[5] = byte(count >> 0)

	n := copy(msg[6:], buf)
	w, err := m.w.Write(msg[:n+6])
	if err != nil {
		log.Infof("%v", err)
	}
	w -= 6
	switch {
	case w <= 0:
		return 0, err
	case w < n:
		return w, err
	case len(buf) > n:
		w, err = m.w.Write(buf[n:])
		if err != nil {
			log.Infof("%v", err)
		}
		n += w
	}
	return n, err
}

type MessengerReader struct {
	mu       *mutex.Mutex
	r        io.Reader
	callback func(code messageKind, msg []byte)
	mh, mt   int
	message  []byte
	error    error
}

func NewMessengerReader(r io.Reader, handle func(messageKind, []byte)) *MessengerReader {
	return &MessengerReader{
		mu:       mutex.New("NewMessengerReader"),
		r:        r,
		callback: handle,
		message:  make([]byte, 32768),
	}
}

func (m *MessengerReader) Read(buf []byte) (cnt int, err error) {
	defer m.mu.Lock("messangerReader")()
	for {
		if len(buf) == 0 {
			return cnt, nil
		}

		// We need at least one byte!
		if !m.fill(1) {
			return cnt, m.error
		}

		if m.message[m.mh] != 0 {
			// Normal data, not the start of a message
			mt := m.mt
			if mt-m.mh > len(buf) {
				mt = m.mh + len(buf)
			}
			x := bytes.IndexByte(m.message[m.mh:mt], 0)
			if x < 0 {
				n := copy(buf, m.message[m.mh:m.mt])
				m.mh += n
				cnt += n
				return cnt, nil
			}
			copy(buf, m.message[m.mh:m.mh+x])
			buf = buf[x:]
			m.mh += x
			cnt += x
		}

		// m.message[m.mh] is a NUL at this point
		if !m.fill(2) {
			return cnt, m.error
		}
		// If we have two NULs in a row then
		// copy one of them into the buffer
		// and continue processing as regular
		// input.
		if m.message[m.mh+1] == 0 {
			buf[0] = 0
			buf = buf[1:]
			cnt++
			m.mh += 2
			continue
		}

		// If we are starting a message and we
		// have partially filled the buffer,
		// return what we have.  The message
		// will be read on the next read.
		if cnt > 0 {
			return cnt, nil
		}

		// We now have a message.
		log.Errorf("Filling 6 bytes")
		if !m.fill(6) {
			return cnt, m.error
		}
		kind := messageKind(m.message[m.mh+1])
		count := (int(m.message[m.mh+2]) << 24) |
			(int(m.message[m.mh+3]) << 16) |
			(int(m.message[m.mh+4]) << 8) |
			(int(m.message[m.mh+5]) << 0)

		m.mh += 6 // skip past NUL, kind, and count

		log.Errorf("Filling %d bytes", count)
		if !m.fill(count) {
			return 0, m.error
		}
		if m.callback != nil {
			m.callback(kind, m.message[m.mh:m.mh+count])
		}
		m.mh += count
	}
}

// fill tries to fill our buffer with count available bytes.
// fill returns false if an error is encountered.
func (m *MessengerReader) fill(count int) bool {
	if count == 0 {
		return true
	}
	if m.mh >= m.mt {
		m.mh, m.mt = 0, 0
		if m.error != nil {
			return false
		}
	}
	for m.error == nil && m.mt-m.mh < count {
		if count > cap(m.message) {
			// We can't fit in the message buffer, so
			// reallocate, rounding up to a 4K buffer size.
			nm := make([]byte, (count+0x1000)&0xfff)
			m.mt = copy(nm, m.message[:m.mt-m.mh])
			m.mh = 0
			m.message = nm
		} else if m.mh+count > cap(m.message) {
			m.mt = copy(m.message, m.message[m.mh:m.mt])
			m.mh = 0
		}
		var n int
		n, m.error = m.r.Read(m.message[m.mt:])
		m.mt += n
		if n == 0 && m.error == nil {
			m.error = io.EOF
		}
	}
	return m.mt-m.mh >= count
}
