package log

import (
	"os"
	"testing"
	"time"
)

func TestLog(t *testing.T) {
	Dir = "/tmp/log.test"
	os.RemoveAll(Dir)
	Duration = 1 * time.Second
	Age = 4 * time.Second
	if err := Init("tlog"); err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(10 * time.Second)
		logger.quit = true
	}()
	tick := time.Tick(time.Second / 4)
	for i := 0; ; i++ {
		select {
		case <-tick:
			Infof("message %d", i)
			continue
		case <-logger.done:
		}
		break
	}
	// We should look in Dir to see if we have the right
	// number of files.
}
