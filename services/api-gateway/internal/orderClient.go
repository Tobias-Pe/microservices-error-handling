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
	"github.com/Tobias-Pe/Microservices-Errorhandling/api/proto"
	"github.com/Tobias-Pe/Microservices-Errorhandling/pkg/models"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"google.golang.org/grpc"
	"net/http"
	"time"
)

type OrderClient struct {
	Conn   *grpc.ClientConn
	client proto.OrderClient
}

func NewOrderClient(orderAddress string, orderPort string) *OrderClient {
	logger.Infof("connecting to order: %s", orderAddress+":"+orderPort)
	// Set up a connection to the server.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*35)
	defer cancel()
	conn, err := grpc.DialContext(ctx, orderAddress+":"+orderPort, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		logger.Errorf("did not connect to order-service: %v", err)
	} else {
		logger.Infoln("Connection to order-service successfully!")
	}
	client := proto.NewOrderClient(conn)
	return &OrderClient{Conn: conn, client: client}
}

func (orderClient OrderClient) GetOrder() gin.HandlerFunc {
	return func(c *gin.Context) {
		orderId := c.Param("id")
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		response, err := orderClient.client.GetOrder(ctx, &proto.RequestOrder{OrderId: orderId})
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		} else {
			c.JSON(http.StatusOK, gin.H{"order": response})
		}
	}
}

func (orderClient OrderClient) CreateOrder() gin.HandlerFunc {
	return func(c *gin.Context) {
		order := models.Order{}
		if err := c.ShouldBindWith(&order, binding.JSON); err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		}
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		response, err := orderClient.client.CreateOrder(ctx, &proto.RequestNewOrder{
			CartId:             order.CartID,
			CustomerAddress:    order.CustomerAddress,
			CustomerName:       order.CustomerName,
			CustomerCreditCard: order.CustomerCreditCard,
		})
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		} else {
			c.JSON(http.StatusOK, gin.H{"order": response})
		}
	}
}
