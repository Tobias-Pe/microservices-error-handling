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

package log

import (
	"github.com/Abramovic/logrus_influxdb"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sirupsen/logrus"
	"os"
	"sync"
	"time"
)

// PrometheusHook contains a counter vector for counting log statements severity levels
type PrometheusHook struct {
	counter *prometheus.CounterVec
}

func NewPrometheusHook() *PrometheusHook {
	counter := promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "log_statements_total",
			Help: "Number of log statements, differentiated by log level.",
		},
		[]string{"level"},
	)

	return &PrometheusHook{
		counter: counter,
	}
}

func (h *PrometheusHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (h *PrometheusHook) Fire(e *logrus.Entry) error {
	h.counter.WithLabelValues(e.Level.String()).Inc()
	return nil
}

// logger is a singleton
var logger *logrus.Logger
var once sync.Once

func InitLogger() *logrus.Logger {
	once.Do(func() {
		logger = logrus.New()
		logger.Out = os.Stdout
		logger.SetLevel(logrus.InfoLevel)
		logger.SetFormatter(&logrus.TextFormatter{
			ForceColors:      true,
			DisableColors:    false,
			DisableTimestamp: false,
			FullTimestamp:    true,
			TimestampFormat:  "15:04:05",
			DisableSorting:   false,
			SortingFunc:      nil,
			PadLevelText:     true,
			QuoteEmptyFields: true,
		})
		logger.AddHook(NewPrometheusHook())

		config := &logrus_influxdb.Config{
			Host:          "influxdb",
			Port:          8086,
			Database:      "logrus",
			UseHTTPS:      false,
			Precision:     "ns",
			AppName:       "Microservices-Errorhandling",
			Tags:          []string{"logrus-logs"},
			BatchInterval: 5 * time.Second,
			BatchCount:    0, // set to "0" to disable batching
		}

		go func() {
			for {
				hook, err := logrus_influxdb.NewInfluxDB(config)
				if err == nil {
					logger.Hooks.Add(hook)
					break
				} else {
					time.Sleep(time.Second * 5)
				}
			}
			logger.Infof("Connected to Influx!")
		}()
	})

	return logger
}
