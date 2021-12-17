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

package main

import (
	loggingUtil "github.com/Tobias-Pe/Microservices-Errorhandling/pkg/log"
	"github.com/Tobias-Pe/Microservices-Errorhandling/services/api-gateway/internal"
	"github.com/gin-gonic/gin"
	loggrus "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/thinkerou/favicon"
)

type configuration struct {
	port            string
	address         string
	currencyAddress string
	currencyPort    string
	stockAddress    string
	stockPort       string
	cartAddress     string
	cartPort        string
	orderAddress    string
	orderPort       string
	rabbitAddress   string
	rabbitPort      string
}

type service struct {
	currencyClient *internal.CurrencyClient
	stockClient    *internal.StockClient
	cartClient     *internal.CartClient
	orderClient    *internal.OrderClient
}

var logger = loggingUtil.InitLogger()

func main() {
	configuration := readConfig()

	service := &service{
		currencyClient: createCurrencyClient(configuration),
		stockClient:    createStockClient(configuration),
		cartClient:     createCartClient(configuration),
		orderClient:    createOrderClient(configuration),
	}

	createRouter(service, configuration)
}

func readConfig() configuration {
	viper.SetConfigType("env")
	viper.SetConfigName("local")
	viper.AddConfigPath("./config")
	viper.AutomaticEnv()
	err := viper.ReadInConfig()

	if err != nil {
		logger.Info(err)
	}

	serverAddress := ""
	serverPort := viper.GetString("API_GATEWAY_PORT")
	currencyAddress := viper.GetString("CURRENCY_NGINX_ADDRESS")
	currencyPort := viper.GetString("CURRENCY_PORT")
	stockAddress := viper.GetString("STOCK_NGINX_ADDRESS")
	stockPort := viper.GetString("STOCK_PORT")
	cartAddress := viper.GetString("CART_NGINX_ADDRESS")
	cartPort := viper.GetString("CART_PORT")
	orderAddress := viper.GetString("ORDER_NGINX_ADDRESS")
	orderPort := viper.GetString("ORDER_PORT")
	rabbitAddress := viper.GetString("RABBIT_MQ_ADDRESS")
	rabbitPort := viper.GetString("RABBIT_MQ_PORT")

	logger.WithFields(loggrus.Fields{
		"API_GATEWAY_PORT":    serverPort,
		"API_GATEWAY_ADDRESS": serverAddress,
		"CURRENCY_PORT":       currencyPort,
		"CURRENCY_ADDRESS":    currencyAddress,
		"STOCK_ADDRESS":       stockAddress,
		"STOCK_PORT":          stockPort,
		"ORDER_ADDRESS":       orderAddress,
		"ORDER_PORT":          orderPort,
		"RABBIT_MQ_ADDRESS":   rabbitAddress,
		"RABBIT_MQ_PORT":      rabbitPort,
	}).Info("config variables read")

	return configuration{
		address:         serverAddress,
		port:            serverPort,
		currencyAddress: currencyAddress,
		currencyPort:    currencyPort,
		stockAddress:    stockAddress,
		stockPort:       stockPort,
		cartAddress:     cartAddress,
		cartPort:        cartPort,
		orderPort:       orderPort,
		orderAddress:    orderAddress,
		rabbitAddress:   rabbitAddress,
		rabbitPort:      rabbitPort,
	}
}

func createCurrencyClient(configuration configuration) *internal.CurrencyClient {
	currencyClient := internal.NewCurrencyClient(configuration.currencyAddress, configuration.currencyPort)

	return currencyClient
}

func createStockClient(configuration configuration) *internal.StockClient {
	stockClient := internal.NewStockClient(configuration.stockAddress, configuration.stockPort)

	return stockClient
}

func createCartClient(configuration configuration) *internal.CartClient {
	cartClient := internal.NewCartClient(
		configuration.cartAddress,
		configuration.cartPort,
		configuration.rabbitAddress,
		configuration.rabbitPort,
	)

	return cartClient
}

func createOrderClient(configuration configuration) *internal.OrderClient {
	orderClient := internal.NewOrderClient(configuration.orderAddress, configuration.orderPort)

	return orderClient
}

func createRouter(service *service, configuration configuration) {
	gin.SetMode(gin.DebugMode)
	// Creates a gin router with default middleware:
	// logger and recovery (crash-free) middleware
	router := gin.Default()
	router.Use(favicon.New("./assets/favicon.ico")) // set favicon middleware

	router.GET("/exchange/:currency", service.currencyClient.GetExchangeRate())
	router.GET("/articles/*category", service.stockClient.GetArticles())
	router.POST("/cart", service.cartClient.CreateCart())
	router.GET("/cart/:id", service.cartClient.GetCart())
	router.PUT("/cart/:id", service.cartClient.AddToCart())
	router.GET("/order/:id", service.orderClient.GetOrder())
	router.POST("/order", service.orderClient.CreateOrder())

	// By default, it serves on :8080 unless a
	// PORT environment variable was defined.
	err := router.Run(":" + configuration.port)
	if err != nil {
		return
	}

	service.closeConnections()
}

func (service *service) closeConnections() {
	err := service.stockClient.Conn.Close()
	if err != nil {
		logger.WithError(err).Error("Error on closing connection to stock-service")
	}

	err = service.orderClient.Conn.Close()
	if err != nil {
		logger.WithError(err).Error("Error on closing connection to order-service")
	}

	err = service.currencyClient.Conn.Close()
	if err != nil {
		logger.WithError(err).Error("Error on closing connection to currency-service")
	}

	err = service.cartClient.GrpcConn.Close()
	if err != nil {
		logger.WithError(err).Error("Error on closing grpc-connection to stock-service")
	}

	err = service.cartClient.AmqpChannel.Close()
	if err != nil {
		logger.WithError(err).Error("Error on closing amqp-channel to cart-service")
	}

	err = service.cartClient.AmqpConn.Close()
	if err != nil {
		logger.WithError(err).Error("Error on closing amqp-connection to cart-service")
	}
}
