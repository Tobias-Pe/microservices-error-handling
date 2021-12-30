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
		1,     // prefetch count
		0,     // prefetch size
		false, // global
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
