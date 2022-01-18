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
	"github.com/Tobias-Pe/Microservices-Errorhandling/pkg/circuitbreaker"
	loggingUtil "github.com/Tobias-Pe/Microservices-Errorhandling/pkg/log"
	"github.com/Tobias-Pe/Microservices-Errorhandling/pkg/metrics"
	"github.com/Tobias-Pe/Microservices-Errorhandling/services/api-gateway/internal"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/sony/gobreaker"
	"github.com/spf13/viper"
	ginlogrus "github.com/toorop/gin-logrus"
	ginprometheus "github.com/zsais/go-gin-prometheus"
	"net/http"
	"strings"
	"time"
)

type configuration struct {
	port                        string
	address                     string
	currencyAddress             string
	currencyPort                string
	catalogueAddress            string
	cataloguePort               string
	cartAddress                 string
	cartPort                    string
	orderAddress                string
	orderPort                   string
	rabbitAddress               string
	rabbitPort                  string
	shouldAddCircuitBreaker     bool
	isAdaptiveTimeout           bool
	staticTimeoutDurationMillis int
}

var logger = loggingUtil.InitLogger()

func main() {
	configuration := readConfig()

	service := newService(configuration)

	createRouter(service, configuration)
}

func newService(configuration configuration) *service {
	service := &service{
		config:   configuration,
		cbMetric: metrics.NewCircuitBreakerMetric(),
	}

	service.timeoutMetric = metrics.NewTimeoutMetric()

	if configuration.shouldAddCircuitBreaker {
		// register circuit-breakers
		service.cbCartClient = circuitbreaker.NewCircuitBreaker("CartClient", service.cbMetric)
		service.cbMetric.InitMetric("CartClient")
		service.cbCurrencyClient = circuitbreaker.NewCircuitBreaker("CurrencyClient", service.cbMetric)
		service.cbMetric.InitMetric("CurrencyClient")
		service.cbCatalogueClient = circuitbreaker.NewCircuitBreaker("CatalogueClient", service.cbMetric)
		service.cbMetric.InitMetric("CatalogueClient")
		service.cbOrderClient = circuitbreaker.NewCircuitBreaker("OrderClient", service.cbMetric)
		service.cbMetric.InitMetric("OrderClient")
	}

	// retry connect to all clients repeatedly
	go func() { service.connectCartClient() }()
	go func() { service.connectCurrencyClient() }()
	go func() { service.connectOrderClient() }()
	go func() { service.connectCatalogueClient() }()

	return service
}

// readConfig fetches the needed addresses and ports for connections from the environment variables or the local.env file
func readConfig() configuration {
	viper.SetConfigType("env")
	viper.SetConfigName("local")
	viper.AddConfigPath("./config")
	viper.AutomaticEnv()
	err := viper.ReadInConfig()

	if err != nil {
		logger.WithError(err).Error("could not read in envs")
	}

	serverAddress := ""
	serverPort := viper.GetString("API_GATEWAY_PORT")
	currencyAddress := viper.GetString("CURRENCY_NGINX_ADDRESS")
	currencyPort := viper.GetString("CURRENCY_PORT")
	catalogueAddress := viper.GetString("CATALOGUE_NGINX_ADDRESS")
	cataloguePort := viper.GetString("CATALOGUE_PORT")
	cartAddress := viper.GetString("CART_NGINX_ADDRESS")
	cartPort := viper.GetString("CART_PORT")
	orderAddress := viper.GetString("ORDER_NGINX_ADDRESS")
	orderPort := viper.GetString("ORDER_PORT")
	rabbitAddress := viper.GetString("RABBIT_MQ_ADDRESS")
	rabbitPort := viper.GetString("RABBIT_MQ_PORT")
	shouldAddCircuitBreaker := viper.GetBool("API_GATEWAY_ADD_CIRCUIT_BREAKER")
	isAdaptiveTimeout := viper.GetBool("API_GATEWAY_ADD_ADAPTIVE_TIMEOUT")
	staticTimeoutDurationMillis := viper.GetInt("API_GATEWAY_STATIC_TIMEOUT")

	config := configuration{
		port:                        serverPort,
		address:                     serverAddress,
		currencyAddress:             currencyAddress,
		currencyPort:                currencyPort,
		catalogueAddress:            catalogueAddress,
		cataloguePort:               cataloguePort,
		cartAddress:                 cartAddress,
		cartPort:                    cartPort,
		orderAddress:                orderAddress,
		orderPort:                   orderPort,
		rabbitAddress:               rabbitAddress,
		rabbitPort:                  rabbitPort,
		shouldAddCircuitBreaker:     shouldAddCircuitBreaker,
		isAdaptiveTimeout:           isAdaptiveTimeout,
		staticTimeoutDurationMillis: staticTimeoutDurationMillis,
	}

	logger.WithFields(logrus.Fields{
		"response": config,
	}).Info("config variables read")

	return config
}

