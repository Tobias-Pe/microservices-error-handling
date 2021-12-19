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
	"github.com/Tobias-Pe/Microservices-Errorhandling/pkg/models"
	loggrus "github.com/sirupsen/logrus"
	"github.com/streadway/amqp"
	"math"
	"math/rand"
	"net/mail"
	"time"
)

var logger = loggingUtil.InitLogger()

type Service struct {
	AmqpChannel   *amqp.Channel
	AmqpConn      *amqp.Connection
	orderMessages <-chan amqp.Delivery
	rabbitUrl     string
}

func NewService(rabbitAddress string, rabbitPort string) *Service {
	s := &Service{}
	s.rabbitUrl = fmt.Sprintf("amqp://guest:guest@%s:%s/", rabbitAddress, rabbitPort)
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
		"email_"+requests.OrderTopic+"_queue", // name
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
	bindings := []string{requests.OrderStatusComplete, requests.OrderStatusAbort}
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
	return nil
}

func (service *Service) mockEmail(customerEMail string) bool {
	_, err := mail.ParseAddress(customerEMail)
	if err != nil {
		return false
	}
	timeout := rand.Intn(500)
	time.Sleep(time.Duration(timeout) * time.Millisecond)
	return true
}

func (service *Service) mockEmailRollback() {
	timeout := rand.Intn(500)
	time.Sleep(time.Duration(timeout) * time.Millisecond)
}

func (service *Service) ListenOrders() {
	for message := range service.orderMessages {
		order := &models.Order{}
		err := json.Unmarshal(message.Body, order)
		if err == nil {
			isAllowed := service.mockEmail(order.CustomerEmail)
			err = message.Ack(false)
			if err != nil {
				logger.WithError(err).Error("Could not ack message.")
				logger.WithFields(loggrus.Fields{"request": *order}).WithError(err).Info("Rolling back transaction...")
				service.mockEmailRollback()
				logger.WithFields(loggrus.Fields{"request": *order}).Info("Rolling back successfully")
			} else {
				if !isAllowed {
					logger.WithFields(loggrus.Fields{"email_status": isAllowed, "request": *order}).Error("Email not sent.")
				} else {
					logger.WithFields(loggrus.Fields{"request": *order}).Infof("E-Mail sent.")
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