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
	loggrus "github.com/sirupsen/logrus"
	"github.com/streadway/amqp"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"math"
	"time"
)

var logger = loggingUtil.InitLogger()

const (
	StockAbortMessage = "We could not reserve the articles in your cart. Create a cart with existing and available articles."

	methodGetArticles             = "GetArticles"
	methodListenSupply            = "AddSupply"
	methodPublishSupplyRequest    = "PublishLowSuppl"
	methodListenReservationOrders = "ReserveArticles"
	methodListenAbortedOrders     = "AbortReservation"
	methodListenCompletedOrders   = "CompleteReservation"
	methodPublishOrder            = "PublishOrder"
)

type Service struct {
	proto.UnimplementedStockServer
	rabbitmq.AmqpService
	Database                 *DbConnection
	reservationOrderMessages <-chan amqp.Delivery
	abortedOrderMessages     <-chan amqp.Delivery
	completedOrderMessages   <-chan amqp.Delivery
	supplyMessages           <-chan amqp.Delivery
	requestsMetric           *metrics.RequestsMetric
	stockMetric              *metrics.StockMetric
}

func NewService(mongoAddress string, mongoPort string, rabbitAddress string, rabbitPort string) *Service {
	service := &Service{Database: NewDbConnection(mongoAddress, mongoPort)}
	service.RabbitURL = fmt.Sprintf("amqp://guest:guest@%s:%s/", rabbitAddress, rabbitPort)
	var err error = nil
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

	err = service.createReservationOrderListener()
	if err != nil {
		return nil
	}

	err = service.createAbortedOrderListener()
	if err != nil {
		return nil
	}

	err = service.createCompletedOrderListener()
	if err != nil {
		return nil
	}

	err = service.createSupplierListener()
	if err != nil {
		return nil
	}

	service.requestsMetric = metrics.NewRequestsMetrics()
	service.stockMetric = metrics.NewStockMetric()

	return service
}

// createReservationOrderListener creates and binds queue to listen for orders in status reservation
func (service *Service) createReservationOrderListener() error {
	var err error
	queueName := fmt.Sprintf("stock_%s_queue", requests.OrderTopic)
	routingKeys := []string{requests.OrderStatusReserve}

	service.reservationOrderMessages, err = service.CreateListener(requests.OrderTopic, queueName, routingKeys)
	if err != nil {
		return err
	}

	// create a coroutine to listen for these messages
	go service.ListenReservationOrders()

	return nil
}

// createAbortedOrderListener creates and binds queue to listen for orders in status aborted
func (service *Service) createAbortedOrderListener() error {
	var err error
	queueName := fmt.Sprintf("stock_%s_aborted_queue", requests.OrderTopic)
	routingKeys := []string{requests.OrderStatusAbort}

	service.abortedOrderMessages, err = service.CreateListener(requests.OrderTopic, queueName, routingKeys)
	if err != nil {
		return err
	}

	// create a coroutine to listen for these messages
	go service.ListenAbortedOrders()

	return nil
}

// createCompletedOrderListener creates and binds queue to listen for orders in status completed
func (service *Service) createCompletedOrderListener() error {
	var err error
	queueName := fmt.Sprintf("stock_%s_completed_queue", requests.OrderTopic)
	routingKeys := []string{requests.OrderStatusComplete}

	service.completedOrderMessages, err = service.CreateListener(requests.OrderTopic, queueName, routingKeys)
	if err != nil {
		return err
	}

	// create a coroutine to listen for these messages
	go service.ListenCompletedOrders()

	return nil
}

// createSupplierListener creates and binds queue to listen for supply responses
func (service *Service) createSupplierListener() error {
	var err error
	queueName := fmt.Sprintf("stock_%s_queue", requests.ArticlesTopic)
	routingKeys := []string{requests.SupplyRoutingKey}

	service.supplyMessages, err = service.CreateListener(requests.ArticlesTopic, queueName, routingKeys)
	if err != nil {
		return err
	}

	// create a coroutine to listen for these messages
	go service.ListenSupply()

	return nil
}

// GetArticles implementation of in the proto file defined interface of stock service
func (service *Service) GetArticles(ctx context.Context, req *proto.RequestArticles) (*proto.ResponseArticles, error) {
	articles, err := service.Database.getArticles(ctx, req.CategoryQuery)
	service.requestsMetric.Increment(err, methodGetArticles)
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

	logger.WithFields(loggrus.Fields{"response": *articles}).Info("Get articles handled")

	return &proto.ResponseArticles{Articles: protoArticles}, nil
}

