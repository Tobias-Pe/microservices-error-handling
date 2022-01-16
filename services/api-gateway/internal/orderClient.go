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
	"github.com/Tobias-Pe/Microservices-Errorhandling/api/proto"
	"github.com/Tobias-Pe/Microservices-Errorhandling/pkg/models"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/sony/gobreaker"
	"google.golang.org/grpc"
	"net/http"
	"net/mail"
	"time"
)

type OrderClient struct {
	Conn   *grpc.ClientConn
	client proto.OrderClient
}

func NewOrderClient(orderAddress string, orderPort string) *OrderClient {
	logger.Infof("connecting to order: %s", orderAddress+":"+orderPort)
	// Set up a connection to the server.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*ConnectionTimeSecs)
	defer cancel()
	conn, err := grpc.DialContext(ctx, orderAddress+":"+orderPort, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		logger.WithError(err).Error("did not connect to order-service")
		return nil
	}
	logger.Infoln("Connection to order-service successfully!")

	client := proto.NewOrderClient(conn)
	return &OrderClient{Conn: conn, client: client}
}

// GetOrder sends grpc request to fetch an order from order service
func (orderClient OrderClient) GetOrder(c *gin.Context, cb *gobreaker.CircuitBreaker) {
	orderId := c.Param("id")
	response, err := cb.Execute(func() (interface{}, error) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), time.Duration(2500)*time.Millisecond)
		defer cancel()
		return orderClient.client.GetOrder(ctx, &proto.RequestOrder{OrderId: orderId})
	})
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	} else {
		c.JSON(http.StatusOK, gin.H{"order": response.(*proto.OrderObject)})
	}
}

// CreateOrder sends grpc request to create an order with the required data
func (orderClient OrderClient) CreateOrder(c *gin.Context, cb *gobreaker.CircuitBreaker) {
	// bind json body to an order object
	order := models.Order{}
	if err := c.ShouldBindWith(&order, binding.JSON); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// validate that all required fields are populated
	if len(order.CartID) == 0 || len(order.CustomerAddress) == 0 || len(order.CustomerName) == 0 || len(order.CustomerCreditCard) == 0 || len(order.CustomerEmail) == 0 {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": fmt.Errorf("invalid order request, values missing").Error()})
		return
	}
	// validate email address
	_, err := mail.ParseAddress(order.CustomerEmail)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	response, err := cb.Execute(func() (interface{}, error) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), time.Duration(2500)*time.Millisecond)
		defer cancel()
		// send request to order service
		return orderClient.client.CreateOrder(ctx, &proto.RequestNewOrder{
			CartId:             order.CartID,
			CustomerAddress:    order.CustomerAddress,
			CustomerName:       order.CustomerName,
			CustomerCreditCard: order.CustomerCreditCard,
			CustomerEmail:      order.CustomerEmail,
		})
	})
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	} else {
		c.JSON(http.StatusOK, gin.H{"order": response.(*proto.OrderObject)})
	}
}
