package main

import (
	"io"
	"sync"
)

type MessengerWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func NewMessengerWriter(w io.Writer) *MessengerWriter {
	return &MessengerWriter{
		w: w,
	}
}

func (m *MessengerWriter) Write(buf []byte) (int, error) {
	return m.WriteMessage(0, buf)
}

var oneK = 1024 // so tests can change it

func (m *MessengerWriter) WriteMessage(kind int, buf []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// We copy up to 1k into the buffer than includes are header.
	// If buf is longer than that, we do a second write.  This is
	// as opposed doing a 5 byte write always followed by a second
	// write.
	msg := make([]byte, oneK)

	count := len(buf)
	msg[0] = byte(count >> 24)
	msg[1] = byte(count >> 16)
	msg[2] = byte(count >> 8)
	msg[3] = byte(count >> 0)
	msg[4] = byte(kind)

	n := copy(msg[5:], buf)
	w, err := m.w.Write(msg[:n+5])
	w -= 5
	switch {
	case w <= 0:
		return 0, err
	case w < n:
		return w, err
	case len(buf) > n:
		w, err = m.w.Write(buf[n:])
		n += w
	}
	return n, err
}

type MessengerReader struct {
	mu       sync.Mutex
	r        io.Reader
	callback func(code int, msg []byte)
	mh, mt   int
	message  []byte
	error    error
	left     int
}

func NewMessengerReader(r io.Reader, handle func(int, []byte)) *MessengerReader {
	return &MessengerReader{
		r:        r,
		callback: handle,
		message:  make([]byte, 32768),
	}
}

func (m *MessengerReader) Read(buf []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for {
		// If m.left > 0 then we are in a data message and
		// we can just copy/read data into buf.
		if m.left > 0 {
			if m.mh < m.mt {
				n := copy(buf, m.message[m.mh:m.mt])
				m.mh += n
				m.left -= n
				return n, nil
			}
			n := m.left
			if n > len(buf) {
				n = len(buf)
			}
			n, m.error = m.r.Read(buf[:n])
			m.left -= n
			return n, m.error
		}

		// We are expecting a new message.  Make sure we have
		// a full header.
		if !m.fill(5) {
			return 0, m.error
		}

		// decode the header
		count := (int(m.message[m.mh+0]) << 24) |
			(int(m.message[m.mh+1]) << 16) |
			(int(m.message[m.mh+2]) << 8) |
			(int(m.message[m.mh+3]) << 0)
		kind := int(m.message[m.mh+4])
		m.mh += 5

		if kind == 0 {
			// regular output, just set m.left to how much
			// there is and go back to the top.  It doesn't
			// matter if we have read the full message or not.
			m.left = count
			continue
		}

		// This is a non-output message.  Make sure we have
		// the entire message, then call the callback, and
		// loop for the next message.
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
// fill only issues a single Read.
// fill returns false if an error is encountered.
func (m *MessengerReader) fill(count int) bool {
	if m.mh >= m.mt {
		m.mh, m.mt = 0, 0
	}
	for m.mt-m.mh < count {
		if m.error != nil {
			return false
		}
		if count > cap(m.message) {
			nm := make([]byte, (count+0x800)&0x7ff)
			m.mt = copy(nm, m.message[:m.mt-m.mh])
			m.mh = 0
			m.message = nm
		} else if m.mh+count > cap(m.message) {
			m.mt = copy(m.message, m.message[m.mh:m.mt])
			m.mh = 0
		}
		n, err := m.r.Read(m.message[m.mt:])
		m.mt += n
		m.error = err
	}
	return true
}
