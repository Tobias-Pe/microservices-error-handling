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
	"github.com/spf13/viper"
	loggingUtil "gitlab.lrz.de/peslalz/errorhandling-microservices-thesis/pkg/log"
	"gitlab.lrz.de/peslalz/errorhandling-microservices-thesis/services/api-gateway"
	"google.golang.org/grpc"
	"strings"
)

type configuration struct {
	port            string
	currencyAddress string
}

var logger = loggingUtil.InitLogger()

func readConfig() configuration {
	viper.SetConfigType("yaml")
	viper.SetConfigName("config")
	viper.AddConfigPath("./services/api-gateway")
	err := viper.ReadInConfig()
	if err != nil {
		logger.Infof("failed to read in config.yml: %s \t if using docker this is not a problem", err)
	}
	replacer := strings.NewReplacer(".", "_")
	viper.SetEnvKeyReplacer(replacer)
	viper.AutomaticEnv()

	return configuration{port: viper.GetString("server.port"), currencyAddress: viper.GetString("currency.address") + ":" + viper.GetString("currency.port")}
}

func main() {
	configuration := readConfig()
	service := api_gateway.NewService(configuration.currencyAddress)
	defer func(Conn *grpc.ClientConn) {
		err := Conn.Close()
		if err != nil {
			logger.WithError(err).Warnf("Could not close grpc connection")
		}
	}(service.Conn)

	gin.SetMode(gin.DebugMode)
	// Creates a gin router with default middleware:
	// logger and recovery (crash-free) middleware
	router := gin.Default()

	router.GET("/exchange/:currency", service.GetExchangeRate())

	// By default it serves on :8080 unless a
	// PORT environment variable was defined.
	err := router.Run(viper.GetString("server.address") + ":" + configuration.port)
	if err != nil {
		return
	}
}
