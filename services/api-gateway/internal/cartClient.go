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
	"github.com/Tobias-Pe/microservices-error-handling/api/proto"
	"github.com/Tobias-Pe/microservices-error-handling/pkg/metrics"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/sony/gobreaker"
	"google.golang.org/grpc"
	"net/http"
	"time"
)

const ConnectionTimeSecs = 60

type CartClient struct {
	GrpcConn   *grpc.ClientConn
	grpcClient proto.CartClient

	timeoutMetric *metrics.TimeoutMetric

	timeoutDurationCreateCart time.Duration
	movingTimeoutCreateCart   *movingaverage.ConcurrentMovingAverage

	timeoutDurationGetCart time.Duration
	movingTimeoutGetCart   *movingaverage.ConcurrentMovingAverage

	timeoutDurationAddToCart time.Duration
	movingTimeoutAddToCart   *movingaverage.ConcurrentMovingAverage
}

// restBody is a temporary struct for json binding
type restBody struct {
	ArticleId string `json:"article_id"`
}

func NewCartClient(cartAddress string, cartPort string, staticTimeoutMillis *int, metric *metrics.TimeoutMetric) *CartClient {
	cc := &CartClient{}

	cc.timeoutMetric = metric

	if staticTimeoutMillis != nil {
		cc.timeoutDurationCreateCart = time.Duration(*staticTimeoutMillis) * time.Millisecond
		cc.timeoutDurationGetCart = time.Duration(*staticTimeoutMillis) * time.Millisecond
		cc.timeoutDurationAddToCart = time.Duration(*staticTimeoutMillis) * time.Millisecond
		cc.timeoutMetric.Update(*staticTimeoutMillis, "AddToCart")
		cc.timeoutMetric.Update(*staticTimeoutMillis, "GetCart")
		cc.timeoutMetric.Update(*staticTimeoutMillis, "CreateCart")
	} else {
		cc.timeoutDurationCreateCart = time.Duration(1000) * time.Millisecond
		cc.timeoutDurationGetCart = time.Duration(1000) * time.Millisecond
		cc.timeoutDurationAddToCart = time.Duration(1000) * time.Millisecond
		cc.timeoutMetric.Update(1000, "AddToCart")
		cc.timeoutMetric.Update(1000, "GetCart")
		cc.timeoutMetric.Update(1000, "CreateCart")
		cc.movingTimeoutAddToCart = movingaverage.Concurrent(movingaverage.New(movingWindowSize))
		cc.movingTimeoutGetCart = movingaverage.Concurrent(movingaverage.New(movingWindowSize))
		cc.movingTimeoutCreateCart = movingaverage.Concurrent(movingaverage.New(movingWindowSize))
	}
	err := cc.initGrpcConnection(cartAddress, cartPort)
	if err != nil {
		return nil
	}

	logger.Infoln("Connection to cart-service successfully!")
	return cc
}

func (cartClient *CartClient) initGrpcConnection(cartAddress string, cartPort string) error {
	// Set up a connection to the server.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*ConnectionTimeSecs)
	defer cancel()
	var err error
	// connection will be closed in main
	cartClient.GrpcConn, err = grpc.DialContext(ctx, cartAddress+":"+cartPort, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		logger.WithError(err).Error("did not connect to cart-service")
		return err
	}
	cartClient.grpcClient = proto.NewCartClient(cartClient.GrpcConn)
	return nil
}

// CreateCart sends grpc request to create a cart to the cart service
func (cartClient CartClient) CreateCart(c *gin.Context, cb *gobreaker.CircuitBreaker) {
	// bind json to restBody struct type
	objArticleId := restBody{}
	if err := c.ShouldBindWith(&objArticleId, binding.JSON); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	request := proto.RequestNewCart{}
	request.ArticleId = objArticleId.ArticleId

	start := time.Now()
	var response interface{}
	var err error
	if cb != nil {
		response, err = cb.Execute(func() (interface{}, error) {
			ctx, cancel := context.WithTimeout(c.Request.Context(), cartClient.timeoutDurationCreateCart)
			response2, err2 := cartClient.grpcClient.CreateCart(ctx, &request)
			cancel()
			elapsed := time.Since(start)
			cartClient.calcTimeoutCreateCart(elapsed)
			return response2, err2
		})
	} else {
		ctx, cancel := context.WithTimeout(c.Request.Context(), cartClient.timeoutDurationCreateCart)
		response, err = cartClient.grpcClient.CreateCart(ctx, &request)
		cancel()
		elapsed := time.Since(start)
		cartClient.calcTimeoutCreateCart(elapsed)
	}

	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	} else {
		c.JSON(http.StatusCreated, gin.H{"cart": response.(*proto.ResponseNewCart)})
	}
}

