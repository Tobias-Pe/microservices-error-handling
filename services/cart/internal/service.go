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
	loggingUtil "github.com/Tobias-Pe/Microservices-Errorhandling/pkg/log"
	"github.com/Tobias-Pe/Microservices-Errorhandling/pkg/models"
	"github.com/gomodule/redigo/redis"
	loggrus "github.com/sirupsen/logrus"
	"github.com/streadway/amqp"
	"math"
	"strconv"
	"time"
)

const expireCartSeconds = 172800

var logger = loggingUtil.InitLogger()

type Service struct {
	proto.UnimplementedCartServer
	AmqpChannel     *amqp.Channel
	AmqpConn        *amqp.Connection
	orderMessages   <-chan amqp.Delivery
	rabbitUrl       string
	database        *DbConnection
	articleMessages <-chan amqp.Delivery
}

func NewService(cacheAddress string, cachePort string, rabbitAddress string, rabbitPort string) *Service {
	newService := Service{}
	newService.initRedisConnection(cacheAddress, cachePort)
	newService.rabbitUrl = fmt.Sprintf("amqp://guest:guest@%s:%s/", rabbitAddress, rabbitPort)
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
	err = newService.createArticleListener()
	if err != nil {
		return nil
	}
	err = newService.createOrderListener()
	if err != nil {
		return nil
	}
	return &newService
}

