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
	"github.com/Tobias-Pe/Microservices-Errorhandling/services/cart/internal"
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
}

var logger = loggingUtil.InitLogger()

func main() {
	config := readConfig()
	createServer(config)
}

// readConfig fetches the needed addresses and ports for connections from the environment variables or the local.env file
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
	serverPort := viper.GetString("CART_PORT")
	cachePort := viper.GetString("CART_REDIS_PORT")
	cacheAddress := viper.GetString("CART_REDIS_ADDRESS")
	rabbitAddress := viper.GetString("RABBIT_MQ_ADDRESS")
	rabbitPort := viper.GetString("RABBIT_MQ_PORT")

	logger.WithFields(loggrus.Fields{
		"CURRENCY_PORT":      serverPort,
		"CURRENCY_ADDRESS":   serverAddress,
		"CART_REDIS_PORT":    cachePort,
		"CART_REDIS_ADDRESS": cacheAddress,
		"RABBIT_MQ_ADDRESS":  rabbitAddress,
		"RABBIT_MQ_PORT":     rabbitPort,
	}).Info("config variables read")

	return configuration{serverAddress: serverAddress,
		serverPort:    serverPort,
		cachePort:     cachePort,
		cacheAddress:  cacheAddress,
		rabbitAddress: rabbitAddress,
		rabbitPort:    rabbitPort,
	}
}

func createServer(config configuration) {
	lis, err := net.Listen("tcp", config.serverAddress+":"+config.serverPort)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	// create service with grpc, amqp and cache connection
	service := internal.NewService(config.cacheAddress, config.cachePort, config.rabbitAddress, config.rabbitPort)
	proto.RegisterCartServer(s, service)

	// start metrics exposer
	httpRouter.NewServer()

	logger.Printf("server listening at %v", lis.Addr())
	// blocking until error appears
	if err = s.Serve(lis); err != nil {
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
