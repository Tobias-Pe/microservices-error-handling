package circuitbreaker

import (
	"github.com/Tobias-Pe/Microservices-Errorhandling/pkg/metrics"
	"github.com/sony/gobreaker"
	"time"
)

func NewCircuitBreaker(name string, cbMetric *metrics.CircuitBreakerMetric) *gobreaker.CircuitBreaker {
	var st gobreaker.Settings
	st.Name = name
	st.ReadyToTrip = func(counts gobreaker.Counts) bool {
		failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
		return counts.Requests >= 50 && failureRatio >= 0.25
	}
	st.Timeout = time.Duration(3) * time.Second
	st.MaxRequests = 25
	st.Interval = time.Duration(10) * time.Second
	st.OnStateChange = func(_ string, from gobreaker.State, to gobreaker.State) {
		cbMetric.Update(from, to, name)
	}
	cbMetric.InitMetric(name)

	cb := gobreaker.NewCircuitBreaker(st)

	return cb
}
