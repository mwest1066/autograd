package consumer

import (
	"fmt"
	"log"

	"github.com/streadway/amqp"

	"github.com/PrairieLearn/autograd/grader"
)

const (
	consumerTag = "autograd-consumer"
)

type Consumer struct {
	conn    *amqp.Connection
	channel *amqp.Channel
	grader  *grader.Grader
	done    chan error
}

func NewConsumer(amqpURI, queueName string, grader *grader.Grader) (*Consumer, error) {
	c := &Consumer{
		conn:    nil,
		channel: nil,
		grader:  grader,
		done:    make(chan error),
	}

	var err error

	log.Printf("dialing %q", amqpURI)
	c.conn, err = amqp.Dial(amqpURI)
	if err != nil {
		return nil, fmt.Errorf("Dial: %s", err)
	}

	go func() {
		log.Printf("closing: %s", <-c.conn.NotifyClose(make(chan *amqp.Error)))
	}()

	log.Printf("got Connection, getting Channel")
	c.channel, err = c.conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("Channel: %s", err)
	}
	if err := c.channel.Qos(1, 0, false); err != nil {
		return nil, fmt.Errorf("Channel Qos: %s", err)
	}

	log.Printf("got Channel, declaring Queue %q", queueName)
	queue, err := c.channel.QueueDeclare(queueName, true, false, false, false, nil)
	if err != nil {
		return nil, fmt.Errorf("Queue Declare: %s", err)
	}

	log.Printf("declared Queue (%q %d messages, %d consumers), starting Consume (consumer tag %q)",
		queue.Name, queue.Messages, queue.Consumers, consumerTag)
	deliveries, err := c.channel.Consume(queue.Name, consumerTag, false, false, false, false, nil)
	if err != nil {
		return nil, fmt.Errorf("Queue Consume: %s", err)
	}

	go c.handle(deliveries, c.done)

	return c, nil
}

func (c *Consumer) Shutdown() error {
	// will close() the deliveries channel
	if err := c.channel.Cancel(consumerTag, true); err != nil {
		return fmt.Errorf("Consumer cancel failed: %s", err)
	}

	if err := c.conn.Close(); err != nil {
		return fmt.Errorf("AMQP connection close error: %s", err)
	}

	defer log.Printf("AMQP shutdown OK")

	// wait for handle() to exit
	return <-c.done
}

func (c *Consumer) NotifyClose() chan *amqp.Error {
	return c.conn.NotifyClose(make(chan *amqp.Error))
}

func (c *Consumer) handle(deliveries <-chan amqp.Delivery, done chan error) {
	for d := range deliveries {
		log.Printf(
			"got %dB delivery: [%v] %q",
			len(d.Body),
			d.DeliveryTag,
			d.Body,
		)

		if err := c.grader.Grade(d.Body); err != nil {
			log.Printf("Error initializing grader: %v", err)
		}

		d.Ack(false)
	}
	log.Printf("handle: deliveries channel closed")
	done <- nil
}