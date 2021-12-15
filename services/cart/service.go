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
	"github.com/gomodule/redigo/redis"
	loggrus "github.com/sirupsen/logrus"
	proto "gitlab.lrz.de/peslalz/errorhandling-microservices-thesis/api/proto"
	loggingUtil "gitlab.lrz.de/peslalz/errorhandling-microservices-thesis/pkg/log"
	"strconv"
)

const expireCartSeconds = 172800

var logger = loggingUtil.InitLogger()

type Service struct {
	proto.UnimplementedCartServer
	connPool *redis.Pool
}

type Cart struct {
	Id         int64    `json:"id"`
	ArticleIds []string `json:"articles"`
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

func (s Service) CreateCart(_ context.Context, req *proto.RequestCart) (*proto.ResponseCart, error) {

	newCart := Cart{
		Id:         -1,
		ArticleIds: []string{},
	}

	if len(req.ArticleId) > 0 {
		newCart.ArticleIds = append(newCart.ArticleIds, req.ArticleId)
	}

	client := s.connPool.Get()
	defer func(client redis.Conn) {
		err := client.Close()
		if err != nil {

		}
	}(client)

	cartId, err := client.Do("INCR", "cartId")
	if err != nil {
		return nil, err
	}

	newCart.Id = cartId.(int64)
	marshal, err := json.Marshal(newCart.ArticleIds)
	if err != nil {
		return nil, err
	}
	_, err = client.Do("SET", cartId, string(marshal))
	if err != nil {
		return nil, err
	}
	_, err = client.Do("EXPIRE", cartId, expireCartSeconds)
	strId := strconv.Itoa(int(newCart.Id))
	logger.WithFields(loggrus.Fields{"ID": newCart.Id, "Data": string(marshal), "Request": req.ArticleId}).Info("Created Cart")
	return &proto.ResponseCart{Id: strId}, nil
}
