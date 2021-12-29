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
	"github.com/Tobias-Pe/Microservices-Errorhandling/pkg/metrics"
	loggrus "github.com/sirupsen/logrus"
	"github.com/streadway/amqp"
	"math"
	"time"
)

const (
	methodListenSupply  = "SupplyRequest"
	methodPublishSupply = "PublishSupply"
)

var logger = loggingUtil.InitLogger()

type Service struct {
	AmqpChannel    *amqp.Channel
	AmqpConn       *amqp.Connection
	supplyMessages <-chan amqp.Delivery
	rabbitUrl      string
	requestsMetric *metrics.RequestsMetric
}

func NewService(rabbitAddress string, rabbitPort string) *Service {
	service := &Service{}
	service.rabbitUrl = fmt.Sprintf("amqp://guest:guest@%s:%s/", rabbitAddress, rabbitPort)
	var err error = nil
	// retry connecting to rabbitmq
	for i := 0; i < 6; i++ {
		err = service.initAmqpConnection()
		if err == nil {
			break
		}
		logger.Infof("Retrying... (%d/%d)", i, 5)
		time.Sleep(time.Duration(int64(math.Pow(2, float64(i)))) * time.Second)
	}
	if err != nil {
		return nil
	}
	err = service.createSupplyListener()
	if err != nil {
		return nil
	}

	service.requestsMetric = metrics.NewRequestsMetrics()

	return service
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
		return err
	}

	// supplyMessages will be where we get our supply messages from
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
		return err
	}
	return nil
}

// supplyArticles handles a restocking request and broadcasts the needed amount of an articleID
func (service *Service) supplyArticles(articleID string, amount int) error {
	bytes, err := json.Marshal(requests.StockSupplyMessage{
		ArticleID: articleID,
		Amount:    amount,
	})
	if err != nil {
		service.requestsMetric.Increment(err, methodPublishSupply)
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
		service.requestsMetric.Increment(err, methodPublishSupply)
		return err
	}

	logger.WithFields(loggrus.Fields{"article": articleID, "amount": amount}).Infof("Sent Supply.")
	service.requestsMetric.Increment(err, methodPublishSupply)

	return nil
}

// ListenSupplyRequests reads out supply messages from bound amqp queue
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
			service.requestsMetric.Increment(err, methodListenSupply)
			if err != nil {
				logger.WithError(err).Error("Could not ack message.")
			}
		} else {
			logger.WithError(err).Error("Could not unmarshall message")
			// ack message despite the error, or else we will get this message repeatedly
			err = message.Ack(false)
			if err != nil {
				logger.WithError(err).Error("Could not ack message.")
			}
			service.requestsMetric.Increment(err, methodListenSupply)
		}
	}
	logger.Warn("Stopped Listening for Supply Requests! Restarting...")
	// try reconnecting
	err := service.createSupplyListener()
	if err != nil {
		logger.WithError(err).Error("Stopped Listening for Supply Requests! Could not restart")
	} else {
		service.ListenSupplyRequests()
	}
}
