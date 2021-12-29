package internal

import (
	"fmt"
	"github.com/Tobias-Pe/Microservices-Errorhandling/pkg/models"
	"github.com/gomodule/redigo/redis"
	"strconv"
)

// DbConnection handles all database operation
type DbConnection struct {
	connPool *redis.Pool
}

func (database DbConnection) getCart(strCartId string) (*models.Cart, error) {
	tmpId, err := strconv.Atoi(strCartId)
	if err != nil {
		return nil, err
	}
	id := int64(tmpId)
	fetchedCart := models.Cart{
		ID:         id,
		ArticleIDs: nil,
	}

	// get a client out of the connection pool
	client := database.connPool.Get()
	defer func(client redis.Conn) {
		err := client.Close()
		if err != nil {
			logger.WithError(err).Warn("connection to redis could not be successfully closed")
		}
	}(client)

	// article ids are stored as list behind the cartID key
	jsonArticles, err := redis.ByteSlices(client.Do("LRANGE", id, 0, -1))
	if err != nil {
		return nil, err
	}
	if len(jsonArticles) == 0 { // check empty slice
		return nil, fmt.Errorf("there is no cart for this id: %s", strCartId)
	}
	// populate return value
	var articles []string
	for _, jsonArticle := range jsonArticles {
		articles = append(articles, string(jsonArticle))
	}
	if err != nil {
		return nil, err
	}
	fetchedCart.ArticleIDs = articles
	return &fetchedCart, nil
}

func (database DbConnection) createCart(strArticleId string) (*models.Cart, error) {
	newCart := models.Cart{
		ID:         -1,
		ArticleIDs: []string{strArticleId},
	}

	// get a client out of the connection pool
	client := database.connPool.Get()
	defer func(client redis.Conn) {
		err := client.Close()
		if err != nil {
			logger.WithError(err).Warn("connection to redis could not be successfully closed")
		}
	}(client)

	// fetch and increment id counter --> this will be the new cart's id
	cartID, err := client.Do("INCR", "cartID")
	if err != nil {
		return nil, err
	}
	newCart.ID = cartID.(int64)

	// RPUSH in this scenario will init a list of values behind the cartID
	_, err = client.Do("RPUSH", cartID, newCart.ArticleIDs[0])
	if err != nil {
		return nil, err
	}
	// set expiration for the cart
	_, err = client.Do("EXPIRE", cartID, expireCartSeconds)
	if err != nil {
		return nil, err
	}

	return &newCart, nil
}

func (database DbConnection) addToCart(strCartId string, strArticleId string) (*int64, error) {
	iCartID, err := strconv.Atoi(strCartId)
	if err != nil {
		return nil, err
	}
	cartID := int64(iCartID)

	// get a client out of the connection pool
	client := database.connPool.Get()
	defer func(client redis.Conn) {
		err := client.Close()
		if err != nil {
			logger.WithError(err).Warn("connection to redis could not be successfully closed")
		}
	}(client)

	// Push article to end of the list of values behind key cartID
	length, err := client.Do("RPUSH", cartID, strArticleId)
	if err != nil {
		return nil, err
	}
	// reset expiration timer, because the cart is still in use
	_, err = client.Do("EXPIRE", cartID, expireCartSeconds)
	if err != nil {
		return nil, err
	}

	index := length.(int64) - 1
	return &index, nil
}

func (database DbConnection) removeFromCart(strCartId string, index int64) error {
	iCartID, err := strconv.Atoi(strCartId)
	if err != nil {
		return err
	}
	cartID := int64(iCartID)

	// get a client out of the connection pool
	client := database.connPool.Get()
	defer func(client redis.Conn) {
		err := client.Close()
		if err != nil {
			logger.WithError(err).Warn("connection to redis could not be successfully closed")
		}
	}(client)

	// MULTI will collect all transaction until EXEC is called
	err = client.Send("MULTI")
	if err != nil {
		return err
	}
	// flag value
	err = client.Send("LSET", cartID, index, "TOBEREMOVED")
	if err != nil {
		return err
	}
	// remove flagged value
	err = client.Send("LREM", cartID, 1, "TOBEREMOVED")
	if err != nil {
		return err
	}
	// reset expiration timer, because the cart is still in use
	err = client.Send("EXPIRE", cartID, expireCartSeconds)
	if err != nil {
		return err
	}
	// execute the multi transactions call
	_, err = redis.Values(client.Do("EXEC"))
	if err != nil {
		return err
	}
	return nil
}
