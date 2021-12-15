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
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"gitlab.lrz.de/peslalz/errorhandling-microservices-thesis/api/proto"
	"google.golang.org/grpc"
	"net/http"
	"time"
)

type CartClient struct {
	Conn   *grpc.ClientConn
	client proto.CartClient
}

type articleId struct {
	ArticleId string `json:"article_id" xml:"article_id"`
}

func NewCartClient(cartAddress string, cartPort string) *CartClient {
	// Set up a connection to the server.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()
	conn, err := grpc.DialContext(ctx, cartAddress+":"+cartPort, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		logger.Errorf("did not connect to cart-service: %v", err)
	} else {
		logger.Infoln("Connection to cart-service successfully!")
	}
	client := proto.NewCartClient(conn)
	return &CartClient{Conn: conn, client: client}
}

func (cartClient CartClient) CreateCart() func(c *gin.Context) {
	return func(c *gin.Context) {
		objArticleId := articleId{}
		request := proto.RequestCart{}
		if err := c.ShouldBindWith(&objArticleId, binding.JSON); err == nil {
			request.ArticleId = objArticleId.ArticleId
		} else {
			logger.WithError(err).Warnf("Binding unsuccessfull")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		response, err := cartClient.client.CreateCart(ctx, &request)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		} else {
			c.JSON(http.StatusCreated, gin.H{"cart": response.Id})
		}
	}
}
