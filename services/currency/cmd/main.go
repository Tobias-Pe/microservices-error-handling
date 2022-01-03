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
	"github.com/Tobias-Pe/Microservices-Errorhandling/pkg/metrics"
	"github.com/Tobias-Pe/Microservices-Errorhandling/services/currency"
	loggrus "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"log"
	"net"
)

type configuration struct {
	port    string
	address string
}

var logger = loggingUtil.InitLogger()

func main() {
	config := readConfig()
	createGrpcServer(config)
}

func createGrpcServer(config configuration) {
	lis, err := net.Listen("tcp", config.address+":"+config.port)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()

	// create the currency service
	service := currency.Service{
		RequestsMetric: metrics.NewRequestsMetrics(),
	}

	proto.RegisterCurrencyServer(s, &service)

	// start metrics exposer
	httpRouter.NewServer()

	logger.Printf("server listening at %v", lis.Addr())
	// block until error appears
	if err := s.Serve(lis); err != nil {
		logger.Fatalf("failed to serve: %v", err)
	}
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

	serverPort := viper.GetString("CURRENCY_PORT")
	serverAddress := ""

	config := configuration{address: serverAddress, port: serverPort}

	logger.WithFields(loggrus.Fields{"response": config}).Info("config variables read")

	return config
}
