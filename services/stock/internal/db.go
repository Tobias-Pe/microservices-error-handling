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
	"github.com/Tobias-Pe/Microservices-Errorhandling/pkg/custom-errors"
	"github.com/Tobias-Pe/Microservices-Errorhandling/pkg/models"
	"github.com/pkg/errors"
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
		logger.WithFields(loggrus.Fields{"request": mongoUri}).WithError(err).Error("Could not connect to DB")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err = db.MongoClient.Connect(ctx)
	if err != nil {
		logger.WithFields(loggrus.Fields{"request": mongoUri}).WithError(err).Error("Could not connect to DB")
	} else {
		logger.WithFields(loggrus.Fields{"request": mongoUri}).Info("Connection to DB successfully!")
	}
	db.stockCollection = db.MongoClient.Database("mongo_db").Collection("stock")
	db.reservationCollection = db.MongoClient.Database("mongo_db").Collection("reservation")
	return db
}

func (database *DbConnection) getArticles(ctx context.Context, category string) (*[]models.Article, error) {
	// configuration for transaction
	wc := writeconcern.New(writeconcern.WMajority())
	rc := readconcern.Snapshot()
	txnOpts := options.Transaction().SetWriteConcern(wc).SetReadConcern(rc)

	session, err := database.MongoClient.StartSession()
	if err != nil {
		return nil, err
	}
	defer session.EndSession(ctx)

	callback := database.newCallbackGetArticles(category)

	result, err := session.WithTransaction(ctx, callback, txnOpts)
	if err != nil {
		return nil, err
	}
	articles := result.([]models.Article)
	return &articles, nil
}

func (database *DbConnection) newCallbackGetArticles(category string) func(sessionContext mongo.SessionContext) (interface{}, error) {
	callback := func(sessionContext mongo.SessionContext) (interface{}, error) {
		var err error

		var articles []models.Article
		var cursor *mongo.Cursor
		if len(strings.TrimSpace(category)) == 0 {
			cursor, err = database.stockCollection.Find(sessionContext, bson.M{})
		} else {
			cursor, err = database.stockCollection.Find(sessionContext, bson.M{"category": strings.ToLower(category)})
		}
		if err != nil {
			return nil, err
		}

		defer func(cursor *mongo.Cursor, ctx context.Context) {
			err := cursor.Close(ctx)
			if err != nil {
				logger.WithError(err).Warn("cursor could not be closed!")
			}
		}(cursor, sessionContext)

		if err = cursor.All(sessionContext, &articles); err != nil {
			return nil, err
		}

		return articles, nil
	}
	return callback
}

func (database *DbConnection) reserveOrder(ctx context.Context, articleQuantityMap map[string]int, order models.Order) (*[]models.Article, error) {
	// configuration for transaction
	wc := writeconcern.New(writeconcern.WMajority())
	rc := readconcern.Snapshot()
	txnOpts := options.Transaction().SetWriteConcern(wc).SetReadConcern(rc)
	session, err := database.MongoClient.StartSession()
	if err != nil {
		return nil, err
	}
	defer session.EndSession(ctx)

	callback := database.newCallbackReserveOrder(articleQuantityMap, order)

	result, err := session.WithTransaction(ctx, callback, txnOpts)
	if err != nil {
		return nil, err
	}
	articles := result.([]models.Article)
	return &articles, nil
}

func (database *DbConnection) newCallbackReserveOrder(articleQuantityMap map[string]int, order models.Order) func(sessionContext mongo.SessionContext) (interface{}, error) {
	callback := func(sessionContext mongo.SessionContext) (interface{}, error) {
		// check if already exists
		result := database.reservationCollection.FindOne(sessionContext, bson.M{"_id": order.ID})
		if result.Err() == nil { // found a match --> bad
			return nil, customerrors.ErrAlreadyPresent
		}

		var articles []models.Article

		// get all articles
		for articleID, amount := range articleQuantityMap {
			// get article
			var stockArticle = &models.Article{}
			hex, err := primitive.ObjectIDFromHex(articleID)
			if err != nil {
				return nil, err
			}
			if err := database.stockCollection.FindOne(sessionContext, bson.M{"_id": hex}).Decode(stockArticle); err != nil {
				return nil, err
			}
			var updatedAmount = stockArticle.Amount - int32(amount)
			if updatedAmount < 0 {
				return nil, errors.Wrap(customerrors.ErrLowStock, fmt.Sprintf("article: %v, amount: %v", articleID, amount))
			}
			stockArticle.Amount = updatedAmount
			articles = append(articles, *stockArticle)
			// update article
			result, err := database.stockCollection.ReplaceOne(
				sessionContext,
				bson.M{"_id": stockArticle.ID},
				stockArticle,
			)
			if err != nil {
				return nil, err
			}
			if result.ModifiedCount != 1 {
				msg := fmt.Sprintf("modified count %v != 1 for article: %v", result.ModifiedCount, stockArticle)
				err = errors.Wrap(customerrors.ErrNoModification, msg)
				logger.WithFields(loggrus.Fields{"request": order}).WithError(err).Error("Could not update article")
				return nil, err
			}
		}

		// make reservation
		result2, err := database.reservationCollection.InsertOne(sessionContext, order)
		if err != nil {
			return nil, err
		}
		if result2.InsertedID == nil {
			msg := fmt.Sprintf("insert not worked for order: %v", order)
			err = errors.Wrap(customerrors.ErrNoModification, msg)
			logger.WithFields(loggrus.Fields{"request": order}).WithError(err).Error("Could not insert order into reservations")
			return nil, err
		}

		return articles, nil
	}
	return callback
}

