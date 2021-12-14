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

package stock

import (
	"context"
	loggrus "github.com/sirupsen/logrus"
	"gitlab.lrz.de/peslalz/errorhandling-microservices-thesis/api/proto"
	loggingUtil "gitlab.lrz.de/peslalz/errorhandling-microservices-thesis/pkg/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
	"strings"
	"time"
)

var logger = loggingUtil.InitLogger()

type Service struct {
	proto.UnimplementedStockServer
	MongoClient     *mongo.Client
	stockCollection *mongo.Collection
}

type Article struct {
	Id       primitive.ObjectID `bson:"_id"`
	Name     string             `bson:"name"`
	Category string             `bson:"category"`
	Price    float64            `bson:"priceEuro"`
	Amount   int32              `bson:"amount"`
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
	stockCollection := client.Database("mongo_db").Collection("stock")
	s := &Service{MongoClient: client, stockCollection: stockCollection}
	return s
}

func (s *Service) GetArticles(ctx context.Context, req *proto.RequestArticles) (*proto.ResponseArticles, error) {

	wc := writeconcern.New(writeconcern.WMajority())
	rc := readconcern.Snapshot()
	txnOpts := options.Transaction().SetWriteConcern(wc).SetReadConcern(rc)
	session, err := s.MongoClient.StartSession()
	if err != nil {
		return nil, err
	}
	defer session.EndSession(context.Background())

	callback := s.createReadStockCallback(ctx, req)

	result, err := session.WithTransaction(context.Background(), callback, txnOpts)
	if err != nil {
		return nil, err
	}
	articles := result.([]Article)

	logger.Info(articles)

	var protoArticles []*proto.ResponseArticles_Article
	for _, a := range articles {
		protoArticles = append(protoArticles, &proto.ResponseArticles_Article{
			Id:        a.Id.Hex(),
			Name:      a.Name,
			Category:  a.Category,
			PriceEuro: float32(a.Price),
			Amount:    a.Amount,
		})
	}

	logger.Info("Request handled")
	return &proto.ResponseArticles{Articles: protoArticles}, nil
}

func (s *Service) createReadStockCallback(ctx context.Context, req *proto.RequestArticles) func(sessionContext mongo.SessionContext) (interface{}, error) {
	callback := func(sessionContext mongo.SessionContext) (interface{}, error) {
		var articles []Article
		var cursor *mongo.Cursor
		var err error
		if req.QueryCategoryValue == "*" {
			cursor, err = s.stockCollection.Find(ctx, bson.M{})
		} else {
			cursor, err = s.stockCollection.Find(ctx, bson.M{"category": strings.ToLower(req.QueryCategoryValue)})
		}
		defer func(cursor *mongo.Cursor, ctx context.Context) {
			err := cursor.Close(ctx)
			if err != nil {
			}
		}(cursor, ctx)
		if err != nil {
			return nil, err
		}
		if err = cursor.All(ctx, &articles); err != nil {
			return nil, err
		}
		return articles, nil
	}
	return callback
}