// reserveArticlesAndCalcPrice reserves the order and with the returned list of articles calculates the price of all reserved articles.
// Also, if the reserved article is low on stock publishes a supply request.
func (service *Service) reserveArticlesAndCalcPrice(order *models.Order) (*float64, error) {
	// map articleId onto the amount of to be reserved articles
	articleQuantityMap := map[string]int{}
	for _, id := range order.Articles {
		_, exists := articleQuantityMap[id]
		if exists {
			articleQuantityMap[id]++
		} else {
			articleQuantityMap[id] = 1
		}
	}
	// reserve articles
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*1)
	defer cancel()
	reservedArticles, err := service.Database.reserveOrder(ctx, articleQuantityMap, *order)
	if err != nil {
		return nil, err
	}
	service.stockMetric.IncrementReservation()
	// calculate the price
	price := 0.0
	for _, article := range *reservedArticles {
		// price of the reserved article times the ordered quantity
		price += article.Price * float64(articleQuantityMap[article.ID.Hex()])
		// check the article
		if article.Amount <= 10 {
			// publish supply order
			err = service.orderArticles(article.ID, 50)
			if err != nil {
				logger.WithError(err).Error("Could not publish supply request")
			}
		}
	}
	return &price, nil
}

// orderArticles broadcasts a supply request
func (service *Service) orderArticles(id primitive.ObjectID, amount int) error {
	bytes, err := json.Marshal(requests.StockSupplyMessage{
		ArticleID: id.Hex(),
		Amount:    amount,
	})
	if err != nil {
		service.requestsMetric.Increment(err, methodPublishSupplyRequest)
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
	service.requestsMetric.Increment(err, methodPublishSupplyRequest)
	if err != nil {
		return err
	}

	logger.WithFields(loggrus.Fields{"request": id.Hex(), "response": fmt.Sprintln(amount)}).Infof("Published Supply Request")

	return nil
}

// ListenReservationOrders reads out reservation order messages from bound amqp queue
func (service *Service) ListenReservationOrders() {
	for message := range service.reservationOrderMessages {
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
			err = service.handleReservationOrder(order, message)
		}
		service.requestsMetric.Increment(err, methodListenReservationOrders)
	}

	logger.Warn("Stopped Listening for Orders! Restarting...")
	// try to reconnect
	err := service.createReservationOrderListener()
	if err != nil {
		logger.WithError(err).Error("Stopped Listening for Orders! Could not restart")
	}
}

func (service *Service) handleReservationOrder(order *models.Order, message amqp.Delivery) error {
	price, err := service.reserveArticlesAndCalcPrice(order)
	if err != nil { // could not reserve order --> abort order because wrong information
		if !errors.Is(err, primitive.ErrInvalidHex) && !errors.Is(err, customerrors.ErrNoModification) && !errors.Is(err, customerrors.ErrLowStock) { // it must be a transaction error
			logger.WithFields(loggrus.Fields{"request": *order}).WithError(err).Warn("Could not reserve this order. Retrying...")
			_ = message.Reject(true) // nack and requeue message
			return err
		}
		logger.WithFields(loggrus.Fields{"request": *order}).WithError(err).Warn("Could not reserve this order. Aborting order...")
		status := models.StatusAborted(StockAbortMessage)
		order.Status = status.Name
		order.Message = status.Message
	} else { // next step for the order is paying the articles in payment-service
		logger.WithFields(loggrus.Fields{"request": *order}).Infof("Articles reserved for this order.")
		status := models.StatusPaying()
		order.Status = status.Name
		order.Message = status.Message
		order.Price = *price
	}

	// broadcast the order update
	err = order.PublishOrderStatusUpdate(service.AmqpChannel)
	service.requestsMetric.Increment(err, methodPublishOrder)
	if err != nil {
		logger.WithError(err).Error("Could not publish order update")
		_ = message.Reject(true) // nack and requeue message
		return err
	}

	err = message.Ack(false)
	if err != nil { // ack could not be sent but database transaction was successfully
		logger.WithError(err).Error("Could not ack message. Trying to roll back...")

		// rollback transaction. because of the missing ack the current request will be resent
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*1)
		defer cancel()
		rollbackErr := service.Database.rollbackReserveOrder(ctx, *order)
		if rollbackErr != nil {
			logger.WithError(rollbackErr).Error("Could not rollback.")
			err = fmt.Errorf("%v ; %v", err.Error(), rollbackErr.Error())
		} else {
			service.stockMetric.DecrementReservation()
			logger.WithFields(loggrus.Fields{"request": *order}).Info("Rolling back successfully")
		}
	}

	return err
}

// ListenAbortedOrders reads out order messages from bound amqp queue
func (service *Service) ListenAbortedOrders() {
	for message := range service.abortedOrderMessages {
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
			err = service.handleAbortedOrder(order, message)
		}
		service.requestsMetric.Increment(err, methodListenAbortedOrders)
	}

	logger.Warn("Stopped Listening for aborted Orders! Restarting...")
	// try to reconnect
	err := service.createAbortedOrderListener()
	if err != nil {
		logger.WithError(err).Error("Stopped Listening for aborted Orders! Could not restart")
	}
}

