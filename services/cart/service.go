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
	"github.com/Tobias-Pe/Microservices-Errorhandling/api/proto"
	"github.com/Tobias-Pe/Microservices-Errorhandling/api/requests"
	loggingUtil "github.com/Tobias-Pe/Microservices-Errorhandling/pkg/log"
	"github.com/Tobias-Pe/Microservices-Errorhandling/pkg/models"
	"github.com/gomodule/redigo/redis"
	loggrus "github.com/sirupsen/logrus"
	"github.com/streadway/amqp"
	"math"
	"math/rand"
	"strconv"
	"time"
)

const expireCartSeconds = 172800

var logger = loggingUtil.InitLogger()

type Service struct {
	proto.UnimplementedCartServer
	connPool    *redis.Pool
	AmqpChannel *amqp.Channel
	AmqpConn    *amqp.Connection
	Queue       amqp.Queue
	messages    <-chan amqp.Delivery
	RabbitUrl   string
}

func NewService(cacheAddress string, cachePort string, rabbitAddress string, rabbitPort string) *Service {
	newService := Service{}
	newService.initRedisConnection(cacheAddress, cachePort)
	newService.RabbitUrl = fmt.Sprintf("amqp://guest:guest@%s:%s/", rabbitAddress, rabbitPort)
	var err error = nil
	for i := 0; i < 6; i++ {
		err = newService.initAmqpConnection()
		if err == nil {
			break
		}
		logger.Infof("Retrying... (%d/%d)", i, 5)
		time.Sleep(time.Duration(int64(math.Pow(2, float64(i)))) * time.Second)
	}
	if err != nil {
		return nil
	}
	return &newService
}

