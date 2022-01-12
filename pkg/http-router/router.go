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
			time.Sleep(time.Second * time.Duration(1))
		}
	}()
}