// GetCart sends grpc request to get all articleID's inside the requested cartID
func (cartClient CartClient) GetCart(c *gin.Context, cb *gobreaker.CircuitBreaker) {
	// fetch cartID from url parameter --> cart/${id}
	cartID := c.Param("id")

	start := time.Now()
	var response interface{}
	var err error
	if cb != nil {
		response, err = cb.Execute(func() (interface{}, error) {
			ctx, cancel := context.WithTimeout(c.Request.Context(), cartClient.timeoutDurationGetCart)
			cart, err2 := cartClient.grpcClient.GetCart(ctx, &proto.RequestCart{CartId: cartID})
			cancel()
			elapsed := time.Since(start)
			cartClient.calcTimeoutGetCart(elapsed)
			return cart, err2
		})
	} else {
		ctx, cancel := context.WithTimeout(c.Request.Context(), cartClient.timeoutDurationGetCart)
		response, err = cartClient.grpcClient.GetCart(ctx, &proto.RequestCart{CartId: cartID})
		cancel()
		elapsed := time.Since(start)
		cartClient.calcTimeoutGetCart(elapsed)
	}

	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	} else {
		c.JSON(http.StatusOK, gin.H{"article_ids": response.(*proto.ResponseCart).ArticleIds})
	}
}

// AddToCart sends amqp message to insert an articleID into an existing cart
func (cartClient CartClient) AddToCart(c *gin.Context, cb *gobreaker.CircuitBreaker) {
	// fetch cartID from url parameter --> cart/${id}
	cartID := c.Param("id")
	if len(cartID) == 0 {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": fmt.Errorf("the id parameter is required after /cart/")})
		return
	}

	// bind json to restBody struct type
	objArticleId := restBody{}
	if err := c.ShouldBindWith(&objArticleId, binding.JSON); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	start := time.Now()
	var err error
	if cb != nil {
		_, err = cb.Execute(func() (interface{}, error) {
			ctx, cancel := context.WithTimeout(c.Request.Context(), cartClient.timeoutDurationAddToCart)
			cart, err2 := cartClient.grpcClient.PutCart(ctx, &proto.RequestPutCart{CartId: cartID, ArticleId: objArticleId.ArticleId})
			cancel()
			elapsed := time.Since(start)
			cartClient.calcTimeoutAddToCart(elapsed)
			return cart, err2
		})
	} else {
		ctx, cancel := context.WithTimeout(c.Request.Context(), cartClient.timeoutDurationAddToCart)
		_, err = cartClient.grpcClient.PutCart(ctx, &proto.RequestPutCart{CartId: cartID, ArticleId: objArticleId.ArticleId})
		cancel()
		elapsed := time.Since(start)
		cartClient.calcTimeoutAddToCart(elapsed)
	}

	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	} else {
		c.JSON(http.StatusCreated, nil)
	}
}

func (cartClient *CartClient) calcTimeoutCreateCart(elapsed time.Duration) {
	if cartClient.movingTimeoutCreateCart != nil {
		cartClient.movingTimeoutCreateCart.Add(elapsed.Seconds())
		if cartClient.movingTimeoutCreateCart.Count() >= movingWindowSize {
			timeMillis := (cartClient.movingTimeoutCreateCart.Avg() * 1000) * 1.5
			cartClient.timeoutDurationCreateCart = time.Duration(timeMillis) * time.Millisecond
			cartClient.timeoutMetric.Update(int(timeMillis), "CreateCart")
		}
	}
}

func (cartClient *CartClient) calcTimeoutGetCart(elapsed time.Duration) {
	if cartClient.movingTimeoutGetCart != nil {
		cartClient.movingTimeoutGetCart.Add(elapsed.Seconds())
		if cartClient.movingTimeoutGetCart.Count() >= movingWindowSize {
			timeMillis := (cartClient.movingTimeoutGetCart.Avg() * 1000) * 1.5
			cartClient.timeoutDurationGetCart = time.Duration(timeMillis) * time.Millisecond
			cartClient.timeoutMetric.Update(int(timeMillis), "GetCart")
		}
	}
}

func (cartClient *CartClient) calcTimeoutAddToCart(elapsed time.Duration) {
	if cartClient.movingTimeoutAddToCart != nil {
		cartClient.movingTimeoutAddToCart.Add(elapsed.Seconds())
		if cartClient.movingTimeoutAddToCart.Count() >= movingWindowSize {
			timeMillis := (cartClient.movingTimeoutAddToCart.Avg() * 1000) * 1.5
			cartClient.timeoutDurationAddToCart = time.Duration(timeMillis) * time.Millisecond
			cartClient.timeoutMetric.Update(int(timeMillis), "AddToCart")
		}
	}
}
