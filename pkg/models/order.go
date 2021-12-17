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

import "go.mongodb.org/mongo-driver/bson/primitive"

type Status struct {
	Name    string
	Message string
}

func StatusFetching() Status {
	return Status{
		Name:    "FETCHING",
		Message: "The articles are being fetched from the cart and the price will be calculated. Please stand by...",
	}
}

func StatusReserving() Status {
	return Status{
		Name:    "RESERVING",
		Message: "We are reserving the articles for you. Please stand by...",
	}
}

func StatusPaying() Status {
	return Status{
		Name:    "PAYING",
		Message: "We are checking your payment data and will debit your credit card. Please stand by...",
	}
}

func StatusShipping() Status {
	return Status{
		Name:    "SHIPPING",
		Message: "The order is complete and is on it's way to you. Thank you for your purchase!",
	}
}

func StatusComplete() Status {
	return Status{
		Name:    "COMPLETE",
		Message: "The order is complete and is on it's way to you. Thank you for your purchase!",
	}
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
}
