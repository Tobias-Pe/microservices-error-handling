package main

import (
	loggingUtil "github.com/Tobias-Pe/Microservices-Errorhandling/pkg/log"
	"time"
)

// go run -race this file
func main() {
	for i := 0; i < 1000; i++ {
		go func(i int) {
			time.Sleep(time.Millisecond * 600)
			logger := loggingUtil.InitLogger()
			logger.Info(" - ", i)
		}(i)
	}
	time.Sleep(time.Second * 3)
}
