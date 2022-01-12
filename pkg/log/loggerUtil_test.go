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
	"github.com/sirupsen/logrus"
	"testing"
	"time"
)

func TestInitLogger(t *testing.T) {
	var logger *logrus.Logger
	var prevLogger *logrus.Logger
	go func() {
		for i := 0; i < 1000; i++ {
			go func(i int) {
				time.Sleep(time.Millisecond * time.Duration(500))
				prevLogger = logger
				logger = InitLogger()
				if logger == nil {
					t.Fail()
				} else if prevLogger != nil && logger != prevLogger {
					t.Fail()
				}
			}(i)
		}
	}()
}
