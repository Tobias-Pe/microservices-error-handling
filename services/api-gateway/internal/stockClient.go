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
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"net/http"
	"strings"
	"time"
)

type StockClient struct {
	Conn   *grpc.ClientConn
	client proto.StockClient
}

func NewStockClient(stockAddress string, stockPort string) *StockClient {
	// Set up a connection to the server.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()
	conn, err := grpc.DialContext(ctx, stockAddress+":"+stockPort, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		logger.WithError(err).Error("did not connect to stock-service")
	} else {
		logger.Infoln("Connection to stock-service successfully!")
	}
	client := proto.NewStockClient(conn)
	return &StockClient{Conn: conn, client: client}
}

// GetArticles creates a grpc request to fetch all articles from the stock service
func (stockClient StockClient) GetArticles() gin.HandlerFunc {
	return func(c *gin.Context) {
		// get the optional category parameter --> articles/${category}
		queryCategory := strings.Trim(c.Param("category"), "/")
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		request := &proto.RequestArticles{
			CategoryQuery: queryCategory,
		}
		response, err := stockClient.client.GetArticles(ctx, request)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		} else {
			c.JSON(http.StatusOK, gin.H{"articles": response.Articles})
		}
	}
}
