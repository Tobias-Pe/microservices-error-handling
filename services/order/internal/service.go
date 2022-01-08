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
	"errors"
	"fmt"
	"github.com/Tobias-Pe/Microservices-Errorhandling/api/proto"
	"github.com/Tobias-Pe/Microservices-Errorhandling/api/requests"
	customerrors "github.com/Tobias-Pe/Microservices-Errorhandling/pkg/custom-errors"
	loggingUtil "github.com/Tobias-Pe/Microservices-Errorhandling/pkg/log"
	"github.com/Tobias-Pe/Microservices-Errorhandling/pkg/metrics"
	"github.com/Tobias-Pe/Microservices-Errorhandling/pkg/models"
	"github.com/Tobias-Pe/Microservices-Errorhandling/pkg/rabbitmq"
	"github.com/sirupsen/logrus"
	"github.com/streadway/amqp"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
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
	rabbitmq.AmqpService
	Database       *DbConnection
	orderMessages  <-chan amqp.Delivery
	requestsMetric *metrics.RequestsMetric
	orderMetric    *metrics.OrdersMetric
}

func NewService(mongoAddress string, mongoPort string, rabbitAddress string, rabbitPort string) *Service {
	// init mongodb connection
	server := &Service{
		Database: NewDbConnection(mongoAddress, mongoPort),
	}
	server.RabbitURL = fmt.Sprintf("amqp://guest:guest@%s:%s/", rabbitAddress, rabbitPort)
	// retry connection to rabbitmq
	var err error = nil
	for i := 0; i < 6; i++ {
		err = server.InitAmqpConnection()
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
	server.orderMetric = metrics.NewOrdersMetric()

	return server
}

// CreateOrder implementation of in the proto file defined interface of cart service
func (service *Service) CreateOrder(ctx context.Context, req *proto.RequestNewOrder) (*proto.OrderObject, error) {
	if req == nil {
		return nil, customerrors.ErrRequestNil
	}

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
	service.requestsMetric.Increment(err, methodCreateOrder)
	if err != nil {
		return nil, err
	}

	logger.WithFields(logrus.Fields{"request": req, "response": order}).Info("Order created")

	// broadcast order
	err = order.PublishOrderStatusUpdate(service.AmqpChannel)
	service.requestsMetric.Increment(err, methodPublishOrder)
	if err != nil {
		return nil, err
	}

	service.orderMetric.Increment(order)
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
	if req == nil {
		return nil, customerrors.ErrRequestNil
	}
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

	logger.WithFields(logrus.Fields{"request": req, "response": order}).Info("Order request handled")
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

// createOrderListener creates and binds queue to listen for all orders except fetching (fetching status is created in this service)
func (service *Service) createOrderListener() error {
	var err error
	queueName := fmt.Sprintf("order_%s_queue", requests.OrderTopic)
	routingKeys := []string{requests.OrderStatusReserve, requests.OrderStatusComplete, requests.OrderStatusShip, requests.OrderStatusPay, requests.OrderStatusAbort}

	service.orderMessages, err = service.CreateListener(requests.OrderTopic, queueName, routingKeys)
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
		// unmarshall message into order
		order := &models.Order{}
		err := json.Unmarshal(message.Body, order)
		if err != nil {
			logger.WithError(err).Error("Could not unmarshall message")
			// ack message despite the error, or else we will get this message repeatedly
			ackErr := message.Ack(false)
			if ackErr != nil {
				logger.WithError(ackErr).Error("Could not ack message.")
				err = fmt.Errorf("%v ; %v", err.Error(), ackErr.Error())
			}
		} else {
			// unmarshall successfully --> handle the order
			err = service.handleOrder(order, message)
		}

		service.requestsMetric.Increment(err, methodListenPersist)
	}

	logger.Warn("Stopped Listening for Orders! Restarting...")
	// try to reconnect
	err := service.createOrderListener()
	if err != nil {
		logger.WithError(err).Error("Stopped Listening for Orders! Could not restart")
	}
}

func (service *Service) handleOrder(order *models.Order, message amqp.Delivery) error {
	// context for this whole transaction
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*1)
	defer cancel()

	// update order if its status is progressive. also, save old order for case of rollback
	oldOrder, err := service.Database.getAndUpdateOrder(ctx, *order)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) { //there was no such order or the order should not be updated
			// ack message & ignore error because rollback is not needed
			_ = message.Ack(false)
			logger.WithFields(logrus.Fields{"request": *order}).WithError(err).Warn("Could not update order.")
			return err
		} else if errors.Is(err, customerrors.ErrStatusProgressionConflict) { // the order should not be updated
			// ack message & ignore error because rollback is not needed
			_ = message.Ack(false)
			return nil // do not treat this is as an error because of asynchronous communication
		}
		_ = message.Reject(true) // nack and requeue message

		logger.WithFields(logrus.Fields{"request": *order}).WithError(err).Warn("Could not update order. Retrying...")

		return err
	}
	logger.WithFields(logrus.Fields{"request": *order}).Infof("Updated order.")

	// ack this transaction
	err = message.Ack(false)
	if err != nil { // ack could not be sent but database transaction was successfully
		logger.WithError(err).Error("Could not ack message. Trying to roll back...")
		// rollback transaction. because of the missing ack the current request will be resent
		rollbackErr := service.Database.updateOrder(ctx, *oldOrder)

		if rollbackErr != nil {
			service.orderMetric.Decrement(*oldOrder)
			service.orderMetric.Increment(*order)
			logger.WithError(rollbackErr).Error("Could not rollback.")
			err = fmt.Errorf("%v ; %v", err.Error(), rollbackErr.Error())
		} else {
			logger.WithFields(logrus.Fields{"response": *oldOrder}).Info("Rolled transaction back.")
		}
		return err
	}

	service.orderMetric.Decrement(*oldOrder)
	service.orderMetric.Increment(*order)

	return nil
}
