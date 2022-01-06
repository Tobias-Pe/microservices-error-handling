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
	"github.com/Tobias-Pe/Microservices-Errorhandling/pkg/metrics"
	"github.com/Tobias-Pe/Microservices-Errorhandling/pkg/models"
	"github.com/Tobias-Pe/Microservices-Errorhandling/pkg/rabbitmq"
	"github.com/sirupsen/logrus"
	"github.com/streadway/amqp"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"google.golang.org/grpc"
	"math"
	"time"
)

var logger = loggingUtil.InitLogger()

const (
	connectionTimeSecs = 15

	methodGetArticles   = "GetArticles"
	methodUpdateArticle = "UpdateCache"
)

type Service struct {
	proto.UnimplementedCatalogueServer
	rabbitmq.AmqpService
	database            *DbConnection
	stockUpdateMessages <-chan amqp.Delivery
	requestsMetric      *metrics.RequestsMetric
}

func NewService(cacheAddress string, cachePort string, stockAddress string, stockPort string, rabbitAddress string, rabbitPort string) *Service {
	service := &Service{}

	service.requestsMetric = metrics.NewRequestsMetrics()

	service.database = NewDbConnection(cacheAddress, cachePort)
	service.initCache(stockAddress, stockPort)

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

	err = service.createStockListener()
	if err != nil {
		return nil
	}

	return service
}

func (service *Service) initCache(stockAddress string, stockPort string) {
	// Set up a connection to the server.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*connectionTimeSecs)

	conn, err := grpc.DialContext(ctx, stockAddress+":"+stockPort, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		logger.WithError(err).Panicln("did not connect to stock-service")
	}
	cancel()
	logger.Infoln("Connection to stock-service successfully!")

	var response *proto.ResponseArticles
	for response == nil {
		client := proto.NewStockClient(conn)
		ctx, cancel = context.WithTimeout(context.Background(), connectionTimeSecs*time.Second)
		request := &proto.RequestArticles{}
		response, err = client.GetArticles(ctx, request)
		cancel()
		if err != nil {
			logger.WithError(err).Panicln("did not get articles from stock-service")
		}
	}

	for _, articleProto := range response.Articles {
		hex, err := primitive.ObjectIDFromHex(articleProto.Id)
		if err != nil {
			logger.WithError(err).Error("could not create object id from articleID")
		} else {
			article := models.Article{
				Category: articleProto.Category,
				Amount:   articleProto.Amount,
				Name:     articleProto.Name,
				Price:    float64(articleProto.PriceEuro),
				ID:       hex,
			}
			err = service.database.updateArticle(&article)
			if err != nil {
				logger.WithError(err).Error("could not persist article")
			}
		}
		service.requestsMetric.Increment(err, methodUpdateArticle)
	}
	logger.WithFields(logrus.Fields{"request": response.String()}).Info("Initialised Cache.")
}

// GetArticles implementation of in the proto file defined interface of catalogue service
func (service *Service) GetArticles(_ context.Context, req *proto.RequestArticles) (*proto.ResponseArticles, error) {
	articles, err := service.database.getArticles(req.CategoryQuery)
	service.requestsMetric.Increment(err, methodGetArticles)
	if err != nil {
		logger.WithFields(logrus.Fields{"request": req.String()}).WithError(err).Error("could not get articles")
		return nil, err
	}
	logger.WithFields(logrus.Fields{"request": req.String()}).Info("Fetched articles.")

	var articlesProto []*proto.Article
	for _, article := range *articles {
		articlesProto = append(articlesProto, &proto.Article{
			Id:        article.ID.Hex(),
			Name:      article.Name,
			Category:  article.Category,
			PriceEuro: float32(article.Price),
			Amount:    article.Amount,
		})
	}
	return &proto.ResponseArticles{Articles: articlesProto}, nil
}

func (service *Service) createStockListener() error {
	var err error
	queueName := fmt.Sprintf("catalogue_%s_queue", requests.ArticlesTopic)
	routingKeys := []string{requests.StockUpdateRoutingKey}

	service.stockUpdateMessages, err = service.CreateListener(requests.ArticlesTopic, queueName, routingKeys)
	if err != nil {
		return err
	}

	// create a coroutine to listen for order messages
	go service.ListenStockUpdates()

	return nil
}

// ListenStockUpdates reads out stockUpdate messages from bound amqp queue
func (service *Service) ListenStockUpdates() {
	for message := range service.stockUpdateMessages {
		// unmarshall message into StockSupplyMessage
		article := &models.Article{}
		err := json.Unmarshal(message.Body, article)
		if err == nil {
			err = service.database.updateArticle(article)
			if err != nil {
				logger.WithFields(logrus.Fields{"request": *article}).WithError(err).Error("Could not update article.")
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
		service.requestsMetric.Increment(err, methodUpdateArticle)
	}

	logger.Warn("Stopped Listening for Stock Updates! Restarting...")
	// try reconnecting
	err := service.createStockListener()
	if err != nil {
		logger.WithError(err).Error("Stopped Listening for Stock Updates! Could not restart")
	} else {
		service.ListenStockUpdates()
	}
}