func (service *Service) initRedisConnection(cacheAddress string, cachePort string) {
	connectionUri := cacheAddress + ":" + cachePort
	service.connPool = &redis.Pool{
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
}

func (service *Service) initAmqpConnection() error {
	conn, err := amqp.Dial(service.RabbitUrl)
	if err != nil {
		logger.WithError(err).WithFields(loggrus.Fields{"url": service.RabbitUrl}).Error("Could not connect to rabbitMq")
		return err
	}
	service.AmqpConn = conn
	service.AmqpChannel, err = conn.Channel()
	if err != nil {
		logger.WithError(err).Error("Could not create channel")
		return err
	}
	err = service.AmqpChannel.ExchangeDeclare(
		requests.CartTopic, // name
		"topic",            // type
		true,               // durable
		false,              // auto-deleted
		false,              // internal
		false,              // no-wait
		nil,                // arguments
	)
	if err != nil {
		logger.WithError(err).Error("Could not create exchange")
		return err
	}
	q, err := service.AmqpChannel.QueueDeclare(
		requests.CartTopic+"_queue", // name
		true,                        // durable
		false,                       // delete when unused
		false,                       // exclusive
		false,                       // no-wait
		nil,                         // arguments
	)
	if err != nil {
		logger.WithError(err).Error("Could not create queue")
		return err
	}
	err = service.AmqpChannel.Qos(
		1,     // prefetch count
		0,     // prefetch size
		false, // global
	)
	if err != nil {
		logger.WithError(err).Error("Could not change qos settings")
		return err
	}
	err = service.AmqpChannel.QueueBind(
		q.Name,                       // queue name
		requests.AddToCartRoutingKey, // routing key
		requests.CartTopic,           // exchange
		false,
		nil,
	)
	if err != nil {
		logger.WithError(err).Error("Could not bind queue")
		return err
	}

	service.messages, err = service.AmqpChannel.Consume(
		q.Name, // queue
		"",     // consumer
		false,  // auto-ack
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)
	if err != nil {
		logger.WithError(err).Error("Could not consume queue")
		return err
	}

	go service.ListenUpdateCart()

	return nil
}

func (service *Service) ListenUpdateCart() {
	for message := range service.messages {
		request := &requests.PutArticleInCartRequest{}
		err := json.Unmarshal(message.Body, request)
		if err == nil {
			for i := 0; i < 10; i++ {
				cart, err := service.addToCart(request.CartID, request.ArticleID)
				if err != nil {
					logger.WithFields(loggrus.Fields{"request": request, "retry": i}).WithError(err).Warn("Could not add to cart. Trying again...")
					n := rand.Intn(600)
					time.Sleep(time.Duration(n) * time.Millisecond)
				} else {
					logger.WithFields(loggrus.Fields{"cart": cart}).Infof("Received a message")
					break
				}
			}
			if err != nil {
				logger.WithFields(loggrus.Fields{"request": request}).WithError(err).Error("Could not add to cart.")
			}
			err = message.Ack(false)
			if err != nil {
				logger.WithError(err).Error("Could not ack message. Rolling transaction back.")
				_, err := service.removeFromCart(request.CartID, request.ArticleID)
				if err != nil {
					logger.WithFields(loggrus.Fields{"request": request}).WithError(err).Error("Could not roll transaction back")
				}
			}
		} else {
			logger.WithError(err).Error("Could not unmarshall message")
		}
	}
	logger.Error("Stopped Listening for Update Cart! Restarting...")
	err := service.initAmqpConnection()
	if err != nil {
		logger.Error("Stopped Listening for Update Cart! Could not restart")
		return
	}
}

func (service Service) CreateCart(_ context.Context, req *proto.RequestNewCart) (*proto.ResponseNewCart, error) {
	cart, err := service.createCart(req.ArticleId)
	if err != nil {
		return nil, err
	}

	strId := strconv.Itoa(int(cart.ID))
	logger.WithFields(loggrus.Fields{"ID": cart.ID, "Data": cart.ArticleIDs, "Request": req.ArticleId}).Info("Created new Cart")
	return &proto.ResponseNewCart{CartId: strId}, nil
}

func (service Service) GetCart(_ context.Context, req *proto.RequestCart) (*proto.ResponseCart, error) {
	cart, err := service.getCart(req.CartId)
	if err != nil {
		return nil, err
	}
	logger.WithFields(loggrus.Fields{"ID": cart.ID, "Data": cart.ArticleIDs, "Request": req.CartId}).Info("Looked up Cart")
	return &proto.ResponseCart{ArticleIds: cart.ArticleIDs}, nil
}

func (service Service) getCart(strCartId string) (*models.Cart, error) {
	tmpId, err := strconv.Atoi(strCartId)
	if err != nil {
		return nil, err
	}
	id := int64(tmpId)
	fetchedCart := models.Cart{
		ID:         id,
		ArticleIDs: nil,
	}

	client := service.connPool.Get()
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

func (service Service) createCart(strArticleId string) (*models.Cart, error) {
	newCart := models.Cart{
		ID:         -1,
		ArticleIDs: []string{strArticleId},
	}

	client := service.connPool.Get()
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

func (service Service) addToCart(strCartId string, strArticleId string) (*models.Cart, error) {

	cartID, err := strconv.Atoi(strCartId)
	cart := models.Cart{ID: int64(cartID)}
	client := service.connPool.Get()
	defer func(client redis.Conn) {
		err := client.Close()
		if err != nil {
			logger.WithError(err).Warn("connection to redis could not be successfully closed")
		}
	}(client)
	_, err = client.Do("WATCH", cart.ID)
	if err != nil {
		return nil, err
	}
	jsonArticles, err := client.Do("GET", cart.ID)
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
	cart.ArticleIDs = append(articles, strArticleId)
	marshal, err := json.Marshal(cart.ArticleIDs)
	if err != nil {
		return nil, err
	}
	_, err = client.Do("MULTI")
	if err != nil {
		return nil, err
	}
	_, err = client.Do("SET", cart.ID, string(marshal))
	if err != nil {
		return nil, err
	}
	_, err = client.Do("EXPIRE", cart.ID, expireCartSeconds)
	if err != nil {
		return nil, err
	}
	_, err = client.Do("EXEC")
	if err != nil {
		logger.WithError(err).Error("error on exec")
		return nil, err
	}

	return &cart, nil
}

func (service Service) removeFromCart(strCartId string, strArticleId string) (*models.Cart, error) {
	cart, err := service.getCart(strCartId)
	if err != nil {
		return nil, err
	}
	for i, articleID := range cart.ArticleIDs {
		if articleID == strArticleId {
			cart.ArticleIDs = append(cart.ArticleIDs[:i], cart.ArticleIDs[i+1:]...)
			break
		}
	}

	client := service.connPool.Get()
	defer func(client redis.Conn) {
		err := client.Close()
		if err != nil {
			logger.WithError(err).Warn("connection to redis could not be successfully closed")
		}
	}(client)

	marshal, err := json.Marshal(cart.ArticleIDs)
	if err != nil {
		return nil, err
	}
	_, err = client.Do("SET", cart.ID, string(marshal))
	if err != nil {
		return nil, err
	}

	return cart, nil
}
