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

package payment

import (
	"encoding/json"
	"fmt"
	"github.com/Tobias-Pe/Microservices-Errorhandling/api/requests"
	loggingUtil "github.com/Tobias-Pe/Microservices-Errorhandling/pkg/log"
	"github.com/Tobias-Pe/Microservices-Errorhandling/pkg/metrics"
	"github.com/Tobias-Pe/Microservices-Errorhandling/pkg/models"
	"github.com/Tobias-Pe/Microservices-Errorhandling/pkg/rabbitmq"
	"github.com/sirupsen/logrus"
	"github.com/streadway/amqp"
	"math"
	"math/rand"
	"strings"
	"time"
)

const (
	methodListenOrders = "PayOrder"
	methodPublishOrder = "PublishOrder"
)

var logger = loggingUtil.InitLogger()

type Service struct {
	rabbitmq.AmqpService
	orderMessages  <-chan amqp.Delivery
	requestsMetric *metrics.RequestsMetric
}

func NewService(rabbitAddress string, rabbitPort string) *Service {
	service := &Service{}
	service.RabbitURL = fmt.Sprintf("amqp://guest:guest@%s:%s/", rabbitAddress, rabbitPort)
	var err error = nil
	// retry connecting to rabbitmq
	for i := 0; i < 6; i++ {
		err = service.InitAmqpConnection()
		if err == nil {
			break
		}
		logger.Infof("Retrying... (%d/%d)", i, 5)
		time.Sleep(time.Duration(int64(math.Pow(2, float64(i)))) * time.Second)
	}
	if err != nil {
		return nil
	}

	err = service.createOrderListener()
	if err != nil {
		return nil
	}

	service.requestsMetric = metrics.NewRequestsMetrics()

	return service
}

// createOrderListener creates and binds queue to listen for orders in status paying
func (service *Service) createOrderListener() error {
	var err error
	queueName := fmt.Sprintf("payment_%s_queue", requests.OrderTopic)
	routingKeys := []string{requests.OrderStatusPay}

	service.orderMessages, err = service.CreateListener(requests.OrderTopic, queueName, routingKeys)
	if err != nil {
		return err
	}

	return nil
}

// mockPayment simulates the paying process. Validates creditCart string and then sleeps random to simulate working
func (service *Service) mockPayment(creditCard string) (bool, error) {
	creditCard = strings.ToLower(strings.TrimSpace(creditCard))
	// simulate work: sleep for every char in creditCard randomly
	for _, charVariable := range creditCard {
		timeout := rand.Intn(5)
		time.Sleep(time.Duration(timeout) * time.Millisecond)
		// validate for only numbers and special symbols
		if charVariable >= 'a' && charVariable <= 'z' {
			return false, fmt.Errorf("credit card invalid format")
		}
	}
	return true, nil
}

// mockPaymentRollback simulates undoing the payment process
func (service *Service) mockPaymentRollback(creditCard string) {
	creditCard = strings.ToLower(strings.TrimSpace(creditCard))
	// simulate work: sleep for every char in creditCard randomly
	for _, charVariable := range creditCard {
		timeout := rand.Intn(5)
		time.Sleep(time.Duration(timeout) * time.Millisecond)
		if charVariable >= 'a' && charVariable <= 'z' {
			return
		}
	}
}

// ListenOrders reads out order messages from bound amqp queue
func (service *Service) ListenOrders() {
	for message := range service.orderMessages {
		// unmarshall message into order
		order := &models.Order{}
		err := json.Unmarshal(message.Body, order)
		if err == nil {
			err = service.handleOrder(order, message)
		} else {
			logger.WithError(err).Error("Could not unmarshall message")
			// ack message despite the error, or else we will get this message repeatedly
			ackErr := message.Ack(false)
			if ackErr != nil {
				logger.WithError(ackErr).Error("Could not ack message.")
				err = fmt.Errorf("%v ; %v", err.Error(), ackErr.Error())
			}
		}
		service.requestsMetric.Increment(err, methodListenOrders)
	}

	logger.Warn("Stopped Listening for Orders! Restarting...")
	// try reconnecting
	err := service.createOrderListener()
	if err != nil {
		logger.WithError(err).Error("Stopped Listening for Orders! Could not restart")
	} else {
		service.ListenOrders()
	}
}

func (service *Service) handleOrder(order *models.Order, message amqp.Delivery) error {
	isAllowed, err := service.mockPayment(order.CustomerCreditCard)
	if !isAllowed { // abort order because of invalid payment data
		logger.WithFields(logrus.Fields{"payment_status": isAllowed, "request": *order}).WithError(err).Warn("Payment unsuccessfully. Aborting order...")
		status := models.StatusAborted("We could not get the needed amount from your credit card. Please check your account.")
		order.Status = status.Name
		order.Message = status.Message
	} else {
		logger.WithFields(logrus.Fields{"request": *order}).Infof("Order payed.")
		status := models.StatusShipping()
		order.Status = status.Name
		order.Message = status.Message
	}

	// broadcast updated order
	err = order.PublishOrderStatusUpdate(service.AmqpChannel)
	service.requestsMetric.Increment(err, methodPublishOrder)
	if err != nil {
		logger.WithFields(logrus.Fields{"request": *order}).WithError(err).Error("Could not publish order update")
		// randomize the requeueing
		go func() {
			sleepMult := rand.Intn(500)
			time.Sleep(time.Millisecond * time.Duration(sleepMult))
			_ = message.Reject(true) // nack and requeue message
		}()
		return err
	}

	err = message.Ack(false)
	if err != nil { // ack could not be sent but transaction was successfully
		logger.WithError(err).Error("Could not ack message. Trying to roll back...")

		logger.WithFields(logrus.Fields{"request": *order}).WithError(err).Info("Rolling back transaction...")
		// rollback transaction. because of the missing ack the current request will be resent
		service.mockPaymentRollback(order.CustomerCreditCard)
		logger.WithFields(logrus.Fields{"request": *order}).Info("Rolling back successfully")
	}

	return err
}
