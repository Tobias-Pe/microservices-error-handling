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
	loggingUtil "gitlab.lrz.de/peslalz/errorhandling-microservices-thesis/pkg/log"
	"gitlab.lrz.de/peslalz/errorhandling-microservices-thesis/services/api-gateway"
	"google.golang.org/grpc"
)

type configuration struct {
	port            string
	address         string
	currencyAddress string
}

var logger = loggingUtil.InitLogger()

func main() {
	configuration := readConfig()

	service := createGrpcClient(configuration)

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
	serverAddress := viper.GetString("API_GATEWAY_ADDRESS")
	currencyAddress := viper.GetString("CURRENCY_ADDRESS")
	currencyPort := viper.GetString("CURRENCY_PORT")

	logger.WithFields(loggrus.Fields{"API_GATEWAY_PORT": serverPort, "API_GATEWAY_ADDRESS": serverAddress, "CURRENCY_PORT": currencyPort, "CURRENCY_ADDRESS": currencyAddress}).Info("config variables read")

	return configuration{address: serverAddress, port: serverPort, currencyAddress: currencyAddress + ":" + currencyPort}
}

func createGrpcClient(configuration configuration) *api_gateway.Service {
	service := api_gateway.NewService(configuration.currencyAddress)
	defer func(Conn *grpc.ClientConn) {
		err := Conn.Close()
		if err != nil {
			logger.WithError(err).Warnf("Could not close grpc connection")
		}
	}(service.Conn)

	logger.Infoln("Connection to currency service successfully!")
	return service
}

func createRouter(service *api_gateway.Service, configuration configuration) {
	gin.SetMode(gin.DebugMode)
	// Creates a gin router with default middleware:
	// logger and recovery (crash-free) middleware
	router := gin.Default()

	router.GET("/exchange/:currency", service.GetExchangeRate())

	// By default, it serves on :8080 unless a
	// PORT environment variable was defined.
	err := router.Run(configuration.address + ":" + configuration.port)
	if err != nil {
		return
	}
}
