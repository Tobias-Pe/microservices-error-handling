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
	"fmt"
	"github.com/Tobias-Pe/Microservices-Errorhandling/pkg/models"
	loggrus "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
	"strings"
	"time"
)

type DbConnection struct {
	MongoClient           *mongo.Client
	stockCollection       *mongo.Collection
	reservationCollection *mongo.Collection
}

func NewDbConnection(mongoAddress string, mongoPort string) *DbConnection {
	var err error
	db := &DbConnection{}
	mongoUri := "mongodb://" + mongoAddress + ":" + mongoPort
	db.MongoClient, err = mongo.NewClient(options.Client().ApplyURI(mongoUri))
	if err != nil {
		logger.WithFields(loggrus.Fields{"mongoURI": mongoUri}).WithError(err).Error("Could not connect to DB")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err = db.MongoClient.Connect(ctx)
	if err != nil {
		logger.WithFields(loggrus.Fields{"mongoURI": mongoUri}).WithError(err).Error("Could not connect to DB")
	} else {
		logger.WithFields(loggrus.Fields{"mongoURI": mongoUri}).Info("Connection to DB successfully!")
	}
	db.stockCollection = db.MongoClient.Database("mongo_db").Collection("stock")
	db.reservationCollection = db.MongoClient.Database("mongo_db").Collection("reservation")
	return db
}

func (database *DbConnection) getArticles(ctx context.Context, category string) (*[]models.Article, error) {
	wc := writeconcern.New(writeconcern.WMajority())
	rc := readconcern.Snapshot()
	txnOpts := options.Transaction().SetWriteConcern(wc).SetReadConcern(rc)
	session, err := database.MongoClient.StartSession()
	if err != nil {
		return nil, err
	}
	defer session.EndSession(context.Background())

	callback := database.newCallbackGetArticles(ctx, category)

	result, err := session.WithTransaction(context.Background(), callback, txnOpts)
	if err != nil {
		return nil, err
	}
	articles := result.([]models.Article)
	return &articles, nil
}

func (database *DbConnection) newCallbackGetArticles(ctx context.Context, category string) func(sessionContext mongo.SessionContext) (interface{}, error) {
	callback := func(sessionContext mongo.SessionContext) (interface{}, error) {
		var articles []models.Article
		var cursor *mongo.Cursor
		var err error
		if len(strings.TrimSpace(category)) == 0 {
			cursor, err = database.stockCollection.Find(ctx, bson.M{})
		} else {
			cursor, err = database.stockCollection.Find(ctx, bson.M{"category": strings.ToLower(category)})
		}
		if err != nil {
			return nil, err
		}
		defer func(cursor *mongo.Cursor, ctx context.Context) {
			err := cursor.Close(ctx)
			if err != nil {
				logger.WithError(err).Warn("cursor could not be closed!")
			}
		}(cursor, ctx)
		if err = cursor.All(ctx, &articles); err != nil {
			return nil, err
		}
		return articles, nil
	}
	return callback
}

func (database *DbConnection) reserveOrder(ctx context.Context, articleQuantityMap map[string]int, order models.Order) (*[]models.Article, error) {
	wc := writeconcern.New(writeconcern.WMajority())
	rc := readconcern.Snapshot()
	txnOpts := options.Transaction().SetWriteConcern(wc).SetReadConcern(rc)
	session, err := database.MongoClient.StartSession()
	if err != nil {
		return nil, err
	}
	defer session.EndSession(context.Background())

	callback := database.newCallbackReserveOrder(ctx, articleQuantityMap, order)

	result, err := session.WithTransaction(context.Background(), callback, txnOpts)
	if err != nil {
		return nil, err
	}
	articles := result.([]models.Article)
	return &articles, nil
}

func (database *DbConnection) newCallbackReserveOrder(ctx context.Context, articleQuantityMap map[string]int, order models.Order) func(sessionContext mongo.SessionContext) (interface{}, error) {
	callback := func(sessionContext mongo.SessionContext) (interface{}, error) {
		var articles []models.Article

		for articleID, amount := range articleQuantityMap {
			// get article
			var stockArticle = &models.Article{}
			hex, err := primitive.ObjectIDFromHex(articleID)
			if err != nil {
				return nil, err
			}
			if err := database.stockCollection.FindOne(ctx, bson.M{"_id": hex}).Decode(stockArticle); err != nil {
				return nil, err
			}
			var updatedAmount = stockArticle.Amount - int32(amount)
			if updatedAmount < 0 {
				return nil, fmt.Errorf("could not reserve article: %v, %v times. there is not enough on stock", stockArticle, amount)
			}
			stockArticle.Amount = updatedAmount
			articles = append(articles, *stockArticle)
			// update article
			result, err := database.stockCollection.ReplaceOne(
				ctx,
				bson.M{"_id": stockArticle.ID},
				stockArticle,
			)
			if err != nil {
				return nil, err
			}
			if result.ModifiedCount != 1 {
				err = fmt.Errorf("modified count %v != 1 for article: %v", result.ModifiedCount, stockArticle)
				logger.WithFields(loggrus.Fields{"article": stockArticle, "order": order}).WithError(err).Error("Could not update article")
				return nil, err
			}
		}
		// make reservation
		_, err := database.reservationCollection.InsertOne(ctx, order)
		if err != nil {
			return nil, err
		}

		return articles, nil
	}
	return callback
}

func (database *DbConnection) rollbackReserveOrder(ctx context.Context, order models.Order) error {
	articleQuantityMap := map[string]int{}
	for _, id := range order.Articles {
		_, exists := articleQuantityMap[id]
		if exists {
			articleQuantityMap[id]++
		} else {
			articleQuantityMap[id] = 1
		}
	}

	wc := writeconcern.New(writeconcern.WMajority())
	rc := readconcern.Snapshot()
	txnOpts := options.Transaction().SetWriteConcern(wc).SetReadConcern(rc)
	session, err := database.MongoClient.StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(context.Background())

	callback := database.newCallbackRollbackReserveOrder(ctx, articleQuantityMap, order)

	_, err = session.WithTransaction(context.Background(), callback, txnOpts)
	if err != nil {
		return err
	}

	return nil
}

func (database *DbConnection) newCallbackRollbackReserveOrder(ctx context.Context, articleQuantityMap map[string]int, order models.Order) func(sessionContext mongo.SessionContext) (interface{}, error) {
	callback := func(sessionContext mongo.SessionContext) (interface{}, error) {
		// undo reservation
		deleted, err := database.reservationCollection.DeleteOne(ctx, bson.M{"_id": order.ID})
		if err != nil {
			return nil, err
		}
		if deleted.DeletedCount != 1 {
			return nil, fmt.Errorf("there is no reservation for this order")
		}

		for articleID, amount := range articleQuantityMap {
			// get article
			var stockArticle = &models.Article{}
			hex, err := primitive.ObjectIDFromHex(articleID)
			if err != nil {
				return nil, err
			}
			if err := database.stockCollection.FindOne(ctx, bson.M{"_id": hex}).Decode(stockArticle); err != nil {
				return nil, err
			}
			var updatedAmount = stockArticle.Amount + int32(amount)
			if updatedAmount < 0 {
				return nil, fmt.Errorf("could not reserve article: %v, %v times. there is not enough on stock", stockArticle, amount)
			}
			stockArticle.Amount = updatedAmount
			// update article
			result, err := database.stockCollection.ReplaceOne(
				ctx,
				bson.M{"_id": stockArticle.ID},
				stockArticle,
			)
			if err != nil {
				return nil, err
			}
			if result.ModifiedCount != 1 {
				err = fmt.Errorf("modified count %v != 1 for article: %v", result.ModifiedCount, stockArticle)
				logger.WithFields(loggrus.Fields{"article": stockArticle, "order": order}).WithError(err).Error("Could not update article")
				return nil, err
			}
		}

		return nil, nil
	}
	return callback
}

func (database *DbConnection) deleteReservation(ctx context.Context, orderID primitive.ObjectID) error {

	wc := writeconcern.New(writeconcern.WMajority())
	rc := readconcern.Snapshot()
	txnOpts := options.Transaction().SetWriteConcern(wc).SetReadConcern(rc)
	session, err := database.MongoClient.StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(context.Background())

	callback := database.newCallbackDeleteReservation(ctx, orderID)

	_, err = session.WithTransaction(context.Background(), callback, txnOpts)
	if err != nil {
		return err
	}

	return nil

}

func (database *DbConnection) newCallbackDeleteReservation(ctx context.Context, orderID primitive.ObjectID) func(sessionContext mongo.SessionContext) (interface{}, error) {
	callback := func(sessionContext mongo.SessionContext) (interface{}, error) {
		// undo reservation
		deleted, err := database.reservationCollection.DeleteOne(ctx, bson.M{"_id": orderID})
		if err != nil {
			return nil, err
		}
		if deleted.DeletedCount != 1 {
			return nil, fmt.Errorf("there is no reservation for this order")
		}
		return nil, nil
	}
	return callback
}

func (database *DbConnection) rollbackDeleteReservation(ctx context.Context, orderID primitive.ObjectID) error {

	wc := writeconcern.New(writeconcern.WMajority())
	rc := readconcern.Snapshot()
	txnOpts := options.Transaction().SetWriteConcern(wc).SetReadConcern(rc)
	session, err := database.MongoClient.StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(context.Background())

	callback := database.newCallbackRollbackDeleteReservation(ctx, orderID)

	_, err = session.WithTransaction(context.Background(), callback, txnOpts)
	if err != nil {
		return err
	}

	return nil

}

func (database *DbConnection) newCallbackRollbackDeleteReservation(ctx context.Context, orderID primitive.ObjectID) func(sessionContext mongo.SessionContext) (interface{}, error) {
	callback := func(sessionContext mongo.SessionContext) (interface{}, error) {
		// undo reservation
		_, err := database.reservationCollection.InsertOne(ctx, bson.M{"_id": orderID})
		if err != nil {
			return nil, err
		}
		return nil, nil
	}
	return callback
}

func (database *DbConnection) restockArticle(ctx context.Context, paramArticleID string, amount int) error {
	articleID, err := primitive.ObjectIDFromHex(paramArticleID)
	if err != nil {
		return err
	}
	wc := writeconcern.New(writeconcern.WMajority())
	rc := readconcern.Snapshot()
	txnOpts := options.Transaction().SetWriteConcern(wc).SetReadConcern(rc)
	session, err := database.MongoClient.StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(context.Background())

	callback := database.newCallbackRestockArticle(ctx, articleID, amount)

	_, err = session.WithTransaction(context.Background(), callback, txnOpts)
	if err != nil {
		return err
	}

	return nil

}

func (database *DbConnection) newCallbackRestockArticle(ctx context.Context, articleID primitive.ObjectID, amount int) func(sessionContext mongo.SessionContext) (interface{}, error) {
	callback := func(sessionContext mongo.SessionContext) (interface{}, error) {
		// get article
		var stockArticle = &models.Article{}
		if err := database.stockCollection.FindOne(ctx, bson.M{"_id": articleID}).Decode(stockArticle); err != nil {
			return nil, err
		}
		stockArticle.Amount = stockArticle.Amount + int32(amount)
		// update article
		result, err := database.stockCollection.ReplaceOne(
			ctx,
			bson.M{"_id": stockArticle.ID},
			stockArticle,
		)
		if err != nil {
			return nil, err
		}
		if result.ModifiedCount != 1 {
			err = fmt.Errorf("modified count %v != 1 for article: %v", result.ModifiedCount, stockArticle)
			logger.WithFields(loggrus.Fields{"article": stockArticle}).WithError(err).Error("Could not update article")
			return nil, err
		}
		return nil, nil
	}
	return callback
}

func (database *DbConnection) rollbackRestockArticle(ctx context.Context, paramArticleID string, amount int) error {
	articleID, err := primitive.ObjectIDFromHex(paramArticleID)
	if err != nil {
		return err
	}
	wc := writeconcern.New(writeconcern.WMajority())
	rc := readconcern.Snapshot()
	txnOpts := options.Transaction().SetWriteConcern(wc).SetReadConcern(rc)
	session, err := database.MongoClient.StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(context.Background())

	callback := database.newCallbackRollbackRestockArticle(ctx, articleID, amount)

	_, err = session.WithTransaction(context.Background(), callback, txnOpts)
	if err != nil {
		return err
	}

	return nil

}

func (database *DbConnection) newCallbackRollbackRestockArticle(ctx context.Context, articleID primitive.ObjectID, amount int) func(sessionContext mongo.SessionContext) (interface{}, error) {
	callback := func(sessionContext mongo.SessionContext) (interface{}, error) {
		// get article
		var stockArticle = &models.Article{}
		if err := database.stockCollection.FindOne(ctx, bson.M{"_id": articleID}).Decode(stockArticle); err != nil {
			return nil, err
		}
		stockArticle.Amount = stockArticle.Amount - int32(amount)
		// update article
		result, err := database.stockCollection.ReplaceOne(
			ctx,
			bson.M{"_id": stockArticle.ID},
			stockArticle,
		)
		if err != nil {
			return nil, err
		}
		if result.ModifiedCount != 1 {
			err = fmt.Errorf("modified count %v != 1 for article: %v", result.ModifiedCount, stockArticle)
			logger.WithFields(loggrus.Fields{"article": stockArticle}).WithError(err).Error("Could not update article")
			return nil, err
		}
		return nil, nil
	}
	return callback
}
