package main

import (
	"context"
	"flag"
	"io"
	"log"
	"math/rand"
	"time"

	"github.com/gphat/jacquard"
	"google.golang.org/grpc"
)

func main() {
	var id = flag.Int("traceid", 0, "Trace ID")
	log.Println(id)

	flag.Parse()

	var opts []grpc.DialOption
	opts = append(opts, grpc.WithInsecure())
	conn, err := grpc.Dial("127.0.0.1:3000", opts...)
	if err != nil {
		log.Fatalf("fail to dial: %v", err)
	}
	defer conn.Close()

	rand.Seed(time.Now().Unix())

	client := jacquard.NewJacquardClient(conn)

	resp, err := client.FindSpans(context.Background(), &jacquard.FindSpanRequest{
		Tags: map[string]string{"foo": "three"},
	})

	// resp, err := client.GetTrace(context.Background(), &jacquard.TraceRequest{
	// 	TraceID: int64(*id),
	// })
	if err != nil {
		log.Fatalf("Failed to get trace: %v", err)
	}

	for {
		span, err := resp.Recv()
		if err == io.EOF {
			log.Println("Done")
			return
		}
		log.Printf("Span: %v\n", span)
	}
}
