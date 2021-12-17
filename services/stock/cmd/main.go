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
	"context"
	"github.com/Tobias-Pe/Microservices-Errorhandling/api/proto"
	loggingUtil "github.com/Tobias-Pe/Microservices-Errorhandling/pkg/log"
	"github.com/Tobias-Pe/Microservices-Errorhandling/services/stock"
	loggrus "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"go.mongodb.org/mongo-driver/mongo"
	"google.golang.org/grpc"
	"log"
	"net"
	"time"
)

type configuration struct {
	serverPort    string
	serverAddress string
	mongoPort     string
	mongoAddress  string
}

var logger = loggingUtil.InitLogger()

func main() {
	config := readConfig()
	createGrpcServer(config)
}

func createGrpcServer(config configuration) {
	lis, err := net.Listen("tcp", config.serverAddress+":"+config.serverPort)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	service := stock.NewService(config.mongoAddress, config.mongoPort)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	defer func(MongoClient *mongo.Client, ctx context.Context) {
		err := MongoClient.Disconnect(ctx)
		if err != nil {
			logger.WithError(err).Warn("could not disconnect from mongodb")
		}
	}(service.MongoClient, ctx)

	proto.RegisterStockServer(s, service)
	logger.Printf("server listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		logger.Fatalf("failed to serve: %v", err)
	}
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
	serverPort := viper.GetString("STOCK_PORT")
	serverAddress := ""
	mongoAddress := viper.GetString("STOCK_MONGO_ADDRESS")
	mongoPort := viper.GetString("STOCK_MONGO_PORT")

	logger.WithFields(loggrus.Fields{"STOCK_PORT": serverPort, "STOCK_ADDRESS": serverAddress, "STOCK_MONGO_ADDRESS": mongoAddress, "STOCK_MONGO_PORT": mongoPort}).Info("config variables read")

	return configuration{serverAddress: serverAddress, serverPort: serverPort, mongoAddress: mongoAddress, mongoPort: mongoPort}
}
