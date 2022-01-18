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
)

type TimeoutMetric struct {
	timeoutGaugeVec *prometheus.GaugeVec
}

func NewTimeoutMetric() *TimeoutMetric {
	metric := TimeoutMetric{}
	metric.timeoutGaugeVec = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "timeout_value",
			Help: "The milliseconds after which the transaction will be aborted if not completed.",
		},
		[]string{"method"},
	)
	return &metric
}

func (metric *TimeoutMetric) Update(timeoutMillis int, method string) {
	gauge, err := metric.timeoutGaugeVec.GetMetricWith(prometheus.Labels{"method": method})
	if err != nil {
		logger.WithError(err).Warn("Could not increment")
		return
	}
	gauge.Set(float64(timeoutMillis))
}
