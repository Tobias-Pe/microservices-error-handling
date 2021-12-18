package internal

import (
	"fmt"
	"github.com/Tobias-Pe/Microservices-Errorhandling/pkg/models"
	"github.com/gomodule/redigo/redis"
	"strconv"
)

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

	client := database.connPool.Get()
	defer func(client redis.Conn) {
		err := client.Close()
		if err != nil {
			logger.WithError(err).Warn("connection to redis could not be successfully closed")
		}
	}(client)
	jsonArticles, err := redis.ByteSlices(client.Do("LRANGE", id, 0, -1))
	if err != nil {
		return nil, err
	}
	if len(jsonArticles) == 0 {
		return nil, fmt.Errorf("there is no cart for this id: %s", strCartId)
	}
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

	client := database.connPool.Get()
	defer func(client redis.Conn) {
		err := client.Close()
		if err != nil {
			logger.WithError(err).Warn("connection to redis could not be successfully closed")
		}
	}(client)

	cartId, err := client.Do("INCR", "cartId")
	if err != nil {
		return nil, err
	}

	newCart.ID = cartId.(int64)
	_, err = client.Do("RPUSH", cartId, newCart.ArticleIDs[0])
	if err != nil {
		return nil, err
	}
	_, err = client.Do("EXPIRE", cartId, expireCartSeconds)
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
	client := database.connPool.Get()
	defer func(client redis.Conn) {
		err := client.Close()
		if err != nil {
			logger.WithError(err).Warn("connection to redis could not be successfully closed")
		}
	}(client)

	length, err := client.Do("RPUSH", cartID, strArticleId)
	if err != nil {
		return nil, err
	}
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
	client := database.connPool.Get()
	defer func(client redis.Conn) {
		err := client.Close()
		if err != nil {
			logger.WithError(err).Warn("connection to redis could not be successfully closed")
		}
	}(client)

	err = client.Send("MULTI")
	if err != nil {
		return err
	}
	err = client.Send("LSET", cartID, index, "TOBEREMOVED")
	if err != nil {
		return err
	}
	err = client.Send("LREM", cartID, 1, "TOBEREMOVED")
	if err != nil {
		return err
	}
	err = client.Send("EXPIRE", cartID, expireCartSeconds)
	if err != nil {
		return err
	}
	_, err = redis.Values(client.Do("EXEC"))
	if err != nil {
		return err
	}
	return nil
}
