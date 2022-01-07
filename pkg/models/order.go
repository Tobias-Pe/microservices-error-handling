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

package models

import (
	"encoding/json"
	"fmt"
	"github.com/Tobias-Pe/Microservices-Errorhandling/api/requests"
	loggingUtils "github.com/Tobias-Pe/Microservices-Errorhandling/pkg/log"
	"github.com/sirupsen/logrus"
	"github.com/streadway/amqp"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	// CartAbortMessage is needed by stock service --> there will be no reservation to abort
	CartAbortMessage = "We could not fetch your cart. Check your cart's ID again."

	statusNameReserving = "RESERVING"
	statusNameFetching  = "FETCHING"
	statusNamePaying    = "PAYING"
	statusNameShipping  = "SHIPPING"
	statusNameCompleted = "COMPLETE"
	statusNameAborted   = "ABORTED"
)

var logger = loggingUtils.InitLogger()

type Status struct {
	Name    string
	Message string
}

func StatusFetching() Status {
	return Status{
		Name:    statusNameFetching,
		Message: "The articles are being fetched from the cart. Please stand by...",
	}
}

func StatusReserving() Status {
	return Status{
		Name:    statusNameReserving,
		Message: "We are reserving the articles for you. Please stand by...",
	}
}

func StatusPaying() Status {
	return Status{
		Name:    statusNamePaying,
		Message: "We are checking your payment data and will debit your credit card. Please stand by...",
	}
}

func StatusShipping() Status {
	return Status{
		Name:    statusNameShipping,
		Message: "The order is payed and will be prepared for shipment. Please stand by...",
	}
}

func StatusComplete() Status {
	return Status{
		Name:    statusNameCompleted,
		Message: "The order is complete and is on it's way to you. Thank you for your purchase!",
	}
}

func StatusAborted(message string) Status {
	return Status{
		Name:    statusNameAborted,
		Message: message,
	}
}

// IsProgressive will compare two status.
// returns if the current status is a new progressive status after old one.
func IsProgressive(newStatus string, oldStatus string) bool {
	switch oldStatus {
	case statusNameCompleted:
		return false
	case statusNameAborted:
		return false
	case statusNameFetching:
		return newStatus == statusNameReserving || newStatus == statusNamePaying || newStatus == statusNameShipping || newStatus == statusNameAborted || newStatus == statusNameCompleted
	case statusNameReserving:
		return newStatus == statusNamePaying || newStatus == statusNameShipping || newStatus == statusNameAborted || newStatus == statusNameCompleted
	case statusNamePaying:
		return newStatus == statusNameShipping || newStatus == statusNameAborted || newStatus == statusNameCompleted
	case statusNameShipping:
		return newStatus == statusNameCompleted || newStatus == statusNameAborted
	}
	return false
}

type Order struct {
	ID                 primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	Status             string             `json:"status" bson:"status"`
	Message            string             `json:"message" bson:"message"`
	Articles           []string           `json:"articleIDs" bson:"articleIDs"`
	CartID             string             `json:"cartId" bson:"cartId"`
	Price              float64            `json:"priceEuro" bson:"priceEuro"`
	CustomerAddress    string             `json:"address" bson:"address"`
	CustomerName       string             `json:"name" bson:"name"`
	CustomerCreditCard string             `json:"creditCard" bson:"creditCard"`
	CustomerEmail      string             `json:"email" bson:"email"`
}

// PublishOrderStatusUpdate will broadcast the current order via amqp channel with the order.status as routing key.
func (order Order) PublishOrderStatusUpdate(channel *amqp.Channel) error {
	routingKey, err := order.getRoutingKey()
	if err != nil {
		return err
	}
	bytes, err := json.Marshal(order)
	if err != nil {
		return err
	}
	err = channel.Publish(
		requests.OrderTopic, // exchange
		routingKey,          // routing key
		true,                // mandatory
		false,               // immediate
		amqp.Publishing{
			DeliveryMode: amqp.Persistent,
			ContentType:  "application/json",
			Body:         bytes,
		})
	if err != nil {
		return err
	}
	logger.WithFields(logrus.Fields{"request": order}).Infof("Published Order update")
	return nil
}

// getRoutingKey will return the orders routing key dependent on the current order status
func (order Order) getRoutingKey() (string, error) {
	switch order.Status {
	case statusNameCompleted:
		return requests.OrderStatusComplete, nil
	case statusNameAborted:
		return requests.OrderStatusAbort, nil
	case statusNameFetching:
		return requests.OrderStatusFetch, nil
	case statusNamePaying:
		return requests.OrderStatusPay, nil
	case statusNameShipping:
		return requests.OrderStatusShip, nil
	case statusNameReserving:
		return requests.OrderStatusReserve, nil
	default:
		return "", fmt.Errorf("unknown status: %s ", order.Status)
	}
}
