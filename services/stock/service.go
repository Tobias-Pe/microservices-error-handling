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
	"fmt"
	loggrus "github.com/sirupsen/logrus"
	"gitlab.lrz.de/peslalz/errorhandling-microservices-thesis/api/proto"
	loggingUtil "gitlab.lrz.de/peslalz/errorhandling-microservices-thesis/pkg/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"strconv"
	"time"
)

var logger = loggingUtil.InitLogger()

type Service struct {
	proto.UnimplementedStockServer
	MongoClient     *mongo.Client
	stockCollection *mongo.Collection
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
	cursor, err := s.stockCollection.Find(ctx, bson.M{})
	defer func(cursor *mongo.Cursor, ctx context.Context) {
		err := cursor.Close(ctx)
		if err != nil {

		}
	}(cursor, ctx)
	if err != nil {
		return nil, err
	}

	var articles []*proto.ResponseArticles_Article
	for cursor.Next(ctx) {
		var article bson.M
		if err = cursor.Decode(&article); err != nil {
			return nil, err
		}
		name := fmt.Sprint(article["name"])
		category := fmt.Sprint(article["category"])
		strPrice := fmt.Sprint(article["priceEuro"])
		price, err := strconv.ParseFloat(strPrice, 64)
		if err != nil {
			logger.Errorf("float parse %s %s", strPrice, err)
			return nil, err
		}
		strAmount := fmt.Sprint(article["amount"])
		amount, err := strconv.Atoi(strAmount)
		if err != nil {
			return nil, err
		}
		articles = append(articles, &proto.ResponseArticles_Article{
			Name:      name,
			Category:  category,
			PriceEuro: float32(price),
			Amount:    int32(amount),
		})
	}

	logger.Info("Request handled")
	return &proto.ResponseArticles{Articles: articles}, nil
}
