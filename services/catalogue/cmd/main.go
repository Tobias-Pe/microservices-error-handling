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
	"github.com/Tobias-Pe/Microservices-Errorhandling/api/proto"
	"github.com/Tobias-Pe/Microservices-Errorhandling/pkg/http-router"
	loggingUtil "github.com/Tobias-Pe/Microservices-Errorhandling/pkg/log"
	"github.com/Tobias-Pe/Microservices-Errorhandling/services/catalogue/internal"
	loggrus "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"log"
	"net"
)

type configuration struct {
	serverPort    string
	serverAddress string
	cachePort     string
	cacheAddress  string
	rabbitAddress string
	rabbitPort    string
	stockPort     string
	stockAddress  string
}

var logger = loggingUtil.InitLogger()

func main() {
	config := readConfig()
	createServer(config)
}

func createServer(configuration configuration) {
	lis, err := net.Listen("tcp", configuration.serverAddress+":"+configuration.serverPort)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	service := internal.NewService(
		configuration.cacheAddress,
		configuration.cachePort,
		configuration.stockAddress,
		configuration.stockPort,
		configuration.rabbitAddress,
		configuration.rabbitPort,
	)

	proto.RegisterCatalogueServer(s, service)

	// start metrics exposer
	httpRouter.NewServer()

	logger.Printf("server listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		logger.Fatalf("failed to serve: %v", err)
	}
	closeConnections(service)
}

func closeConnections(service *internal.Service) {
	err := service.AmqpChannel.Close()
	if err != nil {
		logger.WithError(err).Error("Error on closing amqp-channel")
	}
	err = service.AmqpConn.Close()
	if err != nil {
		logger.WithError(err).Error("Error on closing amqp-connection")
	}
}

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
	serverPort := viper.GetString("CATALOGUE_PORT")
	cachePort := viper.GetString("CATALOGUE_REDIS_PORT")
	cacheAddress := viper.GetString("CATALOGUE_REDIS_ADDRESS")
	stockPort := viper.GetString("STOCK_PORT")
	stockAddress := viper.GetString("STOCK_NGINX_ADDRESS")
	rabbitAddress := viper.GetString("RABBIT_MQ_ADDRESS")
	rabbitPort := viper.GetString("RABBIT_MQ_PORT")

	config := configuration{serverAddress: serverAddress,
		serverPort:    serverPort,
		cachePort:     cachePort,
		cacheAddress:  cacheAddress,
		stockPort:     stockPort,
		stockAddress:  stockAddress,
		rabbitAddress: rabbitAddress,
		rabbitPort:    rabbitPort,
	}

	logger.WithFields(loggrus.Fields{
		"response": config,
	}).Info("config variables read")

	return config
}
