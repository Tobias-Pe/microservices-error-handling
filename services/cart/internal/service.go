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
	"github.com/Tobias-Pe/microservices-error-handling/api/proto"
	"github.com/Tobias-Pe/microservices-error-handling/api/requests"
	customerrors "github.com/Tobias-Pe/microservices-error-handling/pkg/custom-errors"
	loggingUtil "github.com/Tobias-Pe/microservices-error-handling/pkg/log"
	"github.com/Tobias-Pe/microservices-error-handling/pkg/metrics"
	"github.com/Tobias-Pe/microservices-error-handling/pkg/models"
	"github.com/Tobias-Pe/microservices-error-handling/pkg/rabbitmq"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/streadway/amqp"
	"math"
	"math/rand"
	"os"
	"strconv"
	"time"
)

const (
	// expireCartSeconds Seconds until the cart will expire after no updates
	expireCartSeconds  = 172800
	methodGetCart      = "GetCart"
	methodCreateCart   = "CreateCart"
	methodListenOrders = "PopulateOrder"
	methodPutCart      = "AddToCart"
	methodPublishOrder = "PublishOrder"
)

var logger = loggingUtil.InitLogger()

type Service struct {
	proto.UnimplementedCartServer
	rabbitmq.AmqpService
	database       *DbConnection
	orderMessages  <-chan amqp.Delivery
	requestsMetric *metrics.RequestsMetric
}

func NewService(cacheAddress string, cachePort string, rabbitAddress string, rabbitPort string) *Service {
	dbConnection := NewDbConnection(cacheAddress, cachePort)
	if dbConnection == nil {
		return nil
	}
	newService := Service{database: dbConnection}
	newService.RabbitURL = fmt.Sprintf("amqp://guest:guest@%s:%s/", rabbitAddress, rabbitPort)
	var err error = nil
	for i := 0; i < 6; i++ {
		err = newService.InitAmqpConnection()
		if err == nil {
			break
		}
		logger.Infof("Retrying... (%d/%d)", i, 5)
		time.Sleep(time.Duration(int64(math.Pow(2, float64(i)))) * time.Second)
	}
	if err != nil {
		return nil
	}

	err = newService.createOrderListener()
	if err != nil {
		return nil
	}

	newService.requestsMetric = metrics.NewRequestsMetrics()

	return &newService
}

// createOrderListener creates and binds queue to listen for orders in status fetching
func (service *Service) createOrderListener() error {
	var err error
	queueName := fmt.Sprintf("cart_%s_queue", requests.OrderTopic)
	routingKeys := []string{requests.OrderStatusFetch}

	service.orderMessages, err = service.CreateListener(requests.OrderTopic, queueName, routingKeys)
	if err != nil {
		return err
	}

	// create a coroutine to listen for order messages
	go service.ListenOrders()

	return nil
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
		} else {
			// unmarshall successfully --> handle the order
			err = service.handleOrder(order, message)
		}
		service.requestsMetric.Increment(err, methodListenOrders)
	}
	logger.Warn("Stopped Listening for Orders! Restarting...")
	// try to reconnect
	err := service.createOrderListener()
	if err != nil {
		logger.WithError(err).Error("Stopped Listening for Orders! Could not restart")
		os.Exit(1)
	}
}

func (service *Service) handleOrder(order *models.Order, message amqp.Delivery) error {
	// fetch the cart
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(1)*time.Second)
	cart, err := service.database.getCart(ctx, order.CartID)
	cancel()
	if err != nil { // there was no such cart in this order --> abort order because wrong information
		if !errors.Is(err, customerrors.ErrNoCartFound) && !errors.Is(err, customerrors.ErrCartIdInvalid) { // transaction error
			// randomize the requeueing
			go func() {
				sleepMult := rand.Intn(8) + 2
				time.Sleep(time.Second * time.Duration(sleepMult))
				logger.WithFields(logrus.Fields{"request": *order, "response": sleepMult}).WithError(err).Info("Could not reserve get cart. Requeued.")
				_ = message.Reject(true) // nack and requeue message
			}()
			return err
		}

		logger.WithFields(logrus.Fields{"request": *order}).WithError(err).Warn("Could not get cart from this order. Aborting order...")
		status := models.StatusAborted(models.CartAbortMessage)
		order.Status = status.Name
		order.Message = status.Message
	} else { // next step for the order is reserving the articles from the cart in stock
		logger.WithFields(logrus.Fields{"response": *cart, "request": *order}).Infof("Got cart for this order.")
		status := models.StatusReserving()
		order.Status = status.Name
		order.Message = status.Message
		order.Articles = cart.ArticleIDs
	}

	// broadcast the order update
	err = order.PublishOrderStatusUpdate(service.AmqpChannel)
	service.requestsMetric.Increment(err, methodPublishOrder)
	if err != nil {
		logger.WithFields(logrus.Fields{"response": *order}).WithError(err).Error("Could not publish order update")
		// randomize the requeueing
		go func() {
			sleepMult := rand.Intn(8) + 2
			time.Sleep(time.Second * time.Duration(sleepMult))
			logger.WithFields(logrus.Fields{"request": *order, "response": sleepMult}).WithError(err).Info("Could not publish order update. Requeued.")
			_ = message.Reject(true) // nack and requeue message
		}()
		return err
	}

	// there is no rollback transaction for this so ignore the ack error
	_ = message.Ack(false)

	return err
}

// CreateCart implementation of in the proto file defined interface of cart service
func (service *Service) CreateCart(ctx context.Context, req *proto.RequestNewCart) (*proto.ResponseNewCart, error) {
	if req == nil {
		return nil, customerrors.ErrRequestNil
	}
	cart, err := service.database.createCart(ctx, req.ArticleId)
	service.requestsMetric.Increment(err, methodCreateCart)

	if err != nil {
		return nil, err
	}

	strId := strconv.Itoa(int(cart.ID))

	logger.WithFields(logrus.Fields{"response": *cart, "request": req.ArticleId}).Info("Created new Cart")
	service.requestsMetric.Increment(err, methodCreateCart)

	return &proto.ResponseNewCart{CartId: strId}, nil
}

// GetCart implementation of in the proto file defined interface of cart service
func (service *Service) GetCart(ctx context.Context, req *proto.RequestCart) (*proto.ResponseCart, error) {
	if req == nil {
		return nil, customerrors.ErrRequestNil
	}
	cart, err := service.database.getCart(ctx, req.CartId)
	service.requestsMetric.Increment(err, methodGetCart)

	if err != nil {
		return nil, err
	}

	logger.WithFields(logrus.Fields{"response": *cart, "request": req.CartId}).Info("Looked up Cart")

	return &proto.ResponseCart{ArticleIds: cart.ArticleIDs}, nil
}

// PutCart implementation of in the proto file defined interface of cart service
func (service *Service) PutCart(ctx context.Context, req *proto.RequestPutCart) (*proto.Empty, error) {
	if req == nil {
		return nil, customerrors.ErrRequestNil
	}
	// add the requested item into the cart
	_, err := service.database.addToCart(ctx, req.CartId, req.ArticleId)
	service.requestsMetric.Increment(err, methodPutCart)
	if err != nil {
		logger.WithFields(logrus.Fields{"request": req.String()}).WithError(err).Warn("Could not add to cart.")
		return nil, err
	}
	logger.WithFields(logrus.Fields{"request": req.String()}).Infof("Inserted item")

	return &proto.Empty{}, nil
}
