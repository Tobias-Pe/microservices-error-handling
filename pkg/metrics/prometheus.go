package metrics

import (
	loggingUtils "github.com/Tobias-Pe/Microservices-Errorhandling/pkg/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var logger = loggingUtils.InitLogger()

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
