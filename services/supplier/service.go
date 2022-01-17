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
	"github.com/Tobias-Pe/Microservices-Errorhandling/pkg/rabbitmq"
	"github.com/sirupsen/logrus"
	"github.com/streadway/amqp"
	"math"
	"os"
	"time"
)

const (
	methodListenSupply  = "SupplyRequest"
	methodPublishSupply = "PublishSupply"
)

var logger = loggingUtil.InitLogger()

type Service struct {
	rabbitmq.AmqpService
	supplyMessages <-chan amqp.Delivery
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

	err = service.createSupplyListener()
	if err != nil {
		return nil
	}

	service.requestsMetric = metrics.NewRequestsMetrics()

	return service
}

// createSupplyListener initialises exchange and queue and binds the queue to a topic and routing key to listen from
func (service *Service) createSupplyListener() error {
	var err error
	queueName := fmt.Sprintf("supplier_%s_queue", requests.ArticlesTopic)
	routingKeys := []string{requests.StockRequestRoutingKey}

	service.supplyMessages, err = service.CreateListener(requests.ArticlesTopic, queueName, routingKeys)
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
	service.requestsMetric.Increment(err, methodPublishSupply)
	if err != nil {
		return err
	}

	logger.WithFields(logrus.Fields{"request": articleID, "response": fmt.Sprint(amount)}).Infof("Sent Supply.")

	return nil
}

// ListenSupplyRequests reads out supply messages from bound amqp queue
func (service *Service) ListenSupplyRequests() {
	for message := range service.supplyMessages {
		// unmarshall message into StockSupplyMessage
		request := &requests.StockSupplyMessage{}
		err := json.Unmarshal(message.Body, request)
		if err == nil {
			err = service.supplyArticles(request.ArticleID, request.Amount)
			if err != nil {
				logger.WithFields(logrus.Fields{"request": *request}).WithError(err).Error("Could not supply articles.")
			}

			ackErr := message.Ack(false)
			if ackErr != nil {
				// no need to rollback transaction
				logger.WithError(ackErr).Error("Could not ack message.")
				err = fmt.Errorf("%v ; %v", err.Error(), ackErr.Error())
			}

		} else {
			logger.WithError(err).Error("Could not unmarshall message")
			// ack message despite the error, or else we will get this message repeatedly
			err = message.Ack(false)
			if err != nil {
				logger.WithError(err).Error("Could not ack message.")
			}
		}
		service.requestsMetric.Increment(err, methodListenSupply)
	}

	logger.Warn("Stopped Listening for Supply Requests! Restarting...")
	// try reconnecting
	err := service.createSupplyListener()
	if err != nil {
		logger.WithError(err).Error("Stopped Listening for Supply Requests! Could not restart")
		os.Exit(1)
	} else {
		service.ListenSupplyRequests()
	}
}
