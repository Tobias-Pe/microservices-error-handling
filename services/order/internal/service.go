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
	"github.com/Tobias-Pe/Microservices-Errorhandling/pkg/metrics"
	"github.com/Tobias-Pe/Microservices-Errorhandling/pkg/models"
	loggrus "github.com/sirupsen/logrus"
	"github.com/streadway/amqp"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"math"
	"time"
)

var logger = loggingUtil.InitLogger()

const (
	methodCreateOrder   = "CreateOrder"
	methodGetOrder      = "GetOrder"
	methodListenPersist = "UpdateOrder"
	methodPublishOrder  = "PublishOrder"
)

type Service struct {
	proto.UnimplementedOrderServer
	Database       *DbConnection
	AmqpConn       *amqp.Connection
	AmqpChannel    *amqp.Channel
	rabbitUrl      string
	orderMessages  <-chan amqp.Delivery
	requestsMetric *metrics.RequestsMetric
}

func NewService(mongoAddress string, mongoPort string, rabbitAddress string, rabbitPort string) *Service {
	// init mongodb connection
	server := &Service{
		Database:  NewDbConnection(mongoAddress, mongoPort),
		rabbitUrl: fmt.Sprintf("amqp://guest:guest@%s:%s/", rabbitAddress, rabbitPort),
	}
	// retry connection to rabbitmq
	var err error = nil
	for i := 0; i < 6; i++ {
		err = server.initAmqpConnection()
		if err == nil {
			break
		}
		logger.Infof("Retrying... (%d/%d)", i, 5)
		time.Sleep(time.Duration(int64(math.Pow(2, float64(i)))) * time.Second)
	}
	if err != nil {
		return nil
	}

	err = server.createOrderListener()
	if err != nil {
		return nil
	}

	server.requestsMetric = metrics.NewRequestsMetrics()

	return server
}

// CreateOrder implementation of in the proto file defined interface of cart service
func (service *Service) CreateOrder(ctx context.Context, req *proto.RequestNewOrder) (*proto.OrderObject, error) {
	status := models.StatusFetching() // initial status of order
	order := models.Order{
		Status:             status.Name,
		Message:            status.Message,
		Articles:           nil,
		CartID:             req.CartId,
		Price:              -1,
		CustomerAddress:    req.CustomerAddress,
		CustomerName:       req.CustomerName,
		CustomerCreditCard: req.CustomerCreditCard,
		CustomerEmail:      req.CustomerEmail,
	}
	err := service.Database.createOrder(ctx, &order)
	if err != nil {
		service.requestsMetric.Increment(err, methodCreateOrder)
		return nil, err
	}

	logger.WithFields(loggrus.Fields{"Request": req, "Order": order}).Info("Order created")
	service.requestsMetric.Increment(err, methodCreateOrder)

	// broadcast order
	err = order.PublishOrderStatusUpdate(service.AmqpChannel)
	service.requestsMetric.Increment(err, methodPublishOrder)
	if err != nil {
		return nil, err
	}

	return &proto.OrderObject{
		OrderId:            order.ID.Hex(),
		Status:             order.Status,
		Message:            order.Message,
		Price:              float32(order.Price),
		CartId:             order.CartID,
		Articles:           order.Articles,
		CustomerAddress:    order.CustomerAddress,
		CustomerName:       order.CustomerName,
		CustomerCreditCard: order.CustomerCreditCard,
	}, nil
}

// GetOrder implementation of in the proto file defined interface of cart service
func (service *Service) GetOrder(ctx context.Context, req *proto.RequestOrder) (*proto.OrderObject, error) {
	// primitive.ObjectID type needed for mongodb
	orderId, err := primitive.ObjectIDFromHex(req.OrderId)
	if err != nil {
		service.requestsMetric.Increment(err, methodGetOrder)
		return nil, err
	}

	order, err := service.Database.getOrder(ctx, orderId)
	if err != nil {
		service.requestsMetric.Increment(err, methodGetOrder)
		return nil, err
	}

	logger.WithFields(loggrus.Fields{"Request": req, "Order": order}).Info("Order request handled")
	service.requestsMetric.Increment(err, methodGetOrder)

	return &proto.OrderObject{
		OrderId:            order.ID.Hex(),
		Status:             order.Status,
		Message:            order.Message,
		Price:              float32(order.Price),
		CartId:             order.CartID,
		Articles:           order.Articles,
		CustomerAddress:    order.CustomerAddress,
		CustomerName:       order.CustomerName,
		CustomerCreditCard: order.CustomerCreditCard,
	}, nil
}

