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
	"encoding/json"
	"fmt"
	"github.com/Tobias-Pe/Microservices-Errorhandling/api/proto"
	"github.com/Tobias-Pe/Microservices-Errorhandling/api/requests"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	loggrus "github.com/sirupsen/logrus"
	"github.com/streadway/amqp"
	"google.golang.org/grpc"
	"math"
	"net/http"
	"time"
)

type CartClient struct {
	GrpcConn    *grpc.ClientConn
	AmqpConn    *amqp.Connection
	AmqpChannel *amqp.Channel
	grpcClient  proto.CartClient
}

// restBody is a temporary struct for json binding
type restBody struct {
	ArticleId string `json:"article_id"`
}

func NewCartClient(cartAddress string, cartPort string, rabbitAddress string, rabbitPort string) *CartClient {
	cc := &CartClient{}
	err := cc.initGrpcConnection(cartAddress, cartPort)
	if err != nil {
		return nil
	}
	// retry connection to rabbitmq with 2^i sleep
	for i := 0; i < 6; i++ {
		err = cc.initAmqpConnection(rabbitAddress, rabbitPort)
		if err == nil {
			break
		}
		logger.Infof("Retrying... (%d/%d)", i, 5)
		time.Sleep(time.Duration(int64(math.Pow(2, float64(i)))) * time.Second)
	}
	if err != nil {
		return nil
	}
	logger.Infoln("Connection to cart-service successfully!")
	return cc
}

func (cartClient *CartClient) initGrpcConnection(cartAddress string, cartPort string) error {
	// Set up a connection to the server.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()
	var err error
	// connection will be closed in main
	cartClient.GrpcConn, err = grpc.DialContext(ctx, cartAddress+":"+cartPort, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		logger.Errorf("did not connect to cart-service: %v", err)
		return err
	}
	cartClient.grpcClient = proto.NewCartClient(cartClient.GrpcConn)
	return nil
}

func (cartClient *CartClient) initAmqpConnection(rabbitAddress string, rabbitPort string) error {
	url := fmt.Sprintf("amqp://guest:guest@%s:%s/", rabbitAddress, rabbitPort)
	conn, err := amqp.Dial(url)
	if err != nil {
		logger.WithError(err).WithFields(loggrus.Fields{"url": url}).Error("Could not connect to rabbitMq")
		return err
	}
	// connection will be closed in main
	cartClient.AmqpConn = conn
	// channel will be closed in main
	cartClient.AmqpChannel, err = cartClient.AmqpConn.Channel()
	if err != nil {
		logger.WithError(err).Error("Could not create channel")
		return err
	}
	err = cartClient.AmqpChannel.ExchangeDeclare(
		requests.ArticlesTopic, // name
		"topic",                // type
		true,                   // durable
		false,                  // auto-deleted
		false,                  // internal
		false,                  // no-wait
		nil,                    // arguments
	)
	if err != nil {
		logger.WithError(err).Error("Could not declare exchange")
		return err
	}
	return nil
}

// CreateCart sends grpc request to create a cart to the cart service
func (cartClient CartClient) CreateCart() gin.HandlerFunc {
	return func(c *gin.Context) {
		// bind json to restBody struct type
		objArticleId := restBody{}
		if err := c.ShouldBindWith(&objArticleId, binding.JSON); err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		request := proto.RequestNewCart{}
		request.ArticleId = objArticleId.ArticleId
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		response, err := cartClient.grpcClient.CreateCart(ctx, &request)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		} else {
			c.JSON(http.StatusCreated, gin.H{"cart": response})
		}
	}
}

// GetCart sends grpc request to get all articleID's inside the requested cartID
func (cartClient CartClient) GetCart() gin.HandlerFunc {
	return func(c *gin.Context) {
		// fetch cartID from url parameter --> cart/${id}
		cartID := c.Param("id")
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		response, err := cartClient.grpcClient.GetCart(ctx, &proto.RequestCart{CartId: cartID})
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		} else {
			c.JSON(http.StatusOK, gin.H{"article_ids": response.ArticleIds})
		}
	}
}

// AddToCart sends amqp message to insert an articleID into an existing cart
func (cartClient CartClient) AddToCart() gin.HandlerFunc {
	return func(c *gin.Context) {
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
		request := requests.PutArticleInCartRequest{
			ArticleID: objArticleId.ArticleId,
			CartID:    cartID,
		}
		bytes, err := json.Marshal(request)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		err = cartClient.AmqpChannel.Publish(
			requests.ArticlesTopic,       // exchange
			requests.AddToCartRoutingKey, // routing key
			true,                         // mandatory
			false,                        // immediate
			amqp.Publishing{
				DeliveryMode: amqp.Persistent,
				ContentType:  "application/json",
				Body:         bytes,
			})
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		// it's an async message, so we can only acknowledge that the request was sent
		c.JSON(http.StatusAccepted, gin.H{})
	}
}
