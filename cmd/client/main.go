package main

import (
	"context"
	"log"
	"math/rand"
	"time"

	"github.com/gphat/jacquard"
	"github.com/stripe/veneur/ssf"
	"google.golang.org/grpc"
)

func main() {
	words := []string{
		"crasp", "decisive", "three", "right", "mood", "see", "real", "walk", "shake",
	}

	var opts []grpc.DialOption
	opts = append(opts, grpc.WithInsecure())
	conn, err := grpc.Dial("127.0.0.1:3000", opts...)
	if err != nil {
		log.Fatalf("fail to dial: %v", err)
	}
	defer conn.Close()

	rand.Seed(time.Now().Unix())

	client := jacquard.NewJacquardClient(conn)

	stream, err := client.SendSpans(context.Background())
	if err != nil {
		log.Fatalf("sending span %v", err)
	}

	batchCount := 100
	count := 10000
	for i := 0; i < count; i++ {
		spans := make([]*ssf.SSFSpan, batchCount)
		var tags map[string]string
		if rand.Float32() < .25 {
			tags = map[string]string{
				"foo": words[rand.Int31n(int32(len(words)))],
			}
		}
		for j := 0; j < batchCount; j++ {
			span := ssf.SSFSpan{
				Id:      rand.Int63(),
				TraceId: rand.Int63(),
				Name:    "fart",
				Tags:    tags,
			}
			spans[j] = &span
		}
		err2 := stream.SendMsg(&jacquard.SpanBatch{Spans: spans})
		if err2 != nil {
			log.Fatalf("Err sending: %v", err2)
		}
	}
	reply, err := stream.CloseAndRecv()
	if err != nil {
		log.Fatalf("Err closeandrecv: %v", err)
	}
	log.Printf("Got %v", reply)
}
