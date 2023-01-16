package main

import (
	"bytes"
	"fmt"
	"io"
	"sync/atomic"
	"time"

	"github.com/pborman/pty/log"
	"github.com/pborman/pty/mutex"
)

type MessengerWriter struct {
	mu *mutex.Mutex
	w  io.Writer
}

func NewMessengerWriter(w io.Writer) *MessengerWriter {
	name, fd := getFD(w)
	log.Infof("New Messenger Writer %s %d", name, fd)
	checkStdin()
	m := &MessengerWriter{
		mu: mutex.New("NewMessengerWriter"),
		w:  w,
	}
	checkStdin()
	m.mu.SetWait(time.Second * 15)
	checkStdin()
	return m
}

func (m *MessengerWriter) Close() error {
	checkStdin()
	return checkClose(m.w)
}

func (m *MessengerWriter) Write(buf []byte) (int, error) {
	log.Infof("Messenger write %d", len(buf))
	checkStdin()
	x := bytes.IndexByte(buf, 0)
	checkStdin()
	cnt := 0
	checkStdin()
	for x >= 0 {
		checkStdin()
		n, err := m.w.Write(buf[:x+1])
		if err != nil {
			log.Infof("%v", err)
		}
		checkStdin()
		cnt += n
		if err != nil {
			checkStdin()
			return cnt, err
		}
		cnt-- // don't count the double send of NUL
		checkStdin()
		buf = buf[x:] // repeating the NUL
		checkStdin()
		x = bytes.IndexByte(buf[1:], 0)
		checkStdin()
		if x < 0 {
			checkStdin()
			break
		}
		checkStdin()
		x++ // we indexed from buf+1
	}
	checkStdin()
	n, err := m.w.Write(buf)
	if err != nil {
		log.Infof("%v", err)
	}
	checkStdin()
	cnt += n
	checkStdin()
	return cnt, err
}

var oneK = 1024 // so tests can change it

func (m *MessengerWriter) Sendf(kind messageKind, format string, v ...interface{}) (int, error) {
	var buf bytes.Buffer
	checkStdin()
	fmt.Fprintf(&buf, format, v...)
	checkStdin()
	return m.Send(kind, buf.Bytes())
}

var ecnt int32

func (m *MessengerWriter) Send(kind messageKind, buf []byte) (int, error) {
	checkStdin()
	me := atomic.AddInt32(&ecnt, 1)
	checkStdin()
	log.Infof("(%d) Enter Send", me)
	checkStdin()
	defer log.Infof("(%d) Exit Send", me)
	defer checkStdin()
	if kind == 0 {
		checkStdin()
		return m.Write(buf)
	}
	defer m.mu.Lock("Send")()
	checkStdin()

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
	checkStdin()

	n := copy(msg[6:], buf)
	checkStdin()
	w, err := m.w.Write(msg[:n+6])
	if err != nil {
		log.Infof("%v", err)
	}
	checkStdin()
	w -= 6
	switch {
	case w <= 0:
		checkStdin()
		return 0, err
	case w < n:
		checkStdin()
		return w, err
	case len(buf) > n:
		checkStdin()
		w, err = m.w.Write(buf[n:])
		if err != nil {
			log.Infof("%v", err)
		}
		checkStdin()
		n += w
	}
	checkStdin()
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
	name, fd := getFD(r)
	log.Infof("New Messenger Reader %s %d", name, fd)
	return &MessengerReader{
		mu:       mutex.New("NewMessengerReader"),
		r:        r,
		callback: handle,
		message:  make([]byte, 1024),
	}
}

var sem int32

