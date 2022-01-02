package httpRouter

import (
	loggingUtils "github.com/Tobias-Pe/Microservices-Errorhandling/pkg/log"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"net/http"
	"time"
)

var logger = loggingUtils.InitLogger()

// NewServer is non-blocking.
// It will start a http handler on port 9216
//
// serves prometheus metrics on /metrics and will restart on error with a timeout of 1 second
func NewServer() {
	go func() {
		for {
			http.Handle("/metrics", promhttp.Handler())

			err := http.ListenAndServe(":9216", nil)
			if err != nil {
				logger.WithError(err).Error("prometheus metrics server stopped listening")
			}
			time.Sleep(time.Second * 1)
		}
	}()
}