func (database *DbConnection) rollbackReserveOrder(ctx context.Context, order models.Order) (*[]models.Article, error) {
	articleQuantityMap := map[string]int{}
	for _, id := range order.Articles {
		_, exists := articleQuantityMap[id]
		if exists {
			articleQuantityMap[id]++
		} else {
			articleQuantityMap[id] = 1
		}
	}

	// configuration for transaction
	wc := writeconcern.New(writeconcern.WMajority())
	rc := readconcern.Snapshot()
	txnOpts := options.Transaction().SetWriteConcern(wc).SetReadConcern(rc)
	session, err := database.MongoClient.StartSession()
	if err != nil {
		return nil, err
	}
	defer session.EndSession(ctx)

	callback := database.newCallbackRollbackReserveOrder(articleQuantityMap, order)

	result, err := session.WithTransaction(ctx, callback, txnOpts)
	if err != nil {
		return nil, err
	}

	articles := result.([]models.Article)
	return &articles, nil
}

func (database *DbConnection) newCallbackRollbackReserveOrder(articleQuantityMap map[string]int, order models.Order) func(sessionContext mongo.SessionContext) (interface{}, error) {
	callback := func(sessionContext mongo.SessionContext) (interface{}, error) {
		// undo reservation
		deleted, err := database.reservationCollection.DeleteOne(sessionContext, bson.M{"_id": order.ID})
		if err != nil {
			return nil, err
		}

		if deleted.DeletedCount != 1 {
			return nil, customerrors.ErrNoReservation
		}

		var articles []models.Article

		for articleID, amount := range articleQuantityMap {
			// get article
			var stockArticle = &models.Article{}
			hex, err := primitive.ObjectIDFromHex(articleID)
			if err != nil {
				return nil, err
			}

			if err := database.stockCollection.FindOne(sessionContext, bson.M{"_id": hex}).Decode(stockArticle); err != nil {
				return nil, err
			}

			var updatedAmount = stockArticle.Amount + int32(amount)
			stockArticle.Amount = updatedAmount
			// update article
			result, err := database.stockCollection.ReplaceOne(
				sessionContext,
				bson.M{"_id": stockArticle.ID},
				stockArticle,
			)
			if err != nil {
				return nil, err
			}
			if result.ModifiedCount != 1 {
				msg := fmt.Sprintf("modified count %v != 1 for article: %v", result.ModifiedCount, stockArticle)
				err = errors.Wrap(customerrors.ErrNoModification, msg)
				logger.WithFields(loggrus.Fields{"request": order}).WithError(err).Error("Could not update article")
				return nil, err
			}
			articles = append(articles, *stockArticle)
		}

		return articles, nil
	}
	return callback
}

func (database *DbConnection) deleteReservation(ctx context.Context, orderID primitive.ObjectID) error {
	// configuration for transaction
	wc := writeconcern.New(writeconcern.WMajority())
	rc := readconcern.Snapshot()
	txnOpts := options.Transaction().SetWriteConcern(wc).SetReadConcern(rc)
	session, err := database.MongoClient.StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(ctx)

	callback := database.newCallbackDeleteReservation(orderID)

	_, err = session.WithTransaction(ctx, callback, txnOpts)
	if err != nil {
		return err
	}

	return nil

}

func (database *DbConnection) newCallbackDeleteReservation(orderID primitive.ObjectID) func(sessionContext mongo.SessionContext) (interface{}, error) {
	callback := func(sessionContext mongo.SessionContext) (interface{}, error) {
		// undo reservation
		deleted, err := database.reservationCollection.DeleteOne(sessionContext, bson.M{"_id": orderID})
		if err != nil {
			return nil, err
		}

		if deleted.DeletedCount != 1 {
			return nil, customerrors.ErrNoReservation
		}
		return nil, nil
	}
	return callback
}