func (m *MessengerReader) Read(buf []byte) (cnt int, err error) {
	log.Infof("%d in Read(%d)\r\n", atomic.AddInt32(&sem, 1), len(buf))
	defer atomic.AddInt32(&sem, -1)
	checkStdin()
	unlock := m.mu.Lock("messangerReader")
	defer func() {
		checkStdin()
		unlock()
		checkStdin()
	}()
	checkStdin()
	for {
		checkStdin()
		if len(buf) == 0 {
			checkStdin()
			return cnt, nil
		}

		checkStdin()
		// We need at least one byte!
		if !m.fill(1) {
			checkStdin()
			return cnt, m.error
		}

		checkStdin()
		if m.message[m.mh] != 0 {
			checkStdin()
			// Normal data, not the start of a message
			mt := m.mt
			checkStdin()
			if mt-m.mh > len(buf) {
				checkStdin()
				mt = m.mh + len(buf)
				checkStdin()
			}
			checkStdin()
			checkStdin()
			x := bytes.IndexByte(m.message[m.mh:mt], 0)
			checkStdin()
			checkStdin()
			if x < 0 {
				checkStdin()
				n := copy(buf, m.message[m.mh:m.mt])
				checkStdin()
				m.mh += n
				checkStdin()
				cnt += n
				checkStdin()
				checkStdin()
				checkStdin()
				return cnt, nil
			}
			checkStdin()
			copy(buf, m.message[m.mh:m.mh+x])
			checkStdin()
			buf = buf[x:]
			checkStdin()
			m.mh += x
			checkStdin()
			cnt += x
			checkStdin()
		}
		checkStdin()

		// m.message[m.mh] is a NUL at this point
		log.Errorf("Filling 2 bytes")
		checkStdin()
		if !m.fill(2) {
			checkStdin()
			return cnt, m.error
		}
		// If we have two NULs in a row then
		// copy one of them into the buffer
		// and continue processing as regular
		// input.
		checkStdin()
		if m.message[m.mh+1] == 0 {
			checkStdin()
			buf[0] = 0
			checkStdin()
			buf = buf[1:]
			checkStdin()
			cnt++
			checkStdin()
			m.mh += 2
			checkStdin()
			continue
		}

		// If we are starting a message and we
		// have partially filled the buffer,
		// return what we have.  The message
		// will be read on the next read.
		if cnt > 0 {
			checkStdin()
			return cnt, nil
		}

		checkStdin()
		// We now have a message.
		log.Errorf("Filling 6 bytes")
		checkStdin()
		if !m.fill(6) {
			checkStdin()
			return cnt, m.error
		}
		checkStdin()
		kind := messageKind(m.message[m.mh+1])
		checkStdin()
		count := (int(m.message[m.mh+2]) << 24) |
			(int(m.message[m.mh+3]) << 16) |
			(int(m.message[m.mh+4]) << 8) |
			(int(m.message[m.mh+5]) << 0)
		checkStdin()

		m.mh += 6 // skip past NUL, kind, and count
		checkStdin()

		log.Errorf("Filling %d bytes", count)
		checkStdin()
		if !m.fill(count) {
			checkStdin()
			return 0, m.error
		}
		checkStdin()
		if m.callback != nil {
			checkStdin()
			m.callback(kind, m.message[m.mh:m.mh+count])
			checkStdin()
		}
		checkStdin()
		m.mh += count
		checkStdin()
	}
}

// fill tries to fill our buffer with count available bytes.
// fill returns false if an error is encountered.
func (m *MessengerReader) fill(count int) bool {
	if count == 0 {
		return true
	}
	checkStdin()
	log.Infof("fill(%d) %d > %d", count, m.mh, m.mt)
	checkStdin()
	if m.mh >= m.mt {
		checkStdin()
		m.mh, m.mt = 0, 0
		checkStdin()
		if m.error != nil {
			checkStdin()
			return false
		}
	}
	for m.error == nil && m.mt-m.mh < count {
		checkStdin()
		if count > cap(m.message) {
			checkStdin()
			// We can't fit in the message buffer, so
			// reallocate, rounding up to a 4K buffer size.
			nm := make([]byte, (count+0x1000)&0xfff)
			m.mt = copy(nm, m.message[:m.mt-m.mh])
			m.mh = 0
			m.message = nm
			checkStdin()
		} else if m.mh+count > cap(m.message) {
			checkStdin()
			m.mt = copy(m.message, m.message[m.mh:m.mt])
			m.mh = 0
			checkStdin()
		}
		var n int
		checkStdin()
		n, m.error = m.r.Read(m.message[m.mt:])
		// fmt.Printf("Read(%d:%d): %.64q%v\r\n", n, len(m.message[m.mt:]), m.message[m.mt:], m.error)
		checkStdin()
		if m.error != nil {
			log.Infof("%v", m.error)
		}
		checkStdin()
		m.mt += n
		checkStdin()
		if n == 0 && m.error == nil {
			checkStdin()
			m.error = io.EOF
		}
	}
	checkStdin()
	return m.mt-m.mh >= count
}