func (service *Service) initRedisConnection(cacheAddress string, cachePort string) {
	connectionUri := cacheAddress + ":" + cachePort
	service.database = &DbConnection{}
	service.database.connPool = &redis.Pool{
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
	conn, err := amqp.Dial(service.rabbitUrl)
	if err != nil {
		logger.WithError(err).WithFields(loggrus.Fields{"url": service.rabbitUrl}).Error("Could not connect to rabbitMq")
		return err
	}
	service.AmqpConn = conn
	service.AmqpChannel, err = conn.Channel()
	if err != nil {
		logger.WithError(err).Error("Could not create channel")
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
	return nil
}

func (service *Service) createArticleListener() error {
	err := service.AmqpChannel.ExchangeDeclare(
		requests.ArticlesTopic, // name
		"topic",                // type
		true,                   // durable
		false,                  // auto-deleted
		false,                  // internal
		false,                  // no-wait
		nil,                    // arguments
	)
	if err != nil {
		logger.WithError(err).Error("Could not create exchange")
		return err
	}
	q, err := service.AmqpChannel.QueueDeclare(
		"cart_"+requests.ArticlesTopic+"_queue", // name
		true,  // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		logger.WithError(err).Error("Could not create queue")
		return err
	}
	err = service.AmqpChannel.QueueBind(
		q.Name,                       // queue name
		requests.AddToCartRoutingKey, // routing key
		requests.ArticlesTopic,       // exchange
		false,
		nil,
	)
	if err != nil {
		logger.WithError(err).Error("Could not bind queue")
		return err
	}

	service.articleMessages, err = service.AmqpChannel.Consume(
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
func (service *Service) createOrderListener() error {
	err := service.AmqpChannel.ExchangeDeclare(
		requests.OrderTopic, // name
		"topic",             // type
		true,                // durable
		false,               // auto-deleted
		false,               // internal
		false,               // no-wait
		nil,                 // arguments
	)
	if err != nil {
		logger.WithError(err).Error("Could not create exchange")
		return err
	}
	q, err := service.AmqpChannel.QueueDeclare(
		"cart_"+requests.OrderTopic+"_queue", // name
		true,                                 // durable
		false,                                // delete when unused
		false,                                // exclusive
		false,                                // no-wait
		nil,                                  // arguments
	)
	if err != nil {
		logger.WithError(err).Error("Could not create queue")
		return err
	}
	err = service.AmqpChannel.QueueBind(
		q.Name,                    // queue name
		requests.OrderStatusFetch, // routing key
		requests.OrderTopic,       // exchange
		false,
		nil,
	)
	if err != nil {
		logger.WithError(err).Error("Could not bind queue")
		return err
	}

	service.orderMessages, err = service.AmqpChannel.Consume(
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

	go service.ListenOrders()
	return nil
}

func (service *Service) ListenUpdateCart() {
	for message := range service.articleMessages {
		request := &requests.PutArticleInCartRequest{}
		err := json.Unmarshal(message.Body, request)
		if err == nil {
			index, err := service.database.addToCart(request.CartID, request.ArticleID)
			if err != nil {
				logger.WithFields(loggrus.Fields{"request": request}).WithError(err).Error("Could not add to cart.")
			} else {
				logger.WithFields(loggrus.Fields{"index": *index, "request": request}).Infof("Inserted item")
			}
			err = message.Ack(false)
			if err != nil && index != nil {
				logger.WithError(err).Error("Could not ack message. Rolling transaction back.")
				err = service.database.removeFromCart(request.CartID, *index)
				if err != nil {
					logger.WithFields(loggrus.Fields{"request": request}).WithError(err).Error("Could not roll transaction back")
				}
			}
		} else {
			logger.WithError(err).Error("Could not unmarshall message")
		}
	}
	logger.Error("Stopped Listening for Articles! Restarting...")
	err := service.createArticleListener()
	if err != nil {
		logger.Error("Stopped Listening for Articles! Could not restart")
	}
}
func (service *Service) ListenOrders() {
	for message := range service.orderMessages {
		order := &models.Order{}
		err := json.Unmarshal(message.Body, order)
		if err == nil {
			cart, cartErr := service.database.getCart(order.CartID)
			err = message.Ack(false)
			if err != nil {
				logger.WithError(err).Error("Could not ack message.")
			} else {
				if cartErr != nil {
					logger.WithFields(loggrus.Fields{"request": *order}).WithError(err).Error("Could not get cart from this order. Aborting order...")
					status := models.StatusAborted("We could not fetch your cart. Check your cart's ID again.")
					order.Status = status.Name
					order.Message = status.Message
				} else {
					logger.WithFields(loggrus.Fields{"cart": cart, "request": *order}).Infof("Got cart for this order.")
					order.Articles = cart.ArticleIDs
					status := models.StatusReserving()
					order.Status = status.Name
					order.Message = status.Message
				}
				err = order.PublishOrderStatusUpdate(service.AmqpChannel)
				if err != nil {
					logger.WithFields(loggrus.Fields{"cart": cart, "request": *order}).WithError(err).Error("Could not publish order update")
				}
			}
		} else {
			logger.WithError(err).Error("Could not unmarshall message")
		}
	}
	logger.Error("Stopped Listening for Orders! Restarting...")
	err := service.createOrderListener()
	if err != nil {
		logger.Error("Stopped Listening for Orders! Could not restart")
	}
}

func (service *Service) CreateCart(_ context.Context, req *proto.RequestNewCart) (*proto.ResponseNewCart, error) {
	cart, err := service.database.createCart(req.ArticleId)
	if err != nil {
		return nil, err
	}

	strId := strconv.Itoa(int(cart.ID))
	logger.WithFields(loggrus.Fields{"ID": cart.ID, "Data": cart.ArticleIDs, "Request": req.ArticleId}).Info("Created new Cart")
	return &proto.ResponseNewCart{CartId: strId}, nil
}

func (service *Service) GetCart(_ context.Context, req *proto.RequestCart) (*proto.ResponseCart, error) {
	cart, err := service.database.getCart(req.CartId)
	if err != nil {
		return nil, err
	}
	logger.WithFields(loggrus.Fields{"ID": cart.ID, "Data": cart.ArticleIDs, "Request": req.CartId}).Info("Looked up Cart")
	return &proto.ResponseCart{ArticleIds: cart.ArticleIDs}, nil
}