func createRouter(service *service, configuration configuration) {
	gin.SetMode(gin.ReleaseMode)
	// Creates a gin router with default middleware:
	// logger and recovery (crash-free) middleware
	router := gin.New()
	router.Use(ginlogrus.Logger(logger), gin.Recovery())

	// prometheus metrics exporter
	promRouter := ginprometheus.NewPrometheus("gin")
	// preserving a low cardinality for the request counter --> https://prometheus.io/docs/practices/naming/#labels
	promRouter.ReqCntURLLabelMappingFn = func(c *gin.Context) string {
		url := c.Request.URL.Path
		for _, p := range c.Params {
			if p.Key == "id" {
				url = strings.Replace(url, p.Value, ":id", 1)
				break
			} else if p.Key == "currency" {
				url = strings.Replace(url, p.Value, ":currency", 1)
				break
			}
		}
		return url
	}
	promRouter.Use(router)

	router.StaticFile("/favicon.ico", "./assets/favicon.ico")
	router.LoadHTMLGlob("./assets/index.html")
	router.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", nil)
	})

	router.GET("/exchange/:currency", service.GetExchangeRateHandler())
	router.GET("/articles/*category", service.GetArticles())
	router.POST("/cart", service.CreateCart())
	router.GET("/cart/:id", service.GetCart())
	router.PUT("/cart/:id", service.AddToCart())
	router.GET("/order/:id", service.GetOrder())
	router.POST("/order", service.CreateOrder())

	err := router.Run(":" + configuration.port)
	if err != nil {
		return
	}

	service.CloseConnections()
}

type service struct {
	currencyClient    *internal.CurrencyClient
	cbCurrencyClient  *gobreaker.CircuitBreaker
	catalogueClient   *internal.CatalogueClient
	cbCatalogueClient *gobreaker.CircuitBreaker
	cartClient        *internal.CartClient
	cbCartClient      *gobreaker.CircuitBreaker
	orderClient       *internal.OrderClient
	cbOrderClient     *gobreaker.CircuitBreaker
	cbMetric          *metrics.CircuitBreakerMetric
	config            configuration
	timeoutMetric     *metrics.TimeoutMetric
}

func (service *service) connectCurrencyClient() {
	timeout := &service.config.staticTimeoutDurationMillis
	if service.config.isAdaptiveTimeout {
		timeout = nil
	}
	var connection *internal.CurrencyClient
	for connection == nil {
		connection = internal.NewCurrencyClient(
			service.config.currencyAddress,
			service.config.currencyPort,
			timeout,
			service.timeoutMetric,
		)
		time.Sleep(time.Millisecond * time.Duration(500))
	}
	service.currencyClient = connection
}

func (service *service) connectCatalogueClient() {
	timeout := &service.config.staticTimeoutDurationMillis
	if service.config.isAdaptiveTimeout {
		timeout = nil
	}
	var connection *internal.CatalogueClient
	for connection == nil {
		connection = internal.NewCatalogueClient(
			service.config.catalogueAddress,
			service.config.cataloguePort,
			timeout,
			service.timeoutMetric,
		)
		time.Sleep(time.Millisecond * time.Duration(500))
	}
	service.catalogueClient = connection
}

func (service *service) connectCartClient() {
	timeout := &service.config.staticTimeoutDurationMillis
	if service.config.isAdaptiveTimeout {
		timeout = nil
	}
	var connection *internal.CartClient
	for connection == nil {
		connection = internal.NewCartClient(
			service.config.cartAddress,
			service.config.cartPort,
			timeout,
			service.timeoutMetric,
		)
		time.Sleep(time.Millisecond * time.Duration(500))
	}
	service.cartClient = connection
}

func (service *service) connectOrderClient() {
	timeout := &service.config.staticTimeoutDurationMillis
	if service.config.isAdaptiveTimeout {
		timeout = nil
	}
	var connection *internal.OrderClient
	for connection == nil {
		connection = internal.NewOrderClient(
			service.config.orderAddress,
			service.config.orderPort,
			timeout,
			service.timeoutMetric,
		)
		time.Sleep(time.Millisecond * time.Duration(500))
	}
	service.orderClient = connection
}

func (service *service) GetExchangeRateHandler() gin.HandlerFunc {
	return func(context *gin.Context) {
		if service.currencyClient != nil {
			service.currencyClient.GetExchangeRate(context, service.cbCurrencyClient)
		} else {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error:": "currency service not available"})
		}
	}
}

func (service *service) GetArticles() gin.HandlerFunc {
	return func(context *gin.Context) {
		if service.catalogueClient != nil {
			service.catalogueClient.GetArticles(context, service.cbCatalogueClient)
		} else {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error:": "catalogue service not available"})
		}
	}
}

func (service *service) CreateCart() gin.HandlerFunc {
	return func(context *gin.Context) {
		if service.cartClient != nil {
			service.cartClient.CreateCart(context, service.cbCartClient)
		} else {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error:": "cart service not available"})
		}
	}
}

func (service *service) GetCart() gin.HandlerFunc {
	return func(context *gin.Context) {
		logger.Infof("%v", service)
		if service.cartClient != nil {
			service.cartClient.GetCart(context, service.cbCartClient)
		} else {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error:": "cart service not available"})
		}
	}
}

func (service *service) AddToCart() gin.HandlerFunc {
	return func(context *gin.Context) {
		if service.cartClient != nil {
			service.cartClient.AddToCart(context, service.cbCartClient)
		} else {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error:": "cart service not available"})
		}
	}
}

func (service *service) GetOrder() gin.HandlerFunc {
	return func(context *gin.Context) {
		if service.orderClient != nil {
			service.orderClient.GetOrder(context, service.cbOrderClient)
		} else {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error:": "order service not available"})
		}
	}
}

func (service *service) CreateOrder() gin.HandlerFunc {
	return func(context *gin.Context) {
		if service.orderClient != nil {
			service.orderClient.CreateOrder(context, service.cbOrderClient)
		} else {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error:": "order service not available"})
		}
	}
}

func (service *service) CloseConnections() {
	err := service.catalogueClient.Conn.Close()
	if err != nil {
		logger.WithError(err).Error("Error on closing connection to catalogue-service")
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
		logger.WithError(err).Error("Error on closing grpc-connection to cart-service")
	}
}
