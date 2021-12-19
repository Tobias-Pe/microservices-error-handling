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
	proto.UnimplementedStockServer
	Database               *DbConnection
	AmqpChannel            *amqp.Channel
	AmqpConn               *amqp.Connection
	orderMessages          <-chan amqp.Delivery
	rabbitUrl              string
	abortedOrderMessages   <-chan amqp.Delivery
	completedOrderMessages <-chan amqp.Delivery
	supplyMessages         <-chan amqp.Delivery
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
	err = s.createAbortedOrderListener()
	if err != nil {
		return nil
	}
	err = s.createCompletedOrderListener()
	if err != nil {
		return nil
	}
	err = s.createSupplierListener()
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

func (service *Service) createAbortedOrderListener() error {
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
		"stock_"+requests.OrderTopic+"_aborted_queue", // name
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
		q.Name,                    // queue name
		requests.OrderStatusAbort, // routing key
		requests.OrderTopic,       // exchange
		false,
		nil,
	)
	if err != nil {
		logger.WithError(err).Error("Could not bind queue")
		return err
	}

	service.abortedOrderMessages, err = service.AmqpChannel.Consume(
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

	go service.ListenAbortedOrders()
	return nil
}

func (service *Service) createCompletedOrderListener() error {
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
		"stock_"+requests.OrderTopic+"_completed_queue", // name
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
		q.Name,                       // queue name
		requests.OrderStatusComplete, // routing key
		requests.OrderTopic,          // exchange
		false,
		nil,
	)
	if err != nil {
		logger.WithError(err).Error("Could not bind queue")
		return err
	}

	service.completedOrderMessages, err = service.AmqpChannel.Consume(
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

	go service.ListenCompletedOrders()
	return nil
}

