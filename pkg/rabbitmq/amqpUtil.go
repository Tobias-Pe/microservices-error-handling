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

package rabbitmq

import (
	"github.com/streadway/amqp"
)

type AmqpService struct {
	AmqpChannel *amqp.Channel
	AmqpConn    *amqp.Connection
	RabbitURL   string
}

func (service *AmqpService) InitAmqpConnection() error {
	conn, err := amqp.Dial(service.RabbitURL)
	if err != nil {
		return err
	}

	// connection and channel will be closed in main
	service.AmqpConn = conn

	service.AmqpChannel, err = conn.Channel()
	if err != nil {
		return err
	}

	// prefetchCount 1 in QoS will load-balance messages between many instances of this service
	err = service.AmqpChannel.Qos(
		1,    // prefetch count
		0,    // prefetch size
		true, // global
	)
	if err != nil {
		return err
	}

	return nil
}

// CreateListener initialises exchange and queue and binds the queue to a topic and routing key to listen from
func (service *AmqpService) CreateListener(topicName string, queueName string, routingKeys []string) (<-chan amqp.Delivery, error) {
	err := service.AmqpChannel.ExchangeDeclare(
		topicName, // name
		"topic",   // type
		true,      // durable
		false,     // auto-deleted
		false,     // internal
		false,     // no-wait
		nil,       // arguments
	)
	if err != nil {
		return nil, err
	}

	queue, err := service.AmqpChannel.QueueDeclare(
		queueName, // name
		true,      // durable
		false,     // delete when unused
		false,     // exclusive
		false,     // no-wait
		nil,       // arguments
	)
	if err != nil {
		return nil, err
	}

	for _, routingKey := range routingKeys {
		// binds routingKey to the queue
		err = service.AmqpChannel.QueueBind(
			queue.Name, // queue name
			routingKey, // routing key
			topicName,  // exchange
			false,
			nil,
		)
		if err != nil {
			return nil, err
		}
	}

	messages, err := service.AmqpChannel.Consume(
		queue.Name, // queue
		"",         // consumer
		false,      // auto-ack
		false,      // exclusive
		false,      // no-local
		false,      // no-wait
		nil,        // args
	)
	if err != nil {
		return nil, err
	}

	return messages, nil
}
