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
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
	"time"
)

// DbConnection handles all database operations
type DbConnection struct {
	MongoClient     *mongo.Client
	orderCollection *mongo.Collection
}

func NewDbConnection(mongoAddress string, mongoPort string) *DbConnection {
	var err error
	db := &DbConnection{}
	// create mongodb client
	mongoUri := "mongodb://" + mongoAddress + ":" + mongoPort
	db.MongoClient, err = mongo.NewClient(options.Client().ApplyURI(mongoUri))
	if err != nil {
		logger.WithFields(logrus.Fields{"request": mongoUri}).WithError(err).Error("Could not connect to DB")
	}
	// connect to mongodb
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err = db.MongoClient.Connect(ctx)
	if err != nil {
		logger.WithFields(logrus.Fields{"request": mongoUri}).WithError(err).Error("Could not connect to DB")
	} else {
		logger.WithFields(logrus.Fields{"request": mongoUri}).Info("Connection to DB successfully!")
	}
	// fetch the collection for all order operations
	db.orderCollection = db.MongoClient.Database("mongo_db").Collection("order")
	return db
}

// createOrder acid compliant transaction to create an order
func (database *DbConnection) createOrder(ctx context.Context, order *models.Order) error {
	// configuration for transaction
	wc := writeconcern.New(writeconcern.WMajority())
	rc := readconcern.Snapshot()
	txnOpts := options.Transaction().SetWriteConcern(wc).SetReadConcern(rc)
	// open up session to mongodb
	session, err := database.MongoClient.StartSession()
	if err != nil {
		return err
	}
	// close session on return of this method
	defer session.EndSession(ctx)
	// get the callback function for the transaction
	callback := database.newCallbackCreateOrder(*order)
	// do the transaction
	result, err := session.WithTransaction(ctx, callback, txnOpts)
	if err != nil {
		return err
	}

	orderId := result.(primitive.ObjectID)
	order.ID = orderId
	return nil
}

// newCallbackCreateOrder callback for acid transaction in createOrder
func (database *DbConnection) newCallbackCreateOrder(order models.Order) func(sessionContext mongo.SessionContext) (interface{}, error) {
	callback := func(sessionContext mongo.SessionContext) (interface{}, error) {
		// insert order into order collection
		orderResult, err := database.orderCollection.InsertOne(sessionContext, order)
		if err != nil {
			return nil, err
		}
		return orderResult.InsertedID, nil
	}
	return callback
}

// getOrder acid compliant transaction to get an order
func (database *DbConnection) getOrder(ctx context.Context, orderId primitive.ObjectID) (*models.Order, error) {
	// configuration for transaction
	wc := writeconcern.New(writeconcern.WMajority())
	rc := readconcern.Snapshot()
	txnOpts := options.Transaction().SetWriteConcern(wc).SetReadConcern(rc)
	// open up session to mongodb
	session, err := database.MongoClient.StartSession()
	if err != nil {
		return nil, err
	}
	// close session on return of this method
	defer session.EndSession(ctx)
	// get the callback function for the transaction
	callback := database.newCallbackGetOrder(orderId)
	// do the transaction
	result, err := session.WithTransaction(ctx, callback, txnOpts)
	if err != nil {
		return nil, err
	}

	order := result.(models.Order)
	return &order, nil
}

// newCallbackGetOrder callback for acid transaction in getOrder
func (database *DbConnection) newCallbackGetOrder(orderId primitive.ObjectID) func(sessionContext mongo.SessionContext) (interface{}, error) {
	callback := func(sessionContext mongo.SessionContext) (interface{}, error) {
		// query collection for the orderId
		result := database.orderCollection.FindOne(sessionContext, bson.M{"_id": orderId})
		// decode the result into an order object
		order := models.Order{}
		err := result.Decode(&order)
		if err != nil {
			return nil, err
		}

		return order, nil
	}
	return callback
}

// updateOrder acid compliant transaction to create an order
func (database *DbConnection) updateOrder(ctx context.Context, order models.Order) error {
	// configuration for transaction
	wc := writeconcern.New(writeconcern.WMajority())
	rc := readconcern.Snapshot()
	txnOpts := options.Transaction().SetWriteConcern(wc).SetReadConcern(rc)
	// open up session to mongodb
	session, err := database.MongoClient.StartSession()
	if err != nil {
		return err
	}
	// close session on return of this method
	defer session.EndSession(ctx)
	// get the callback function for the transaction
	callback := database.newCallbackUpdateOrder(order)
	// do the transaction
	_, err = session.WithTransaction(ctx, callback, txnOpts)

	if err != nil {
		return err
	}
	return nil
}

// newCallbackUpdateOrder callback for acid transaction in updateOrder
func (database *DbConnection) newCallbackUpdateOrder(order models.Order) func(sessionContext mongo.SessionContext) (interface{}, error) {
	callback := func(sessionContext mongo.SessionContext) (interface{}, error) {
		// replace order with matching ID
		result, err := database.orderCollection.ReplaceOne(
			sessionContext,
			bson.M{"_id": order.ID},
			order,
		)
		if err != nil {
			return nil, err
		}

		// throw error if no order was updated
		if result.ModifiedCount != 1 {
			err = fmt.Errorf("modified count %v != 1 for order with id: %v", result.ModifiedCount, order.ID)
			return nil, err
		}
		return result, nil
	}
	return callback
}

// getAndUpdateOrder acid compliant transaction to update an order. Returns the old overwritten order.
func (database *DbConnection) getAndUpdateOrder(ctx context.Context, order models.Order) (*models.Order, error) {
	// configuration for transaction
	wc := writeconcern.New(writeconcern.WMajority())
	rc := readconcern.Snapshot()
	txnOpts := options.Transaction().SetWriteConcern(wc).SetReadConcern(rc)
	// open up session to mongodb
	session, err := database.MongoClient.StartSession()
	if err != nil {
		return nil, err
	}
	// close session on return of this method
	defer session.EndSession(ctx)
	// get the callback function for the transaction
	callback := database.newCallbackGetAndUpdateOrder(order)
	// do the transaction
	result, err := session.WithTransaction(ctx, callback, txnOpts)
	if err != nil {
		return nil, err
	}

	order = result.(models.Order)
	return &order, nil
}

// newCallbackGetAndUpdateOrder callback for acid transaction in getAndUpdateOrder
func (database *DbConnection) newCallbackGetAndUpdateOrder(order models.Order) func(sessionContext mongo.SessionContext) (interface{}, error) {
	callback := func(sessionContext mongo.SessionContext) (interface{}, error) {
		// query collection for the orderId
		result := database.orderCollection.FindOne(sessionContext, bson.M{"_id": order.ID})
		// decode the result into an order object
		oldOrder := models.Order{}
		err := result.Decode(&oldOrder)
		if err != nil {
			return nil, err
		}

		if !models.IsProgressive(order.Status, oldOrder.Status) {
			return nil, errors.Wrap(customerrors.ErrStatusProgressionConflict, fmt.Sprintf("current status: %v, New requested status: %v", oldOrder.Status, order.Status))
		}

		// replace order with matching ID
		result2, err := database.orderCollection.ReplaceOne(
			sessionContext,
			bson.M{"_id": order.ID},
			order,
		)
		if err != nil {
			return nil, err
		}

		// throw error if no order was updated
		if result2.ModifiedCount != 1 {
			err = fmt.Errorf("modified count %v != 1 for order with id: %v", result2.ModifiedCount, order.ID)
			return nil, err
		}

		// return oldOrder for rollback
		return oldOrder, nil
	}
	return callback
}
