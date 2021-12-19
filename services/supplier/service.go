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

package supplier

import (
	"encoding/json"
	"fmt"
	"github.com/Tobias-Pe/Microservices-Errorhandling/api/requests"
	loggingUtil "github.com/Tobias-Pe/Microservices-Errorhandling/pkg/log"
	loggrus "github.com/sirupsen/logrus"
	"github.com/streadway/amqp"
	"math"
	"time"
)

var logger = loggingUtil.InitLogger()

type Service struct {
	AmqpChannel    *amqp.Channel
	AmqpConn       *amqp.Connection
	supplyMessages <-chan amqp.Delivery
	rabbitUrl      string
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
	err = s.createSupplyListener()
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
func (service *Service) createSupplyListener() error {
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
		"supplier_"+requests.ArticlesTopic+"_queue", // name
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
		q.Name,                          // queue name
		requests.StockRequestRoutingKey, // routing key
		requests.ArticlesTopic,          // exchange
		false,
		nil,
	)
	if err != nil {
		logger.WithError(err).Error("Could not bind queue")
		return err
	}

	service.supplyMessages, err = service.AmqpChannel.Consume(
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

func (service *Service) supplyArticles(articleID string, amount int) error {
	bytes, err := json.Marshal(requests.StockSupplyMessage{
		ArticleID: articleID,
		Amount:    amount,
	})
	if err != nil {
		return err
	}
	err = service.AmqpChannel.Publish(
		requests.ArticlesTopic,    // exchange
		requests.SupplyRoutingKey, // routing key
		true,                      // mandatory
		false,                     // immediate
		amqp.Publishing{
			DeliveryMode: amqp.Persistent,
			ContentType:  "application/json",
			Body:         bytes,
		})
	if err != nil {
		return err
	}
	logger.WithFields(loggrus.Fields{"article": articleID, "amount": amount}).Infof("Sent Supply.")
	return nil
}

func (service *Service) ListenSupplyRequests() {
	for message := range service.supplyMessages {
		request := &requests.StockSupplyMessage{}
		err := json.Unmarshal(message.Body, request)
		if err == nil {
			err := service.supplyArticles(request.ArticleID, request.Amount)
			if err != nil {
				logger.WithFields(loggrus.Fields{"request": *request}).WithError(err).Error("Could not supply articles.")
			}
			err = message.Ack(false)
			if err != nil {
				logger.WithError(err).Error("Could not ack message.")
			}
		} else {
			logger.WithError(err).Error("Could not unmarshall message")
			err = message.Ack(false)
			if err != nil {
				logger.WithError(err).Error("Could not ack message.")
			}
		}
	}
	logger.Error("Stopped Listening for Supply Requests! Restarting...")
	err := service.createSupplyListener()
	if err != nil {
		logger.Error("Stopped Listening for Supply Requests! Could not restart")
	} else {
		service.ListenSupplyRequests()
	}
}
