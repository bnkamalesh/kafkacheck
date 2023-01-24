package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/bnkamalesh/errors"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
)

type Config struct {
	Seeds         []string
	Topics        []string
	ConsumerGroup string

	IdleTimeout            time.Duration
	RequestTimeoutOverhead time.Duration
	RetryTimeout           time.Duration
	TxnTimeout             time.Duration
	RecordTimeout          time.Duration
	SessionTimeout         time.Duration
}

func New(ctx context.Context, cfg *Config) (*kgo.Client, error) {
	cli, err := kgo.NewClient(
		kgo.SeedBrokers(cfg.Seeds...),
		kgo.ConsumeTopics(cfg.Topics...),
		kgo.ConnIdleTimeout(cfg.IdleTimeout),
		kgo.RetryTimeout(cfg.RetryTimeout),
		kgo.RequestTimeoutOverhead(cfg.RequestTimeoutOverhead),
		kgo.TransactionTimeout(cfg.TxnTimeout),
		kgo.RecordDeliveryTimeout(cfg.RecordTimeout),
		kgo.SessionTimeout(cfg.SessionTimeout),
		// DisableAutoCommit is required to handle usecases where we have to NACK a message
		// if the processing fails and the message has to be retried
		kgo.DisableAutoCommit(),
		kgo.ConsumerGroup(cfg.ConsumerGroup),
	)
	if err != nil {
		return nil, errors.Wrapf(err, "[%s] kafka client initialization failed", cfg.ConsumerGroup)
	}

	ctx, cancel := context.WithTimeout(ctx, time.Second*3)
	defer cancel()

	err = cli.Ping(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "kafka ping failed")
	}

	err = cli.Flush(ctx)
	if err != nil {
		return nil, errors.Wrap(err)
	}

	return cli, nil
}

func doRandomChecks(ctx context.Context, kcli *kgo.Client, kcfg Config) error {
	debugOut := bytes.NewBuffer(make([]byte, 0, 256))
	defer func() {
		debuginfo := debugOut.String()
		// uncomment below line to see all debug info
		debuginfo = ""
		fmt.Println(debuginfo)
	}()

	gmeta, memberID := kcli.GroupMetadata()
	debugOut.Write([]byte(fmt.Sprintf("gmeta: %s, memberID: %d\n", gmeta, memberID)))
	debugOut.Write([]byte(fmt.Sprintf("re-adding topics: %v\n", kcfg.Topics)))
	kcli.AddConsumeTopics(kcfg.Topics...)
	kcli.ForceMetadataRefresh()

	admCli := kadm.NewClient(kcli)
	bm, err := admCli.BrokerMetadata(ctx)
	if err != nil {
		return errors.Wrap(err)
	}
	jbytes, _ := json.MarshalIndent(bm, "", " ")
	debugOut.Write(jbytes)
	debugOut.Write([]byte("\n"))

	out, err := admCli.DescribeTopicConfigs(ctx, kcfg.Topics...)
	if err != nil {
		return errors.Wrap(err)
	}
	jbytes, _ = json.MarshalIndent(out, "", " ")
	debugOut.Write(jbytes)
	debugOut.Write([]byte("\n"))

	tdets, err := admCli.ListTopicsWithInternal(ctx, kcfg.Topics...)
	if err != nil {
		return errors.Wrap(err)
	}
	jbytes, _ = json.MarshalIndent(tdets, "", " ")
	debugOut.Write(jbytes)
	debugOut.Write([]byte("\n"))

	_, err = admCli.CreateTopics(ctx, 1, 1, nil, kcfg.Topics...)
	if err != nil {
		return errors.Wrap(err)
	}

	err = kcli.ProduceSync(ctx, &kgo.Record{Topic: kcfg.Topics[0], Value: []byte(`hello world: ` + time.Now().Format(time.RFC3339))}).FirstErr()
	if err != nil {
		return errors.Wrap(err)
	}
	return nil
}

func subWithPollRecords(ctx context.Context, kcli *kgo.Client, kcfg Config) error {
	for {
		fmt.Println("PollRecords, topics:", kcfg.Topics)
		recs := kcli.PollRecords(ctx, 3)
		errs := recs.Errors()
		if len(errs) > 0 {
			for _, err := range errs {
				fmt.Println("pollrec.err:", err)
			}
			return errors.New("poll record stopped")
		}
		recs.EachRecord(func(r *kgo.Record) {
			fmt.Println("pollrec:", string(r.Value))
		})
	}
}

func subWithPollFetches(ctx context.Context, kcli *kgo.Client, kcfg Config) error {
	for {
		ctx := context.Background()
		fmt.Println("PollFetches, topics:", kcfg.Topics)
		fetches := kcli.PollFetches(ctx)
		errs := fetches.Errors()
		if len(errs) > 0 {
			for _, err := range errs {
				fmt.Println("pollfetch: rec err:", err)
			}
			return errors.New("poll fetch stopped")
		}

		iter := fetches.RecordIter()
		recordCommits := make([]*kgo.Record, 0, fetches.NumRecords())
		for !iter.Done() {
			record := iter.Next()
			fmt.Println("pollfetch:", string(record.Value))
			recordCommits = append(recordCommits, record)
		}

		err := kcli.CommitRecords(ctx, recordCommits...)
		if err != nil {
			// the subscriber should not exit if there's a commit error. It should just log
			// and continue listening
			fmt.Printf("pollfetch, commit err: %+v\n", err)
		}
	}
}

func main() {
	defer log.Println("exited")

	ctx := context.Background()
	kcfg := Config{
		Seeds:                  []string{os.Getenv("KAFKA_SEEDS")},
		Topics:                 []string{os.Getenv("KAFKA_CONSUMER_TOPIC")},
		ConsumerGroup:          "kafkacheck",
		IdleTimeout:            time.Second * 3,
		RequestTimeoutOverhead: time.Second * 3,
		RetryTimeout:           time.Second * 3,
		TxnTimeout:             time.Second * 3,
		RecordTimeout:          time.Second * 3,
		SessionTimeout:         time.Second * 3,
	}
	kcli, err := New(ctx, &kcfg)
	if err != nil {
		fmt.Printf("New: %+v\n", err)
		return
	}

	err = doRandomChecks(ctx, kcli, kcfg)
	if err != nil {
		fmt.Printf("doRandomChecks:\n%+v\n", err)
		return
	}

	// go func() {
	// 	err := subWithPollRecords(ctx, kcli, kcfg)
	// 	if err != nil {
	// 		fmt.Printf("subWithPollRecords:\n%+v\n", err)
	// 	}
	// }()

	err = subWithPollFetches(ctx, kcli, kcfg)
	if err != nil {
		fmt.Printf("subWithPollFetches:\n%+v\n", err)
	}

}
