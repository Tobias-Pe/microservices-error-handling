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
	"encoding/json"
	"github.com/Tobias-Pe/Microservices-Errorhandling/pkg/models"
	"github.com/gomodule/redigo/redis"
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

// updateArticle saves the article data under its id and indexes the id under all articles and its category
func (database DbConnection) updateArticle(article *models.Article) error {
	// get a client out of the connection pool
	client := database.connPool.Get()
	defer func(client redis.Conn) {
		err := client.Close()
		if err != nil {
			logger.WithError(err).Warn("connection to redis could not be successfully closed")
		}
	}(client)

	// add to all
	_, err := client.Do("SADD", "articles", article.ID.Hex())
	if err != nil {
		return err
	}

	// add to category
	_, err = client.Do("SADD", article.Category, article.ID.Hex())
	if err != nil {
		return err
	}

	// marshall object data
	jsonArticle, err := json.Marshal(*article)
	if err != nil {
		return err
	}

	// save object data under article id
	_, err = client.Do("SET", article.ID.Hex(), string(jsonArticle))
	if err != nil {
		return err
	}

	return nil
}

// getArticles gets all articles if category is empty or the requested categories articles
func (database DbConnection) getArticles(category string) (*[]models.Article, error) {
	// get a client out of the connection pool
	client := database.connPool.Get()
	defer func(client redis.Conn) {
		err := client.Close()
		if err != nil {
			logger.WithError(err).Warn("connection to redis could not be successfully closed")
		}
	}(client)
	var articleIDs []interface{}

	if len(category) == 0 { // get all articles
		response, err := client.Do("SMEMBERS", "articles")
		if err != nil {
			return nil, err
		}
		articleIDs = response.([]interface{})
	} else { // get categories articles
		response, err := client.Do("SMEMBERS", category)
		if err != nil {
			return nil, err
		}
		articleIDs = response.([]interface{})
	}

	var articles []models.Article
	// populate articles array
	for _, interfaceArticleID := range articleIDs {
		articleID := string(interfaceArticleID.([]byte))
		response, err := client.Do("GET", articleID)
		if err != nil {
			return nil, err
		}
		jsonArticle := response.([]byte)
		var article models.Article
		err = json.Unmarshal(jsonArticle, &article)
		if err != nil {
			return nil, err
		}

		articles = append(articles, article)
	}

	return &articles, nil
}