func (service *Service) createSupplierListener() error {
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
		"stock_"+requests.ArticlesTopic+"_queue", // name
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
		q.Name,                    // queue name
		requests.SupplyRoutingKey, // routing key
		requests.ArticlesTopic,    // exchange
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

	go service.ListenSupply()
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

func (service *Service) reserveArticlesAndCalcPrice(order *models.Order) (*float64, error) {
	articleQuantityMap := map[string]int{}
	for _, id := range order.Articles {
		_, exists := articleQuantityMap[id]
		if exists {
			articleQuantityMap[id]++
		} else {
			articleQuantityMap[id] = 1
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*1)
	reservedArticles, err := service.Database.reserveOrder(ctx, articleQuantityMap, *order)
	cancel()
	if err != nil {
		return nil, err
	}
	price := 0.0
	for _, article := range *reservedArticles {
		price += article.Price * float64(articleQuantityMap[article.ID.Hex()])
		if article.Amount <= 10 {
			err := service.orderArticles(article.ID, 50)
			if err != nil {
				logger.WithError(err).Error("Could not publish supply request")
			}
		}
	}
	return &price, nil
}

func (service *Service) orderArticles(id primitive.ObjectID, amount int) error {
	bytes, err := json.Marshal(requests.StockSupplyMessage{
		ArticleID: id.Hex(),
		Amount:    amount,
	})
	if err != nil {
		return err
	}
	err = service.AmqpChannel.Publish(
		requests.ArticlesTopic,          // exchange
		requests.StockRequestRoutingKey, // routing key
		true,                            // mandatory
		false,                           // immediate
		amqp.Publishing{
			DeliveryMode: amqp.Persistent,
			ContentType:  "application/json",
			Body:         bytes,
		})
	if err != nil {
		return err
	}
	logger.WithFields(loggrus.Fields{"article": id.Hex(), "amount": amount}).Infof("Published Supply Request")
	return nil
}

func (service *Service) ListenOrders() {
	for message := range service.orderMessages {
		order := &models.Order{}
		err := json.Unmarshal(message.Body, order)
		if err == nil {
			price, reservationErr := service.reserveArticlesAndCalcPrice(order)
			err = message.Ack(false)
			if err != nil {
				logger.WithError(err).Error("Could not ack message.")
				if reservationErr == nil {
					logger.WithFields(loggrus.Fields{"request": *order}).WithError(err).Info("Rolling back transaction...")
					ctx, cancel := context.WithTimeout(context.Background(), time.Second*1)
					err = service.Database.rollbackReserveOrder(ctx, *order)
					cancel()
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

func (service *Service) ListenAbortedOrders() {
	for message := range service.abortedOrderMessages {
		order := &models.Order{}
		err := json.Unmarshal(message.Body, order)
		if err == nil {
			var abortionErr error
			for i := 4; i < 7; i++ {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*1)
				abortionErr = service.Database.rollbackReserveOrder(ctx, *order)
				cancel()
				if abortionErr == nil {
					logger.WithFields(loggrus.Fields{"request": *order}).Infof("Reservation undone.")
					break
				}
				logger.WithFields(loggrus.Fields{"retry": i - 3, "request": *order}).WithError(abortionErr).Error("Could not delete reservation. Retrying...")
				time.Sleep(time.Duration(int64(math.Pow(2, float64(i)))) * time.Millisecond)
			}
			err = message.Ack(false)
			if err != nil {
				logger.WithError(err).Error("Could not ack message.")
				if abortionErr == nil {
					logger.WithFields(loggrus.Fields{"request": *order}).WithError(err).Info("Rolling back transaction...")
					_, err = service.reserveArticlesAndCalcPrice(order)
					if err != nil {
						logger.WithFields(loggrus.Fields{"request": *order}).WithError(err).Warn("Rolling back unsuccessfully")
					} else {
						logger.WithFields(loggrus.Fields{"request": *order}).Info("Rolling back successfully")
					}
				}
			}
		} else {
			logger.WithError(err).Error("Could not unmarshall message")
		}
	}
	logger.Error("Stopped Listening for aborted Orders! Restarting...")
	err := service.createAbortedOrderListener()
	if err != nil {
		logger.Error("Stopped Listening for aborted Orders! Could not restart")
	}
}

func (service *Service) ListenCompletedOrders() {
	for message := range service.completedOrderMessages {
		order := &models.Order{}
		err := json.Unmarshal(message.Body, order)
		if err == nil {
			var deletionErr error
			for i := 4; i < 7; i++ {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*1)
				deletionErr = service.Database.deleteReservation(ctx, order.ID)
				cancel()
				if deletionErr == nil {
					logger.WithFields(loggrus.Fields{"request": *order}).Infof("Reservation deleted.")
					break
				}
				logger.WithFields(loggrus.Fields{"retry": i - 3, "request": *order}).WithError(deletionErr).Error("Could not delete reservation. Retrying...")
				time.Sleep(time.Duration(int64(math.Pow(2, float64(i)))) * time.Millisecond)
			}
			err = message.Ack(false)
			if err != nil {
				logger.WithError(err).Error("Could not ack message.")
				if deletionErr == nil {
					logger.WithFields(loggrus.Fields{"request": *order}).WithError(err).Info("Rolling back transaction...")
					ctx, cancel := context.WithTimeout(context.Background(), time.Second*1)
					err = service.Database.rollbackDeleteReservation(ctx, order.ID)
					cancel()
					if err != nil {
						logger.WithFields(loggrus.Fields{"request": *order}).WithError(err).Warn("Rolling back unsuccessfully")
					} else {
						logger.WithFields(loggrus.Fields{"request": *order}).Info("Rolling back successfully")
					}
				}
			}
		} else {
			logger.WithError(err).Error("Could not unmarshall message")
		}
	}
	logger.Error("Stopped Listening for aborted Orders! Restarting...")
	err := service.createCompletedOrderListener()
	if err != nil {
		logger.Error("Stopped Listening for aborted Orders! Could not restart")
	}
}

func (service *Service) ListenSupply() {
	for message := range service.supplyMessages {
		supply := &requests.StockSupplyMessage{}
		err := json.Unmarshal(message.Body, supply)
		if err == nil {
			var updateErr error
			for i := 4; i < 7; i++ {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*1)
				updateErr = service.Database.restockArticle(ctx, supply.ArticleID, supply.Amount)
				cancel()
				if updateErr == nil {
					logger.WithFields(loggrus.Fields{"request": *supply}).Infof("Restocked.")
					break
				}
				logger.WithFields(loggrus.Fields{"retry": i - 3, "request": *supply}).WithError(updateErr).Error("Could not restock requested article. Retrying...")
				time.Sleep(time.Duration(int64(math.Pow(2, float64(i)))) * time.Millisecond)
			}
			err = message.Ack(false)
			if err != nil {
				logger.WithError(err).Error("Could not ack message.")
				if updateErr == nil {
					logger.WithFields(loggrus.Fields{"request": *supply}).WithError(err).Info("Rolling back transaction...")
					ctx, cancel := context.WithTimeout(context.Background(), time.Second*1)
					err = service.Database.rollbackRestockArticle(ctx, supply.ArticleID, supply.Amount)
					cancel()
					if err != nil {
						logger.WithFields(loggrus.Fields{"request": *supply}).WithError(err).Warn("Rolling back unsuccessfully")
					} else {
						logger.WithFields(loggrus.Fields{"request": *supply}).Info("Rolling back successfully")
					}
				}
			}
		} else {
			logger.WithError(err).Error("Could not unmarshall message")
		}
	}
	logger.Error("Stopped Listening for Supply! Restarting...")
	err := service.createSupplierListener()
	if err != nil {
		logger.Error("Stopped Listening for Supply! Could not restart")
	}
}
