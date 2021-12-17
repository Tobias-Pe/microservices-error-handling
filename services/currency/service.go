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

package currency

import (
	"context"
	"fmt"
	"github.com/Tobias-Pe/Microservices-Errorhandling/api/proto"
	loggingUtil "github.com/Tobias-Pe/Microservices-Errorhandling/pkg/log"
	loggrus "github.com/sirupsen/logrus"
	"strings"
)

var logger = loggingUtil.InitLogger()

type Service struct {
	proto.UnimplementedCurrencyServer
}

func getExchangeRate(targetCurrency string) (float32, error) {
	targetCurrency = strings.ToUpper(strings.TrimSpace(targetCurrency))
	switch targetCurrency {
	case "USD": //USA
		return 1.1317644, nil
	case "GBP": //GB
		return 0.85286078, nil
	case "INR": //INDIA
		return 85.754929, nil
	case "CAS": //CANADA
		return 1.4400515, nil
	case "JPY": //JAPAN
		return 128.30817, nil
	case "SEK": //SWEDEN
		return 10.244315, nil
	case "PLN": // POLAND
		return 4.6181471, nil
	default:
		return -1, fmt.Errorf("target currency not supported: %s", targetCurrency)

	}
}

func (s *Service) GetExchangeRate(_ context.Context, req *proto.RequestExchangeRate) (*proto.ReplyExchangeRate, error) {
	exchangeRate, err := getExchangeRate(req.CustomerCurrency)
	if err != nil {
		logger.WithFields(loggrus.Fields{"Request:": req.CustomerCurrency}).WithError(err).Warn("Responding with an error in GetExchangeRate")
	} else {
		logger.WithFields(loggrus.Fields{"Request:": req.CustomerCurrency, "Response": exchangeRate}).Info("Request handled")
	}
	return &proto.ReplyExchangeRate{ExchangeRate: exchangeRate}, err
}
