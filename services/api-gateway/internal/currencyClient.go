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

package internal

import (
	"context"
	"fmt"
	movingaverage "github.com/RobinUS2/golang-moving-average"
	"github.com/Tobias-Pe/Microservices-Errorhandling/api/proto"
	loggingUtil "github.com/Tobias-Pe/Microservices-Errorhandling/pkg/log"
	"github.com/gin-gonic/gin"
	"github.com/sony/gobreaker"
	"google.golang.org/grpc"
	"net/http"
	"regexp"
	"time"
)

var logger = loggingUtil.InitLogger()

const movingWindowSize = 100

type CurrencyClient struct {
	Conn            *grpc.ClientConn
	client          proto.CurrencyClient
	timeoutDuration time.Duration
	movingTimeout   *movingaverage.ConcurrentMovingAverage
}

func NewCurrencyClient(currencyAddress string, currencyPort string, staticTimeoutMillis *int) *CurrencyClient {
	// Set up a connection to the server.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*ConnectionTimeSecs)
	defer cancel()
	conn, err := grpc.DialContext(ctx, currencyAddress+":"+currencyPort, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		logger.WithError(err).Error("did not connect to currency-service")
		return nil
	}
	logger.Infoln("Connection to currency-service successfully!")

	client := proto.NewCurrencyClient(conn)
	currencyClient := CurrencyClient{Conn: conn, client: client}
	if staticTimeoutMillis != nil {
		currencyClient.timeoutDuration = time.Duration(*staticTimeoutMillis) * time.Millisecond
	} else {
		currencyClient.timeoutDuration = time.Duration(1000) * time.Millisecond
		currencyClient.movingTimeout = movingaverage.Concurrent(movingaverage.New(movingWindowSize))
	}
	return &currencyClient
}

// GetExchangeRate validates client request and creates grpc request to the service
func (currencyClient CurrencyClient) GetExchangeRate(c *gin.Context, cb *gobreaker.CircuitBreaker) {
	// get parameter --> exchange/${currency}
	currency := c.Param("currency")
	// validate the currency parameter for only letters
	isOnlyLetters := regexp.MustCompile("^[a-zA-Z]+$").MatchString(currency)
	if len(currency) == 0 || !isOnlyLetters {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("only letters in param allowed, your param: %s", currency)})
		return
	}
	targetCurrency := &proto.RequestExchangeRate{CustomerCurrency: currency}

	start := time.Now()
	var response interface{}
	var err error
	if cb != nil {
		response, err = cb.Execute(func() (interface{}, error) {
			ctx, cancel := context.WithTimeout(c.Request.Context(), currencyClient.timeoutDuration)
			defer cancel()
			response, err := currencyClient.client.GetExchangeRate(ctx, targetCurrency)
			return response, err
		})
	} else {
		ctx, cancel := context.WithTimeout(c.Request.Context(), currencyClient.timeoutDuration)
		response, err = currencyClient.client.GetExchangeRate(ctx, targetCurrency)
		cancel()
	}
	elapsed := time.Since(start)
	currencyClient.calcTimeout(elapsed)

	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	} else {
		c.JSON(http.StatusOK, gin.H{"exchangeRate": response.(*proto.ReplyExchangeRate).ExchangeRate})
	}
}

func (currencyClient *CurrencyClient) calcTimeout(elapsed time.Duration) {
	if currencyClient.movingTimeout != nil {
		currencyClient.movingTimeout.Add(elapsed.Seconds())
		if currencyClient.movingTimeout.Count() >= movingWindowSize {
			currencyClient.timeoutDuration = time.Duration(currencyClient.movingTimeout.Avg()*1000) * time.Millisecond
		}
	}
}
