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
	"github.com/Tobias-Pe/microservices-error-handling/pkg/models"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type OrdersMetric struct {
	ordersGaugeVec *prometheus.GaugeVec
}

func NewOrdersMetric() *OrdersMetric {
	ordersGaugeVec := promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "orders",
			Help: "The total number of orders by status.",
		},
		[]string{"status"},
	)

	orderMetric := OrdersMetric{ordersGaugeVec}

	_, err := orderMetric.ordersGaugeVec.GetMetricWith(prometheus.Labels{"status": models.StatusFetching().Name})
	if err != nil {
		return nil
	}

	_, err = orderMetric.ordersGaugeVec.GetMetricWith(prometheus.Labels{"status": models.StatusReserving().Name})
	if err != nil {
		return nil
	}

	_, err = orderMetric.ordersGaugeVec.GetMetricWith(prometheus.Labels{"status": models.StatusComplete().Name})
	if err != nil {
		return nil
	}

	_, err = orderMetric.ordersGaugeVec.GetMetricWith(prometheus.Labels{"status": models.StatusShipping().Name})
	if err != nil {
		return nil
	}

	_, err = orderMetric.ordersGaugeVec.GetMetricWith(prometheus.Labels{"status": models.StatusPaying().Name})
	if err != nil {
		return nil
	}

	_, err = orderMetric.ordersGaugeVec.GetMetricWith(prometheus.Labels{"status": models.StatusAborted("").Name})
	if err != nil {
		return nil
	}

	return &orderMetric
}

func (requestsMetric *OrdersMetric) Increment(order models.Order) {
	if gauge, err := requestsMetric.ordersGaugeVec.GetMetricWith(prometheus.Labels{"status": order.Status}); err == nil {
		gauge.Inc()
	} else {
		logger.WithError(err).Warn("Could not increment")
	}
}

func (requestsMetric *OrdersMetric) Decrement(order models.Order) {
	if gauge, err := requestsMetric.ordersGaugeVec.GetMetricWith(prometheus.Labels{"status": order.Status}); err == nil {
		gauge.Dec()
	} else {
		logger.WithError(err).Warn("Could not decrement")
	}
}
