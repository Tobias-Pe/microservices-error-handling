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

package email

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
	"net/mail"
	"os"
	"time"
)

var logger = loggingUtil.InitLogger()

const (
	methodListenOrders = "SendEmail"
)

type Service struct {
	rabbitmq.AmqpService
	orderMessages  <-chan amqp.Delivery
	requestsMetric *metrics.RequestsMetric
}

func NewService(rabbitAddress string, rabbitPort string) *Service {
	service := &Service{}

	service.RabbitURL = fmt.Sprintf("amqp://guest:guest@%s:%s/", rabbitAddress, rabbitPort)
	var err error = nil
	// retry connection to rabbitmq
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

// createOrderListener creates and binds queue to listen for completed and aborted orders
func (service *Service) createOrderListener() error {
	var err error
	queueName := fmt.Sprintf("email_%s_queue", requests.OrderTopic)
	routingKeys := []string{requests.OrderStatusComplete, requests.OrderStatusAbort}

	service.orderMessages, err = service.CreateListener(requests.OrderTopic, queueName, routingKeys)
	if err != nil {
		return err
	}

	return nil
}

// mockEmail simulates sending an email through waiting a random time
func (service *Service) mockEmail(customerEMail string) error {
	// validate email
	_, err := mail.ParseAddress(customerEMail)
	if err != nil {
		return fmt.Errorf("Email-Address not valid")
	}
	// random waiting to simulate work
	timeout := rand.Intn(20)
	time.Sleep(time.Duration(timeout) * time.Millisecond)
	return nil
}

// mockEmailRollback simulates trying to get the email back after sending it (email could be queued)
func (service *Service) mockEmailRollback() {
	// random waiting to simulate work
	timeout := rand.Intn(20)
	time.Sleep(time.Duration(timeout) * time.Millisecond)
}

// ListenOrders reads out order messages from bound amqp queue
func (service *Service) ListenOrders() {
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

			service.requestsMetric.Increment(err, methodListenOrders)
		} else {
			// unmarshall successfully --> handle the order
			err = service.handleOrder(order, message)

			service.requestsMetric.Increment(err, methodListenOrders)
		}
	}

	logger.Warn("Stopped Listening for Orders! Restarting...")
	// reconnect
	err := service.createOrderListener()
	if err != nil {
		logger.WithError(err).Error("Stopped Listening for Orders! Could not restart")
		os.Exit(1)
	} else {
		// reconnection successfully --> start listening again
		service.ListenOrders()
	}
}

func (service *Service) handleOrder(order *models.Order, message amqp.Delivery) error {
	emailErr := service.mockEmail(order.CustomerEmail)

	if emailErr != nil {
		logger.WithFields(logrus.Fields{"request": *order}).WithError(emailErr).Warn("Email not sent.")

		// ack message & ignore error because rollback is not needed
		_ = message.Ack(false)

		return emailErr
	}

	logger.WithFields(logrus.Fields{"request": *order}).Infof("E-Mail sent.")

	ackErr := message.Ack(false)
	if ackErr != nil {
		// ack could not be sent but email transaction was successfully
		logger.WithFields(logrus.Fields{"request": *order}).WithError(ackErr).Info("Rolling back transaction...")
		// rollback transaction. because of the missing ack the current request will be resent
		service.mockEmailRollback()
		logger.WithFields(logrus.Fields{"request": *order}).Info("Rolling back successfully")
	}

	return ackErr

}
