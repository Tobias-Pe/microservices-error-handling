/*
 * MIT License
 *
 * Copyright (c) 2021 Tobias Leonhard Joschka Peslalz
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sony/gobreaker"
)

type CircuitBreakerMetric struct {
	circuitBreakerCounterVec *prometheus.CounterVec
}

func NewCircuitBreakerMetric() *CircuitBreakerMetric {
	metric := CircuitBreakerMetric{}
	metric.circuitBreakerCounterVec = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "circuit_breaker_states",
			Help: "The number of circuit breaker states by name and state.",
		},
		[]string{"name", "status"},
	)
	return &metric
}

func (metric *CircuitBreakerMetric) InitMetric(name string) {
	_, _ = metric.circuitBreakerCounterVec.GetMetricWith(prometheus.Labels{"name": name, "status": "closed"})
	_, _ = metric.circuitBreakerCounterVec.GetMetricWith(prometheus.Labels{"name": name, "status": "half-open"})
	_, _ = metric.circuitBreakerCounterVec.GetMetricWith(prometheus.Labels{"name": name, "status": "open"})
}

func (metric *CircuitBreakerMetric) Increment(state gobreaker.State, name string) {
	if counter, err := metric.circuitBreakerCounterVec.GetMetricWith(prometheus.Labels{"name": name, "status": state.String()}); err == nil {
		counter.Inc()
	} else {
		logger.WithError(err).Warn("Could not increment")
	}
}
