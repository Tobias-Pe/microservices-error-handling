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

type DbConnection struct {
	MongoClient     *mongo.Client
	orderCollection *mongo.Collection
}

func NewDbConnection(mongoAddress string, mongoPort string) *DbConnection {
	var err error = nil
	db := &DbConnection{}
	mongoUri := "mongodb://" + mongoAddress + ":" + mongoPort
	db.MongoClient, err = mongo.NewClient(options.Client().ApplyURI(mongoUri))
	if err != nil {
		logger.WithFields(loggrus.Fields{"mongoURI": mongoUri}).WithError(err).Errorf("Could not connect to DB")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err = db.MongoClient.Connect(ctx)
	if err != nil {
		logger.WithFields(loggrus.Fields{"mongoURI": mongoUri}).WithError(err).Errorf("Could not connect to DB")
	} else {
		logger.WithFields(loggrus.Fields{"mongoURI": mongoUri}).Info("Connection to DB successfully!")
	}
	db.orderCollection = db.MongoClient.Database("mongo_db").Collection("order")
	return db
}

func (database *DbConnection) createOrder(ctx context.Context, order *models.Order) error {
	wc := writeconcern.New(writeconcern.WMajority())
	rc := readconcern.Snapshot()
	txnOpts := options.Transaction().SetWriteConcern(wc).SetReadConcern(rc)
	session, err := database.MongoClient.StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(context.Background())

	callback := database.newCallbackCreateOrder(ctx, *order)

	result, err := session.WithTransaction(context.Background(), callback, txnOpts)
	if err != nil {
		return err
	}
	orderId := result.(primitive.ObjectID)
	order.ID = orderId
	return nil
}

func (database *DbConnection) newCallbackCreateOrder(ctx context.Context, order models.Order) func(sessionContext mongo.SessionContext) (interface{}, error) {
	callback := func(sessionContext mongo.SessionContext) (interface{}, error) {
		orderResult, err := database.orderCollection.InsertOne(ctx, order)
		if err != nil {
			return nil, err
		}
		return orderResult.InsertedID, nil
	}
	return callback
}

func (database *DbConnection) getOrder(ctx context.Context, orderId primitive.ObjectID) (*models.Order, error) {
	wc := writeconcern.New(writeconcern.WMajority())
	rc := readconcern.Snapshot()
	txnOpts := options.Transaction().SetWriteConcern(wc).SetReadConcern(rc)
	session, err := database.MongoClient.StartSession()
	if err != nil {
		return nil, err
	}
	defer session.EndSession(context.Background())

	callback := database.newCallbackGetOrder(ctx, orderId)
	result, err := session.WithTransaction(context.Background(), callback, txnOpts)
	if err != nil {
		return nil, err
	}
	order := result.(models.Order)
	return &order, nil
}

func (database *DbConnection) newCallbackGetOrder(ctx context.Context, orderId primitive.ObjectID) func(sessionContext mongo.SessionContext) (interface{}, error) {
	callback := func(sessionContext mongo.SessionContext) (interface{}, error) {
		result := database.orderCollection.FindOne(ctx, bson.M{"_id": orderId})
		order := models.Order{}
		err := result.Decode(&order)
		if err != nil {
			return nil, err
		}
		return order, nil
	}
	return callback
}

func (database *DbConnection) updateOrder(ctx context.Context, order models.Order) error {
	wc := writeconcern.New(writeconcern.WMajority())
	rc := readconcern.Snapshot()
	txnOpts := options.Transaction().SetWriteConcern(wc).SetReadConcern(rc)
	session, err := database.MongoClient.StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(context.Background())

	callback := database.newCallbackUpdateOrder(ctx, order)
	modifiedCount, err := session.WithTransaction(context.Background(), callback, txnOpts)
	if err != nil {
		return err
	}
	if modifiedCount.(int64) != 1 {
		err = fmt.Errorf("modified count is not one: %v for order with id: %v", modifiedCount.(int64), order.ID)
		logger.WithFields(loggrus.Fields{"order": order}).WithError(err).Errorf("Could not update order")
		return err
	}
	return nil
}

func (database *DbConnection) newCallbackUpdateOrder(ctx context.Context, order models.Order) func(sessionContext mongo.SessionContext) (interface{}, error) {
	callback := func(sessionContext mongo.SessionContext) (interface{}, error) {
		result, err := database.orderCollection.ReplaceOne(
			ctx,
			bson.M{"_id": order.ID},
			order,
		)
		if err != nil {
			return nil, err
		}
		return result.ModifiedCount, nil
	}
	return callback
}
