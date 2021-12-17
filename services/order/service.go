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

package order

import (
	"context"
	loggrus "github.com/sirupsen/logrus"
	"gitlab.lrz.de/peslalz/errorhandling-microservices-thesis/api/proto"
	loggingUtil "gitlab.lrz.de/peslalz/errorhandling-microservices-thesis/pkg/log"
	"gitlab.lrz.de/peslalz/errorhandling-microservices-thesis/pkg/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
	"time"
)

var logger = loggingUtil.InitLogger()

type Service struct {
	proto.UnimplementedOrderServer
	MongoClient     *mongo.Client
	orderCollection *mongo.Collection
}

func NewService(mongoAddress string, mongoPort string) *Service {
	mongoUri := "mongodb://" + mongoAddress + ":" + mongoPort
	client, err := mongo.NewClient(options.Client().ApplyURI(mongoUri))
	if err != nil {
		logger.WithFields(loggrus.Fields{"mongoURI": mongoUri}).WithError(err).Errorf("Could not connect to DB")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err = client.Connect(ctx)
	if err != nil {
		logger.WithFields(loggrus.Fields{"mongoURI": mongoUri}).WithError(err).Errorf("Could not connect to DB")
	} else {
		logger.WithFields(loggrus.Fields{"mongoURI": mongoUri}).Info("Connection to DB successfully!")
	}
	orderCollection := client.Database("mongo_db").Collection("order")
	s := &Service{MongoClient: client, orderCollection: orderCollection}
	return s
}

func (s *Service) CreateOrder(ctx context.Context, req *proto.RequestNewOrder) (*proto.OrderObject, error) {
	status := models.StatusFetching()
	order := models.Order{
		Status:             status.Name,
		Message:            status.Message,
		Articles:           nil,
		CartID:             req.CartId,
		Price:              -1,
		CustomerAddress:    req.CustomerAddress,
		CustomerName:       req.CustomerName,
		CustomerCreditCard: req.CustomerCreditCard,
	}

	wc := writeconcern.New(writeconcern.WMajority())
	rc := readconcern.Snapshot()
	txnOpts := options.Transaction().SetWriteConcern(wc).SetReadConcern(rc)
	session, err := s.MongoClient.StartSession()
	if err != nil {
		return nil, err
	}
	defer session.EndSession(context.Background())

	callback := s.newCallbackCreateOrder(ctx, order)

	result, err := session.WithTransaction(context.Background(), callback, txnOpts)
	if err != nil {
		return nil, err
	}
	orderId := result.(primitive.ObjectID)
	order.ID = orderId
	logger.WithFields(loggrus.Fields{"Request": req, "Order": order}).Info("Order created")
	return &proto.OrderObject{
		OrderId:            order.ID.Hex(),
		Status:             order.Status,
		Message:            order.Message,
		Price:              float32(order.Price),
		CustomerAddress:    order.CustomerAddress,
		CustomerName:       order.CustomerName,
		CustomerCreditCard: order.CustomerCreditCard,
	}, nil
}

func (s *Service) GetOrder(ctx context.Context, req *proto.RequestOrder) (*proto.OrderObject, error) {
	orderId, err := primitive.ObjectIDFromHex(req.OrderId)
	if err != nil {
		return nil, err
	}
	wc := writeconcern.New(writeconcern.WMajority())
	rc := readconcern.Snapshot()
	txnOpts := options.Transaction().SetWriteConcern(wc).SetReadConcern(rc)
	session, err := s.MongoClient.StartSession()
	if err != nil {
		return nil, err
	}
	defer session.EndSession(context.Background())

	callback := s.newCallbackGetOrder(ctx, orderId)
	result, err := session.WithTransaction(context.Background(), callback, txnOpts)
	if err != nil {
		return nil, err
	}
	order := result.(models.Order)
	logger.WithFields(loggrus.Fields{"Request": req, "Order": order}).Info("Get order")
	return &proto.OrderObject{
		OrderId:            order.ID.Hex(),
		Status:             order.Status,
		Message:            order.Message,
		Price:              float32(order.Price),
		CustomerAddress:    order.CustomerAddress,
		CustomerName:       order.CustomerName,
		CustomerCreditCard: order.CustomerCreditCard,
	}, nil
}

func (s *Service) newCallbackCreateOrder(ctx context.Context, order models.Order) func(sessionContext mongo.SessionContext) (interface{}, error) {
	callback := func(sessionContext mongo.SessionContext) (interface{}, error) {
		orderResult, err := s.orderCollection.InsertOne(ctx, order)
		if err != nil {
			return nil, err
		}
		return orderResult.InsertedID, nil
	}
	return callback
}

func (s *Service) newCallbackGetOrder(ctx context.Context, orderId primitive.ObjectID) func(sessionContext mongo.SessionContext) (interface{}, error) {
	callback := func(sessionContext mongo.SessionContext) (interface{}, error) {
		result := s.orderCollection.FindOne(ctx, bson.M{"_id": orderId})
		order := models.Order{}
		err := result.Decode(&order)
		if err != nil {
			return nil, err
		}
		return order, nil
	}
	return callback
}
