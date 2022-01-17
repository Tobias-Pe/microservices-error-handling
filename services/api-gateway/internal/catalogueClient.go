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
	"github.com/sony/gobreaker"
	"google.golang.org/grpc"
	"net/http"
	"strings"
	"time"
)

type CatalogueClient struct {
	Conn            *grpc.ClientConn
	client          proto.CatalogueClient
	timeoutDuration time.Duration
}

func NewCatalogueClient(catalogueAddress string, cataloguePort string, staticTimeoutMillis *int) *CatalogueClient {
	// Set up a connection to the server.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*ConnectionTimeSecs)
	defer cancel()
	conn, err := grpc.DialContext(ctx, catalogueAddress+":"+cataloguePort, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		logger.WithError(err).Error("did not connect to catalogue-service")
		return nil
	}
	logger.Infoln("Connection to catalogue-service successfully!")

	client := proto.NewCatalogueClient(conn)
	catalogueClient := CatalogueClient{Conn: conn, client: client}
	if staticTimeoutMillis != nil {
		catalogueClient.timeoutDuration = time.Duration(*staticTimeoutMillis) * time.Millisecond
	}
	return &catalogueClient
}

// GetArticles creates a grpc request to fetch all articles from the catalogue service
func (catalogueClient CatalogueClient) GetArticles(c *gin.Context, cb *gobreaker.CircuitBreaker) {
	// get the optional category parameter --> articles/${category}
	queryCategory := strings.Trim(c.Param("category"), "/")
	request := &proto.RequestArticles{
		CategoryQuery: queryCategory,
	}

	var response interface{}
	var err error
	if cb != nil {
		response, err = cb.Execute(func() (interface{}, error) {
			ctx, cancel := context.WithTimeout(c.Request.Context(), catalogueClient.timeoutDuration)
			defer cancel()
			return catalogueClient.client.GetArticles(ctx, request)
		})
	} else {
		ctx, cancel := context.WithTimeout(c.Request.Context(), catalogueClient.timeoutDuration)
		response, err = catalogueClient.client.GetArticles(ctx, request)
		cancel()
	}

	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	} else {
		c.JSON(http.StatusOK, gin.H{"articles": response.(*proto.ResponseArticles).Articles})
	}
}