func (service *Service) initAmqpConnection() error {
	conn, err := amqp.Dial(service.rabbitUrl)
	if err != nil {
		return err
	}

	// connection and channel will be closed in main
	service.AmqpConn = conn
	service.AmqpChannel, err = conn.Channel()
	if err != nil {
		return err
	}

	// prefetchCount 1 in QoS will load-balance messages between many instances of this service
	err = service.AmqpChannel.Qos(
		1,     // prefetch count
		0,     // prefetch size
		false, // global
	)
	if err != nil {
		return err
	}
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
		return err
	}
	q, err := service.AmqpChannel.QueueDeclare(
		"order_"+requests.OrderTopic+"_queue", // name
		true,  // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		return err
	}

	// listen to all statuses except the initial which is broadcast by this service
	bindings := []string{requests.OrderStatusReserve, requests.OrderStatusComplete, requests.OrderStatusShip, requests.OrderStatusPay, requests.OrderStatusAbort}
	for _, bindingKey := range bindings {
		// bind queue to all routing keys in bindings
		err = service.AmqpChannel.QueueBind(
			q.Name,              // queue name
			bindingKey,          // routing key
			requests.OrderTopic, // exchange
			false,
			nil,
		)
		if err != nil {
			return err
		}
	}

	// orderMessages will be where we will get our order updates from
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
		return err
	}
	// create coroutine to listen for order messages
	go service.ListenAndPersistOrders()
	return nil
}

// ListenAndPersistOrders reads out order messages from bound amqp queue
func (service *Service) ListenAndPersistOrders() {
	for message := range service.orderMessages {
		order := &models.Order{}
		err := json.Unmarshal(message.Body, order)
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*1)
			// fetch current order for case of rollback
			oldOrder, err := service.Database.getOrder(ctx, order.ID)
			oldOrderIsUpdated := false
			if err != nil {
				// order doesn't exist
				logger.WithFields(loggrus.Fields{"request": *order}).WithError(err).Warn("This order does not exist.")
			} else {
				// update order
				err := service.Database.updateOrder(ctx, *order)
				if err != nil {
					logger.WithFields(loggrus.Fields{"request": *order}).WithError(err).Warn("Could not update the order.")
				} else {
					logger.WithFields(loggrus.Fields{"request": *order}).Infof("Updated order.")
					// order was updated
					oldOrderIsUpdated = true
				}
			}
			cancel()
			err = message.Ack(false)
			if err != nil && oldOrderIsUpdated { // ack could not be sent but database transaction was successfully
				logger.WithError(err).Error("Could not ack message. Trying to roll back...")
				// rollback transaction. because of the missing ack the current request will be resent
				err := service.Database.updateOrder(ctx, *oldOrder)
				if err != nil {
					logger.WithError(err).Error("Could not rollback.")
				} else {
					logger.WithFields(loggrus.Fields{"order": oldOrder}).Info("Rolled transaction back.")
				}
			}
			service.requestsMetric.Increment(err, methodListenPersist)
		} else {
			logger.WithError(err).Error("Could not unmarshall message")
			// ack message despite the error, or else we will get this message repeatedly
			err = message.Ack(false)
			if err != nil {
				logger.WithError(err).Error("Could not ack message.")
			}
			service.requestsMetric.Increment(err, methodListenPersist)
		}
	}
	logger.Warn("Stopped Listening for Orders! Restarting...")
	// try to reconnect
	err := service.createOrderListener()
	if err != nil {
		logger.WithError(err).Error("Stopped Listening for Orders! Could not restart")
	}
}
