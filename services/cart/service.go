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

package cart

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gomodule/redigo/redis"
	loggrus "github.com/sirupsen/logrus"
	"gitlab.lrz.de/peslalz/errorhandling-microservices-thesis/api/proto"
	loggingUtil "gitlab.lrz.de/peslalz/errorhandling-microservices-thesis/pkg/log"
	"gitlab.lrz.de/peslalz/errorhandling-microservices-thesis/pkg/models"
	"strconv"
)

const expireCartSeconds = 172800

var logger = loggingUtil.InitLogger()

type Service struct {
	proto.UnimplementedCartServer
	connPool *redis.Pool
}

func NewService(cacheAddress string, cachePort string) *Service {
	newService := Service{}
	connectionUri := cacheAddress + ":" + cachePort
	newService.connPool = &redis.Pool{
		MaxIdle:   80,
		MaxActive: 12000,
		Dial: func() (redis.Conn, error) {
			c, err := redis.Dial("tcp", connectionUri)
			if err != nil {
				logger.WithError(err).Error("Error appeared on dialing redis!")
			}
			return c, err
		},
	}
	return &newService
}

func (s Service) CreateCart(_ context.Context, req *proto.RequestNewCart) (*proto.ResponseNewCart, error) {
	cart, err := s.createCart(req.ArticleId)
	if err != nil {
		return nil, err
	}

	strId := strconv.Itoa(int(cart.ID))
	logger.WithFields(loggrus.Fields{"ID": cart.ID, "Data": cart.ArticleIDs, "Request": req.ArticleId}).Info("Created new Cart")
	return &proto.ResponseNewCart{CartId: strId}, nil
}

func (s Service) GetCart(_ context.Context, req *proto.RequestCart) (*proto.ResponseCart, error) {
	cart, err := s.getCart(req.CartId)
	if err != nil {
		return nil, err
	}
	logger.WithFields(loggrus.Fields{"ID": cart.ID, "Data": cart.ArticleIDs, "Request": req.CartId}).Info("Looked up Cart")
	return &proto.ResponseCart{ArticleIds: cart.ArticleIDs}, nil
}

func (s Service) getCart(strCartId string) (*models.Cart, error) {
	tmpId, err := strconv.Atoi(strCartId)
	if err != nil {
		return nil, err
	}
	id := int64(tmpId)
	fetchedCart := models.Cart{
		ID:         id,
		ArticleIDs: nil,
	}

	client := s.connPool.Get()
	defer func(client redis.Conn) {
		err := client.Close()
		if err != nil {
			logger.WithError(err).Warn("connection to redis could not be successfully closed")
		}
	}(client)
	jsonArticles, err := client.Do("GET", id)
	if err != nil {
		return nil, err
	}
	if jsonArticles == nil {
		return nil, fmt.Errorf("there is no cart for this id: %s", strCartId)
	}
	var articles []string
	err = json.Unmarshal(jsonArticles.([]byte), &articles)
	if err != nil {
		return nil, err
	}
	fetchedCart.ArticleIDs = articles
	return &fetchedCart, nil
}

func (s Service) createCart(strArticleId string) (*models.Cart, error) {
	newCart := models.Cart{
		ID:         -1,
		ArticleIDs: []string{strArticleId},
	}

	client := s.connPool.Get()
	defer func(client redis.Conn) {
		err := client.Close()
		if err != nil {
			logger.WithError(err).Warn("connection to redis could not be successfully closed")
		}
	}(client)

	cartId, err := client.Do("INCR", "cartId")
	if err != nil {
		return nil, err
	}

	newCart.ID = cartId.(int64)
	marshal, err := json.Marshal(newCart.ArticleIDs)
	if err != nil {
		return nil, err
	}
	_, err = client.Do("SET", cartId, string(marshal))
	if err != nil {
		return nil, err
	}
	_, err = client.Do("EXPIRE", cartId, expireCartSeconds)
	if err != nil {
		return nil, err
	}

	return &newCart, nil
}
