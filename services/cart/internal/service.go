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

// expireCartSeconds Seconds until the cart will expire after no updates
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

// initRedisConnection creates a connection to redis and makes an object (DbConnection) to interact with redis
func (service *Service) initRedisConnection(cacheAddress string, cachePort string) {
	connectionUri := cacheAddress + ":" + cachePort
	service.database = &DbConnection{}
	// connection pool to redis
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

// initAmqpConnection connects to rabbitmq
func (service *Service) initAmqpConnection() error {
	conn, err := amqp.Dial(service.rabbitUrl)
	if err != nil {
		logger.WithError(err).WithFields(loggrus.Fields{"url": service.rabbitUrl}).Error("Could not connect to rabbitMq")
		return err
	}
	// connection and channel close functions will be called in main
	service.AmqpConn = conn
	service.AmqpChannel, err = conn.Channel()
	if err != nil {
		logger.WithError(err).Error("Could not create channel")
		return err
	}
	// prefetchCount 1 in QoS will load-balance messages between many instances of this service
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

// createArticleListener initialises exchange and queue and binds the queue to a topic and routing key to listen from
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
	// articleMessages will be where we will get our messages from
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
	// make a coroutine which will listen for article messages
	go service.ListenUpdateCart()
	return nil
}

// createOrderListener initialises exchange and queue and binds the queue to a topic and routing key to listen from
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
	// orderMessages is where we will get our order messages from
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
	// create a coroutine to listen for order messages
	go service.ListenOrders()
	return nil
}

// ListenUpdateCart reads out article messages from bound amqp queue
func (service *Service) ListenUpdateCart() {
	for message := range service.articleMessages {
		request := &requests.PutArticleInCartRequest{}
		err := json.Unmarshal(message.Body, request)
		if err == nil {
			// add the requested item into the cart
			index, err := service.database.addToCart(request.CartID, request.ArticleID)
			if err != nil {
				logger.WithFields(loggrus.Fields{"request": request}).WithError(err).Warn("Could not add to cart.")
			} else {
				logger.WithFields(loggrus.Fields{"index": *index, "request": request}).Infof("Inserted item")
			}
			err = message.Ack(false)
			if err != nil && index != nil { // ack could not be sent but database transaction was successfully
				logger.WithError(err).Error("Could not ack message. Trying to roll back...")
				// rollback transaction because of the missing ack the current request will be resent
				err = service.database.removeFromCart(request.CartID, *index)
				if err != nil {
					logger.WithError(err).Error("Could not rollback.")
				} else {
					logger.WithFields(loggrus.Fields{"request": request}).Info("Rolled transaction back.")
				}
			}
		} else {
			logger.WithError(err).Error("Could not unmarshall message")
			// ack message despite the error, or else we will get this message repeatedly
			err = message.Ack(false)
			if err != nil {
				logger.WithError(err).Error("Could not ack message.")
			}
		}
	}
	logger.Warn("Stopped Listening for Articles! Restarting...")
	err := service.createArticleListener()
	if err != nil {
		logger.Warn("Stopped Listening for Articles! Could not restart")
	}
}

// ListenOrders reads out order messages from bound amqp queue
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
				if cartErr != nil { // there was no such cart in this order --> abort order because wrong information
					logger.WithFields(loggrus.Fields{"request": *order}).WithError(err).Warn("Could not get cart from this order. Aborting order...")
					status := models.StatusAborted("We could not fetch your cart. Check your cart's ID again.")
					order.Status = status.Name
					order.Message = status.Message
				} else { // next step for the order is reserving the articles from the cart in stock
					logger.WithFields(loggrus.Fields{"cart": cart, "request": *order}).Infof("Got cart for this order.")
					order.Articles = cart.ArticleIDs
					status := models.StatusReserving()
					order.Status = status.Name
					order.Message = status.Message
				}
				// broadcast the order update
				err = order.PublishOrderStatusUpdate(service.AmqpChannel)
				if err != nil {
					logger.WithFields(loggrus.Fields{"cart": cart, "request": *order}).WithError(err).Error("Could not publish order update")
				}
			}
		} else {
			logger.WithError(err).Error("Could not unmarshall message")
			// ack message despite the error, or else we will get this message repeatedly
			err = message.Ack(false)
			if err != nil {
				logger.WithError(err).Error("Could not ack message.")
			}
		}
	}
	logger.Warn("Stopped Listening for Orders! Restarting...")
	err := service.createOrderListener()
	if err != nil {
		logger.Error("Stopped Listening for Orders! Could not restart")
	}
}

// CreateCart implementation of in the proto file defined interface of cart service
func (service *Service) CreateCart(_ context.Context, req *proto.RequestNewCart) (*proto.ResponseNewCart, error) {
	cart, err := service.database.createCart(req.ArticleId)
	if err != nil {
		return nil, err
	}

	strId := strconv.Itoa(int(cart.ID))
	logger.WithFields(loggrus.Fields{"ID": cart.ID, "Data": cart.ArticleIDs, "Request": req.ArticleId}).Info("Created new Cart")
	return &proto.ResponseNewCart{CartId: strId}, nil
}

// GetCart implementation of in the proto file defined interface of cart service
func (service *Service) GetCart(_ context.Context, req *proto.RequestCart) (*proto.ResponseCart, error) {
	cart, err := service.database.getCart(req.CartId)
	if err != nil {
		return nil, err
	}
	logger.WithFields(loggrus.Fields{"ID": cart.ID, "Data": cart.ArticleIDs, "Request": req.CartId}).Info("Looked up Cart")
	return &proto.ResponseCart{ArticleIds: cart.ArticleIDs}, nil
}
