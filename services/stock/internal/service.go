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
	"math"
	"time"
)

var logger = loggingUtil.InitLogger()

type Service struct {
	proto.UnimplementedStockServer
	Database      *DbConnection
	AmqpChannel   *amqp.Channel
	AmqpConn      *amqp.Connection
	Queue         amqp.Queue
	orderMessages <-chan amqp.Delivery
	rabbitUrl     string
}

func NewService(mongoAddress string, mongoPort string, rabbitAddress string, rabbitPort string) *Service {
	s := &Service{Database: NewDbConnection(mongoAddress, mongoPort)}
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
		"stock_"+requests.OrderTopic+"_queue", // name
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
		q.Name,                      // queue name
		requests.OrderStatusReserve, // routing key
		requests.OrderTopic,         // exchange
		false,
		nil,
	)
	if err != nil {
		logger.WithError(err).Error("Could not bind queue")
		return err
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

	go service.ListenOrders()
	return nil
}

func (service *Service) GetArticles(ctx context.Context, req *proto.RequestArticles) (*proto.ResponseArticles, error) {
	articles, err := service.Database.getArticles(ctx, req.CategoryQuery)
	if err != nil {
		return nil, err
	}

	var protoArticles []*proto.Article
	for _, a := range *articles {
		protoArticles = append(protoArticles, &proto.Article{
			Id:        a.ID.Hex(),
			Name:      a.Name,
			Category:  a.Category,
			PriceEuro: float32(a.Price),
			Amount:    a.Amount,
		})
	}

	logger.WithFields(loggrus.Fields{"articles": articles}).Info("Get articles handled")
	return &proto.ResponseArticles{Articles: protoArticles}, nil
}

func (service *Service) ListenOrders() {
	for message := range service.orderMessages {
		order := &models.Order{}
		err := json.Unmarshal(message.Body, order)
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*1)
			price, reservationErr := service.Database.reserveOrder(ctx, *order)
			cancel()
			err = message.Ack(false)
			if err != nil {
				logger.WithError(err).Error("Could not ack message.")
				if reservationErr != nil {
					logger.WithFields(loggrus.Fields{"request": *order}).WithError(err).Info("Rolling back transaction...")
					_, err = service.Database.rollbackReserveOrder(ctx, *order)
					if err != nil {
						logger.WithFields(loggrus.Fields{"request": *order}).WithError(err).Warn("Rolling back unsuccessfully")
					} else {
						logger.WithFields(loggrus.Fields{"request": *order}).Info("Rolling back successfully")
					}
				}
			} else {
				if reservationErr != nil {
					logger.WithFields(loggrus.Fields{"request": *order}).WithError(reservationErr).Error("Could not reserve this order. Aborting order...")
					status := models.StatusAborted("We could not reserve the articles in your cart. Create a cart with existing and available articles.")
					order.Status = status.Name
					order.Message = status.Message
				} else {
					logger.WithFields(loggrus.Fields{"request": *order}).Infof("Articles reserved for this order.")
					order.Price = *price
					status := models.StatusPaying()
					order.Status = status.Name
					order.Message = status.Message
				}
				err = order.PublishOrderStatusUpdate(service.AmqpChannel)
				if err != nil {
					logger.WithFields(loggrus.Fields{"request": *order}).WithError(err).Error("Could not publish order update")
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
