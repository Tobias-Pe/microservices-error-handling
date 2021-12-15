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
	"github.com/gin-gonic/gin"
	loggrus "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/thinkerou/favicon"
	loggingUtil "gitlab.lrz.de/peslalz/errorhandling-microservices-thesis/pkg/log"
	"gitlab.lrz.de/peslalz/errorhandling-microservices-thesis/services/api-gateway/internal"
	"google.golang.org/grpc"
)

type configuration struct {
	port            string
	address         string
	currencyAddress string
	currencyPort    string
	stockAddress    string
	stockPort       string
}

type service struct {
	currencyClient *internal.CurrencyClient
	stockClient    *internal.StockClient
}

var logger = loggingUtil.InitLogger()

func main() {
	configuration := readConfig()

	service := &service{}

	service.currencyClient = createGrpcCurrencyClient(configuration)
	service.stockClient = createGrpcStockClient(configuration)

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
	serverPort := viper.GetString("API_GATEWAY_PORT")
	serverAddress := ""
	currencyAddress := viper.GetString("CURRENCY_NGINX_ADDRESS")
	currencyPort := viper.GetString("CURRENCY_PORT")
	stockAddress := viper.GetString("STOCK_NGINX_ADDRESS")
	stockPort := viper.GetString("STOCK_PORT")

	logger.WithFields(loggrus.Fields{"API_GATEWAY_PORT": serverPort, "API_GATEWAY_ADDRESS": serverAddress, "CURRENCY_PORT": currencyPort, "CURRENCY_ADDRESS": currencyAddress, "STOCK_ADDRESS": stockAddress, "STOCK_PORT": stockPort}).Info("config variables read")

	return configuration{address: serverAddress, port: serverPort, currencyAddress: currencyAddress, currencyPort: currencyPort, stockAddress: stockAddress, stockPort: stockPort}
}

func createGrpcCurrencyClient(configuration configuration) *internal.CurrencyClient {
	currencyClient := internal.NewCurrencyClient(configuration.currencyAddress, configuration.currencyPort)
	return currencyClient
}

func createGrpcStockClient(configuration configuration) *internal.StockClient {
	stockClient := internal.NewStockClient(configuration.stockAddress, configuration.stockPort)
	return stockClient
}

func createRouter(service *service, configuration configuration) {
	gin.SetMode(gin.DebugMode)
	// Creates a gin router with default middleware:
	// logger and recovery (crash-free) middleware
	router := gin.Default()
	router.Use(favicon.New("./assets/favicon.ico")) // set favicon middleware

	router.GET("/exchange/:currency", service.currencyClient.GetExchangeRate())
	router.GET("/articles/*category", service.stockClient.GetArticles())

	// By default, it serves on :8080 unless a
	// PORT environment variable was defined.
	err := router.Run(":" + configuration.port)
	if err != nil {
		return
	}

	defer func(Conn *grpc.ClientConn) {
		if Conn != nil {
			err := Conn.Close()
			if err != nil {
				logger.WithError(err).Warnf("Could not close grpc connection")
			}
		}
	}(service.currencyClient.Conn)
	defer func(Conn *grpc.ClientConn) {
		if Conn != nil {
			err := Conn.Close()
			if err != nil {
				logger.WithError(err).Warnf("Could not close grpc connection")
			}
		}
	}(service.stockClient.Conn)
}