func (database *DbConnection) rollbackDeleteReservation(ctx context.Context, order *models.Order) error {
	// configuration for transaction
	wc := writeconcern.New(writeconcern.WMajority())
	rc := readconcern.Snapshot()
	txnOpts := options.Transaction().SetWriteConcern(wc).SetReadConcern(rc)
	session, err := database.MongoClient.StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(ctx)

	callback := database.newCallbackRollbackDeleteReservation(order)

	_, err = session.WithTransaction(ctx, callback, txnOpts)
	if err != nil {
		return err
	}

	return nil

}

func (database *DbConnection) newCallbackRollbackDeleteReservation(order *models.Order) func(sessionContext mongo.SessionContext) (interface{}, error) {
	callback := func(sessionContext mongo.SessionContext) (interface{}, error) {
		// check if already exists
		result := database.reservationCollection.FindOne(sessionContext, bson.M{"_id": order.ID})
		if result.Err() == nil { // found a match --> bad
			return nil, customerrors.ErrAlreadyPresent
		}

		// redo reservation
		_, err := database.reservationCollection.InsertOne(sessionContext, order)
		if err != nil {
			return nil, err
		}
		return nil, nil
	}
	return callback
}

func (database *DbConnection) restockArticle(ctx context.Context, paramArticleID string, amount int) (*models.Article, error) {
	articleID, err := primitive.ObjectIDFromHex(paramArticleID)
	if err != nil {
		return nil, err
	}
	// configuration for transaction
	wc := writeconcern.New(writeconcern.WMajority())
	rc := readconcern.Snapshot()
	txnOpts := options.Transaction().SetWriteConcern(wc).SetReadConcern(rc)
	session, err := database.MongoClient.StartSession()
	if err != nil {
		return nil, err
	}
	defer session.EndSession(ctx)

	callback := database.newCallbackRestockArticle(articleID, amount)

	result, err := session.WithTransaction(ctx, callback, txnOpts)
	if err != nil {
		return nil, err
	}

	article := result.(models.Article)
	return &article, nil
}

func (database *DbConnection) newCallbackRestockArticle(articleID primitive.ObjectID, amount int) func(sessionContext mongo.SessionContext) (interface{}, error) {
	callback := func(sessionContext mongo.SessionContext) (interface{}, error) {
		// get article
		var stockArticle = &models.Article{}
		if err := database.stockCollection.FindOne(sessionContext, bson.M{"_id": articleID}).Decode(stockArticle); err != nil {
			return nil, err
		}
		stockArticle.Amount = stockArticle.Amount + int32(amount)
		// update article
		result, err := database.stockCollection.ReplaceOne(
			sessionContext,
			bson.M{"_id": stockArticle.ID},
			stockArticle,
		)
		if err != nil {
			return nil, err
		}
		if result.ModifiedCount != 1 {
			msg := fmt.Sprintf("modified count %v != 1 for article: %v", result.ModifiedCount, stockArticle)
			err = errors.Wrap(customerrors.ErrNoModification, msg)
			logger.WithFields(loggrus.Fields{"request": stockArticle}).WithError(err).Error("Could not update article")
			return nil, err
		}
		return *stockArticle, nil
	}
	return callback
}

func (database *DbConnection) rollbackRestockArticle(ctx context.Context, paramArticleID string, amount int) error {
	articleID, err := primitive.ObjectIDFromHex(paramArticleID)
	if err != nil {
		return err
	}
	// configuration for transaction
	wc := writeconcern.New(writeconcern.WMajority())
	rc := readconcern.Snapshot()
	txnOpts := options.Transaction().SetWriteConcern(wc).SetReadConcern(rc)
	session, err := database.MongoClient.StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(ctx)

	callback := database.newCallbackRollbackRestockArticle(articleID, amount)

	_, err = session.WithTransaction(ctx, callback, txnOpts)
	if err != nil {
		return err
	}

	return nil

}

func (database *DbConnection) newCallbackRollbackRestockArticle(articleID primitive.ObjectID, amount int) func(sessionContext mongo.SessionContext) (interface{}, error) {
	callback := func(sessionContext mongo.SessionContext) (interface{}, error) {
		// get article
		var stockArticle = &models.Article{}
		if err := database.stockCollection.FindOne(sessionContext, bson.M{"_id": articleID}).Decode(stockArticle); err != nil {
			return nil, err
		}
		stockArticle.Amount = stockArticle.Amount - int32(amount)
		// update article
		result, err := database.stockCollection.ReplaceOne(
			sessionContext,
			bson.M{"_id": stockArticle.ID},
			stockArticle,
		)
		if err != nil {
			return nil, err
		}
		if result.ModifiedCount != 1 {
			msg := fmt.Sprintf("modified count %v != 1 for article: %v", result.ModifiedCount, stockArticle)
			err = errors.Wrap(customerrors.ErrNoModification, msg)
			logger.WithFields(loggrus.Fields{"request": stockArticle}).WithError(err).Error("Could not update article")
			return nil, err
		}
		return nil, nil
	}
	return callback
}
