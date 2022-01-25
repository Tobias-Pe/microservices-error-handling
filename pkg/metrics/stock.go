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

type StockMetric struct {
	reservationsGauge prometheus.Gauge
	articlesGaugeVec  *prometheus.GaugeVec
}

func NewStockMetric() *StockMetric {
	reservationsGauge := promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "reservations",
			Help: "The total number of reservations.",
		},
	)

	articlesGaugeVec := promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "articles",
			Help: "The total number of articles by id, name and category.",
		},
		[]string{"id", "name", "category"},
	)

	stockMetric := StockMetric{
		reservationsGauge: reservationsGauge,
		articlesGaugeVec:  articlesGaugeVec,
	}

	return &stockMetric
}

func (stockMetric *StockMetric) IncrementReservation() {
	stockMetric.reservationsGauge.Inc()
}

func (stockMetric *StockMetric) DecrementReservation() {
	stockMetric.reservationsGauge.Dec()
}

func (stockMetric *StockMetric) UpdateArticle(article models.Article) {
	gauge, err := stockMetric.articlesGaugeVec.GetMetricWith(prometheus.Labels{"id": article.ID.Hex(), "name": article.Name, "category": article.Category})
	if err == nil {
		gauge.Set(float64(article.Amount))
	} else {
		logger.WithError(err).Warn("Could not update")
	}
}

func (stockMetric *StockMetric) UpdateArticles(articles []models.Article) {
	for _, article := range articles {
		gauge, err := stockMetric.articlesGaugeVec.GetMetricWith(prometheus.Labels{"id": article.ID.Hex(), "name": article.Name, "category": article.Category})
		if err == nil {
			gauge.Set(float64(article.Amount))
		} else {
			logger.WithError(err).Warn("Could not update")
		}
	}
}
