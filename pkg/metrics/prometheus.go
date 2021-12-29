package metrics

import (
	loggingUtils "github.com/Tobias-Pe/Microservices-Errorhandling/pkg/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"net/http"
	"time"
)

var logger = loggingUtils.InitLogger()

// NewServer is non-blocking.
// It will start a http handler on port 9216 serving prometheus metrics on /metrics and will restart on error with a timeout of 1 second
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

type RequestsMetric struct {
	requestsCounterVec *prometheus.CounterVec
}

func NewRequestsMetrics() *RequestsMetric {
	reqMetric := RequestsMetric{}
	reqMetric.requestsCounterVec = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "requests_total",
			Help: "The total number of service requests by error and method.",
		},
		[]string{"error", "method"},
	)
	return &reqMetric
}

func (requestsMetric *RequestsMetric) Increment(err error, requestName string) {
	if err != nil {
		if counter, err := requestsMetric.requestsCounterVec.GetMetricWith(prometheus.Labels{"error": err.Error(), "method": requestName}); err == nil {
			counter.Inc()
		} else {
			logger.WithError(err).Warn("Could not increment")
		}
	} else {
		if counter, err := requestsMetric.requestsCounterVec.GetMetricWith(prometheus.Labels{"error": "", "method": requestName}); err == nil {
			counter.Inc()
		} else {
			logger.WithError(err).Warn("Could not increment")
		}
	}
}
