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
	loggrus "github.com/sirupsen/logrus"
	"github.com/streadway/amqp"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"math"
	"time"
)

var logger = loggingUtil.InitLogger()

type Service struct {
	proto.UnimplementedOrderServer
	Database      *DbConnection
	AmqpConn      *amqp.Connection
	AmqpChannel   *amqp.Channel
	rabbitUrl     string
	orderMessages <-chan amqp.Delivery
}

func NewService(mongoAddress string, mongoPort string, rabbitAddress string, rabbitPort string) *Service {
	// init mongo connection
	s := &Service{
		Database:  NewDbConnection(mongoAddress, mongoPort),
		rabbitUrl: fmt.Sprintf("amqp://guest:guest@%s:%s/", rabbitAddress, rabbitPort),
	}
	// init amqp connection
	var err error = nil
	for i := 0; i < 6; i++ {
		err = s.initAmqpConnection()
		if err == nil {
			break
		}
		logger.Infof("Retrying... (%d/%d)", i, 5)
		time.Sleep(time.Duration(int64(math.Pow(2, float64(i)))) * time.Second)
	}
	if err != nil {
		return nil
	}
	err = s.createOrderListener()
	if err != nil {
		return nil
	}
	return s
}

func (service *Service) CreateOrder(ctx context.Context, req *proto.RequestNewOrder) (*proto.OrderObject, error) {
	status := models.StatusFetching()
	order := models.Order{
		Status:             status.Name,
		Message:            status.Message,
		Articles:           nil,
		CartID:             req.CartId,
		Price:              -1,
		CustomerAddress:    req.CustomerAddress,
		CustomerName:       req.CustomerName,
		CustomerCreditCard: req.CustomerCreditCard,
	}
	err := service.Database.createOrder(ctx, &order)
	if err != nil {
		return nil, err
	}
	logger.WithFields(loggrus.Fields{"Request": req, "Order": order}).Info("Order created")
	err = order.PublishOrderStatusUpdate(service.AmqpChannel)
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

func (service *Service) GetOrder(ctx context.Context, req *proto.RequestOrder) (*proto.OrderObject, error) {
	orderId, err := primitive.ObjectIDFromHex(req.OrderId)
	if err != nil {
		return nil, err
	}
	order, err := service.Database.getOrder(ctx, orderId)
	if err != nil {
		return nil, err
	}
	logger.WithFields(loggrus.Fields{"Request": req, "Order": order}).Info("Order request handled")
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
		"order_"+requests.OrderTopic+"_queue", // name
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
	bindings := []string{requests.OrderStatusReserve, requests.OrderStatusComplete, requests.OrderStatusShip, requests.OrderStatusPay, requests.OrderStatusAbort}
	for _, bindingKey := range bindings {
		err = service.AmqpChannel.QueueBind(
			q.Name,              // queue name
			bindingKey,          // routing key
			requests.OrderTopic, // exchange
			false,
			nil,
		)
		if err != nil {
			logger.WithError(err).Error("Could not bind queue")
			return err
		}
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

	go service.ListenAndPersistOrders()
	return nil
}

func (service *Service) ListenAndPersistOrders() {
	for message := range service.orderMessages {
		order := &models.Order{}
		err := json.Unmarshal(message.Body, order)
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*1)
			oldOrder, err := service.Database.getOrder(ctx, order.ID)
			oldOrderIsUpdated := false
			if err != nil {
				logger.WithFields(loggrus.Fields{"request": *order}).WithError(err).Error("This order does not exist.")
			} else {
				err := service.Database.updateOrder(ctx, *order)
				if err != nil {
					logger.WithFields(loggrus.Fields{"request": *order}).WithError(err).Error("Could not update the order.")
				} else {
					logger.WithFields(loggrus.Fields{"request": *order}).Infof("Updated order.")
					oldOrderIsUpdated = true
				}
			}
			cancel()
			err = message.Ack(false)
			if err != nil && oldOrderIsUpdated {
				logger.WithError(err).Error("Could not ack message. Trying to roll back...")
				err := service.Database.updateOrder(ctx, *oldOrder)
				if err != nil {
					logger.WithError(err).Error("Could not rollback.")
				} else {
					logger.WithFields(loggrus.Fields{"order": oldOrder}).Error("Rolled transaction back.")
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
