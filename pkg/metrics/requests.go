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
