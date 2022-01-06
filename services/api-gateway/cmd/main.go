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
	ginprometheus "github.com/zsais/go-gin-prometheus"
	"net/http"
	"strings"
)

type configuration struct {
	port             string
	address          string
	currencyAddress  string
	currencyPort     string
	catalogueAddress string
	cataloguePort    string
	cartAddress      string
	cartPort         string
	orderAddress     string
	orderPort        string
	rabbitAddress    string
	rabbitPort       string
}

var logger = loggingUtil.InitLogger()

func main() {
	configuration := readConfig()

	service := newService(configuration)

	createRouter(service, configuration)
}

func newService(configuration configuration) *service {
	service := &service{
		currencyClient:  createCurrencyClient(configuration),
		cartClient:      createCartClient(configuration),
		orderClient:     createOrderClient(configuration),
		catalogueClient: createCatalogueClient(configuration),
		config:          configuration,
	}
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

	config := configuration{
		address:          serverAddress,
		port:             serverPort,
		currencyAddress:  currencyAddress,
		currencyPort:     currencyPort,
		catalogueAddress: catalogueAddress,
		cataloguePort:    cataloguePort,
		cartAddress:      cartAddress,
		cartPort:         cartPort,
		orderPort:        orderPort,
		orderAddress:     orderAddress,
		rabbitAddress:    rabbitAddress,
		rabbitPort:       rabbitPort,
	}

	logger.WithFields(loggrus.Fields{
		"response": config,
	}).Info("config variables read")

	return config
}

func createCurrencyClient(configuration configuration) *internal.CurrencyClient {
	currencyClient := internal.NewCurrencyClient(configuration.currencyAddress, configuration.currencyPort)

	return currencyClient
}

func createCatalogueClient(configuration configuration) *internal.CatalogueClient {
	catalogueClient := internal.NewCatalogueClient(configuration.catalogueAddress, configuration.cataloguePort)

	return catalogueClient
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
	gin.SetMode(gin.ReleaseMode)
	// Creates a gin router with default middleware:
	// logger and recovery (crash-free) middleware
	router := gin.Default()

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
	currencyClient  *internal.CurrencyClient
	catalogueClient *internal.CatalogueClient
	cartClient      *internal.CartClient
	orderClient     *internal.OrderClient
	config          configuration
}

func (service *service) GetExchangeRateHandler() gin.HandlerFunc {
	if service.currencyClient != nil {
		return service.currencyClient.GetExchangeRate()
	} else {
		return func(c *gin.Context) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error:": "currency service not available"})
		}
	}
}

func (service *service) GetArticles() gin.HandlerFunc {
	if service.catalogueClient != nil {
		return service.catalogueClient.GetArticles()
	} else {
		return func(c *gin.Context) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error:": "catalogue service not available"})
		}
	}
}

func (service *service) CreateCart() gin.HandlerFunc {
	if service.cartClient != nil {
		return service.cartClient.CreateCart()
	} else {
		return func(c *gin.Context) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error:": "cart service not available"})
		}
	}
}

func (service *service) GetCart() gin.HandlerFunc {
	if service.cartClient != nil {
		return service.cartClient.GetCart()
	} else {
		return func(c *gin.Context) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error:": "cart service not available"})
		}
	}
}

func (service *service) AddToCart() gin.HandlerFunc {
	if service.cartClient != nil {
		return service.cartClient.AddToCart()
	} else {
		return func(c *gin.Context) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error:": "cart service not available"})
		}
	}
}

func (service *service) GetOrder() gin.HandlerFunc {
	if service.orderClient != nil {
		return service.orderClient.GetOrder()
	} else {
		return func(c *gin.Context) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error:": "order service not available"})
		}
	}
}

func (service *service) CreateOrder() gin.HandlerFunc {
	if service.orderClient != nil {
		return service.orderClient.CreateOrder()
	} else {
		return func(c *gin.Context) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error:": "order service not available"})
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
