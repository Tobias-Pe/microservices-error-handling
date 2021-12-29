package log

import (
	"github.com/sirupsen/logrus"
	"testing"
	"time"
)

func TestInitLogger(t *testing.T) {
	var logger *logrus.Logger
	var prevlogger *logrus.Logger
	go func() {
		for i := 0; i < 1000; i++ {
			go func(i int) {
				time.Sleep(time.Millisecond * 600)
				prevlogger = logger
				logger = InitLogger()
				if logger == nil {
					t.Fail()
				} else if prevlogger != nil && logger != prevlogger {
					t.Fail()
				}
			}(i)
		}
	}()
}
