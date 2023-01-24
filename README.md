# How to run?

Ensure you have docker & docker compose cli plugin installed. If not, you can use 'docker-compose', if it's available
on your machine

```bash
$ docker compose up -d
```

After this is up and running, test the app by running the following commands from the terminal.
It'd be easier to check if we have multiple terminals open.

On terminal 1:

```bash
$ docker compose exec -ti application bash
# and inside the container
$ go run main.go
```

On terminal 2:

```bash
$ docker compose exec -ti kafka bash
# and inside the container
$ kafka-console-consumer.sh --bootstrap-server=localhost:9092 --topic=my-topic
# can try pushing messages with below command
$ kafka-console-producer.sh --bootstrap-server=localhost:9092 --topic=my-topic
> msg1
> msg2
> msg3
```

On terminal 3:

```bash
$ docker compose exec -ti application bash
# and inside the container
$ go run main.go
```

# What's wrong?

The `kafka-console-consumer` prints messages from Kafka as expected. The Go app's subscription
doesn't seem to receive any message. And if you check the Kafka container logs (docker compose logs -f kafka), the consumer group
registration log for the Go app cannot be found.
