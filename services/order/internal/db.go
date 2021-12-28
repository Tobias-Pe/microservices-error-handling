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
		logger.WithFields(loggrus.Fields{"mongoURI": mongoUri}).WithError(err).Error("Could not connect to DB")
	}
	// connect to mongodb
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err = db.MongoClient.Connect(ctx)
	if err != nil {
		logger.WithFields(loggrus.Fields{"mongoURI": mongoUri}).WithError(err).Error("Could not connect to DB")
	} else {
		logger.WithFields(loggrus.Fields{"mongoURI": mongoUri}).Info("Connection to DB successfully!")
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
	defer session.EndSession(context.Background())
	// get the callback function for the transaction
	callback := database.newCallbackCreateOrder(ctx, *order)
	// do the transaction
	result, err := session.WithTransaction(context.Background(), callback, txnOpts)
	if err != nil {
		return err
	}

	orderId := result.(primitive.ObjectID)
	order.ID = orderId
	return nil
}

// newCallbackCreateOrder callback for acid transaction in createOrder
func (database *DbConnection) newCallbackCreateOrder(ctx context.Context, order models.Order) func(sessionContext mongo.SessionContext) (interface{}, error) {
	callback := func(sessionContext mongo.SessionContext) (interface{}, error) {
		// insert order into order collection
		orderResult, err := database.orderCollection.InsertOne(ctx, order)
		if err != nil {
			return nil, err
		}
		return orderResult.InsertedID, nil
	}
	return callback
}

// createOrder acid compliant transaction to get an order
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
	defer session.EndSession(context.Background())
	// get the callback function for the transaction
	callback := database.newCallbackGetOrder(ctx, orderId)
	// do the transaction
	result, err := session.WithTransaction(context.Background(), callback, txnOpts)
	if err != nil {
		return nil, err
	}

	order := result.(models.Order)
	return &order, nil
}

// newCallbackGetOrder callback for acid transaction in getOrder
func (database *DbConnection) newCallbackGetOrder(ctx context.Context, orderId primitive.ObjectID) func(sessionContext mongo.SessionContext) (interface{}, error) {
	callback := func(sessionContext mongo.SessionContext) (interface{}, error) {
		// query collection for the orderId
		result := database.orderCollection.FindOne(ctx, bson.M{"_id": orderId})
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

// createOrder acid compliant transaction to update an order
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
	defer session.EndSession(context.Background())
	// get the callback function for the transaction
	callback := database.newCallbackUpdateOrder(ctx, order)
	// do the transaction
	_, err = session.WithTransaction(context.Background(), callback, txnOpts)

	if err != nil {
		return err
	}
	return nil
}

// newCallbackUpdateOrder callback for acid transaction in updateOrder
func (database *DbConnection) newCallbackUpdateOrder(ctx context.Context, order models.Order) func(sessionContext mongo.SessionContext) (interface{}, error) {
	callback := func(sessionContext mongo.SessionContext) (interface{}, error) {
		// replace order with matching ID
		result, err := database.orderCollection.ReplaceOne(
			ctx,
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
