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
	customerrors "github.com/Tobias-Pe/Microservices-Errorhandling/pkg/custom-errors"
	"github.com/Tobias-Pe/Microservices-Errorhandling/pkg/models"
	"github.com/gomodule/redigo/redis"
	"github.com/pkg/errors"
	"strconv"
)

// DbConnection handles all database operation
type DbConnection struct {
	connPool *redis.Pool
}

// NewDbConnection creates a connection to redis and makes an object (DbConnection) to interact with redis
func NewDbConnection(cacheAddress string, cachePort string) *DbConnection {
	connectionUri := cacheAddress + ":" + cachePort
	database := &DbConnection{}
	// connection pool to redis
	pool := redis.Pool{
		MaxIdle:   80,
		MaxActive: 12000,
		Dial: func() (redis.Conn, error) {
			c, err := redis.Dial("tcp", connectionUri)
			if err != nil {
				logger.WithError(err).Error("Error appeared on dialing redis!")
			}
			return c, err
		},
	}
	conn := pool.Get()
	defer func(conn redis.Conn) {
		err := conn.Close()
		if err != nil {
			logger.WithError(err).Error("connection to redis could not be successfully closed")
		}
	}(conn)
	_, err := conn.Do("PING")
	if err != nil {
		return nil
	}
	database.connPool = &pool
	return database
}

func (database DbConnection) getCart(ctx context.Context, strCartId string) (*models.Cart, error) {
	tmpId, err := strconv.Atoi(strCartId)
	if err != nil {
		return nil, errors.Wrap(customerrors.ErrCartIdInvalid, err.Error())
	}
	id := int64(tmpId)
	fetchedCart := models.Cart{
		ID:         id,
		ArticleIDs: nil,
	}

	// get a client out of the connection pool
	client, err := database.connPool.GetContext(ctx)
	if err != nil {
		return nil, err
	}
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
		return nil, customerrors.ErrNoCartFound
	}

	// populate return value
	var articles []string
	for _, jsonArticle := range jsonArticles {
		articles = append(articles, string(jsonArticle))
	}
	fetchedCart.ArticleIDs = articles
	return &fetchedCart, nil
}

func (database DbConnection) createCart(ctx context.Context, strArticleId string) (*models.Cart, error) {
	newCart := models.Cart{
		ID:         -1,
		ArticleIDs: []string{strArticleId},
	}

	// get a client out of the connection pool
	client, err := database.connPool.GetContext(ctx)
	if err != nil {
		return nil, err
	}
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

func (database DbConnection) addToCart(ctx context.Context, strCartId string, strArticleId string) (*int64, error) {
	iCartID, err := strconv.Atoi(strCartId)
	if err != nil {
		return nil, err
	}
	cartID := int64(iCartID)

	// get a client out of the connection pool
	client, err := database.connPool.GetContext(ctx)
	if err != nil {
		return nil, err
	}
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
