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

package api_gateway

import (
	"context"
	"fmt"
	"github.com/gin-gonic/gin"
	proto2 "gitlab.lrz.de/peslalz/errorhandling-microservices-thesis/api/proto"
	loggingUtil "gitlab.lrz.de/peslalz/errorhandling-microservices-thesis/pkg/log"
	"google.golang.org/grpc"
	"net/http"
	"regexp"
	"time"
)

var logger = loggingUtil.InitLogger()

type Service struct {
	currencyAddress string
	Conn            *grpc.ClientConn
	client          proto2.CurrencyClient
}

func NewService(currencyAddress string) *Service {
	// Set up a connection to the server.
	conn, err := grpc.Dial(currencyAddress, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		logger.Fatalf("did not connect: %v", err)
	}
	client := proto2.NewCurrencyClient(conn)
	return &Service{currencyAddress: currencyAddress, Conn: conn, client: client}
}

func (s Service) GetExchangeRate() func(c *gin.Context) {
	return func(c *gin.Context) {
		currency := c.Param("currency")
		isOnlyLetters := regexp.MustCompile("^[a-zA-Z]+$").MatchString(currency)
		if len(currency) > 0 && isOnlyLetters {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()
			targetCurrency := &proto2.RequestExchangeRate{CustomerCurrency: currency}
			response, err := s.client.GetExchangeRate(ctx, targetCurrency)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			} else {
				c.JSON(http.StatusOK, gin.H{"exchangeRate": response.ExchangeRate})
			}
		} else {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("only letters in param allowed, your param: %s", currency)})
		}
	}
}