func (service *Service) handleAbortedOrder(order *models.Order, message amqp.Delivery) error {
	var err error

	// Don't listen for own abortion or abortions from cart service --> there will be no reservation
	if order.Message == StockAbortMessage || order.Message == models.CartAbortMessage {
		_ = message.Ack(false)
		return nil
	}

	// retry deleting aborted order from reservations
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*1)
	// aborting reservation == rollback reservation
	err = service.Database.rollbackReserveOrder(ctx, *order)
	cancel()

	if err != nil {
		logger.WithFields(loggrus.Fields{"request": *order}).WithError(err).Warn("Could not delete reservation.")

		if /*errors.Is(err, customerrors.ErrNoReservation) ||*/ errors.Is(err, primitive.ErrInvalidHex) || errors.Is(err, customerrors.ErrNoModification) {
			// there is no rollback needed so ignore the ack error
			_ = message.Ack(false)
		} else {
			_ = message.Reject(true) // nack and requeue message
		}

		return err
	}
	logger.WithFields(loggrus.Fields{"request": *order}).Infof("Reservation undone.")
	service.stockMetric.DecrementReservation()

	err = message.Ack(false)
	if err != nil { // ack could not be sent but database transaction was successfully
		logger.WithError(err).Error("Could not ack message. Trying to roll back...")

		// rollback transaction. because of the missing ack the current request will be resent
		logger.WithFields(loggrus.Fields{"request": *order}).WithError(err).Info("Rolling back transaction...")
		_, rollbackErr := service.reserveArticlesAndCalcPrice(order)
		if rollbackErr != nil {
			logger.WithError(rollbackErr).Error("Could not rollback.")
			err = fmt.Errorf("%v ; %v", err.Error(), rollbackErr.Error())
		} else {
			service.stockMetric.IncrementReservation()
			logger.WithFields(loggrus.Fields{"request": *order}).Info("Rolling back successfully")
		}
	}

	return err
}

// ListenCompletedOrders reads out order messages from bound amqp queue
func (service *Service) ListenCompletedOrders() {
	for message := range service.completedOrderMessages {
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
			err = service.handleCompletedOrder(order, message)
		}
		service.requestsMetric.Increment(err, methodListenCompletedOrders)
	}

	logger.Warn("Stopped Listening for completed Orders! Restarting...")
	// try to reconnect
	err := service.createCompletedOrderListener()
	if err != nil {
		logger.WithError(err).Error("Stopped Listening for completed Orders! Could not restart")
	}
}

func (service *Service) handleCompletedOrder(order *models.Order, message amqp.Delivery) error {
	var err error

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*1)
	// remove the reservation
	err = service.Database.deleteReservation(ctx, order.ID)
	cancel()
	if err != nil {
		logger.WithFields(loggrus.Fields{"request": *order}).WithError(err).Warn("Could not delete reservation.")

		_ = message.Reject(true) // nack and requeue message

		return err
	}
	logger.WithFields(loggrus.Fields{"request": *order}).Info("Reservation deleted.")
	service.stockMetric.DecrementReservation()

	err = message.Ack(false)
	if err != nil { // ack could not be sent but database transaction was successfully
		logger.WithError(err).Error("Could not ack message. Trying to roll back...")

		// rollback transaction. because of the missing ack the current request will be resent
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*1)
		rollbackErr := service.Database.rollbackDeleteReservation(ctx, order.ID)
		cancel()
		if err != nil {
			logger.WithError(rollbackErr).Error("Could not rollback.")
			err = fmt.Errorf("%v ; %v", err.Error(), rollbackErr.Error())
		} else {
			service.stockMetric.IncrementReservation()
			logger.WithFields(loggrus.Fields{"request": *order}).Info("Rolling back successfully")
		}
	}

	return err
}

// ListenSupply reads out supply messages from bound amqp queue
func (service *Service) ListenSupply() {
	for message := range service.supplyMessages {
		// unmarshall message into StockSupplyMessage
		supply := &requests.StockSupplyMessage{}
		err := json.Unmarshal(message.Body, supply)
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
			err = service.handleSupply(supply, message)
		}
		service.requestsMetric.Increment(err, methodListenSupply)
	}
	logger.Warn("Stopped Listening for Supply! Restarting...")
	err := service.createSupplierListener()
	if err != nil {
		logger.WithError(err).Error("Stopped Listening for Supply! Could not restart")
	}
}

func (service *Service) handleSupply(supply *requests.StockSupplyMessage, message amqp.Delivery) error {
	var err error

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*1)
	defer cancel()

	err = service.Database.restockArticle(ctx, supply.ArticleID, supply.Amount)
	if err != nil {
		logger.WithFields(loggrus.Fields{"request": *supply}).WithError(err).Warn("Could not restock supply.")

		if errors.Is(err, customerrors.ErrNoModification) {
			// there is no rollback needed so ignore the ack error
			_ = message.Ack(false)
		} else {
			_ = message.Reject(true) // nack and requeue message
		}

		return err
	}

	err = message.Ack(false)
	if err != nil {
		logger.WithError(err).Error("Could not ack message.")

		logger.WithFields(loggrus.Fields{"request": *supply}).WithError(err).Info("Rolling back transaction...")
		rollbackErr := service.Database.rollbackRestockArticle(ctx, supply.ArticleID, supply.Amount)
		if err != nil {
			logger.WithError(rollbackErr).Error("Could not rollback.")
			err = fmt.Errorf("%v ; %v", err.Error(), rollbackErr.Error())
		} else {
			logger.WithFields(loggrus.Fields{"request": *supply}).Info("Rolling back successfully")
		}
	}
	return err
}
