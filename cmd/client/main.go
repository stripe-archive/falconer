package main

import (
	"context"
	"log"

	"github.com/gphat/jacquard"
	"github.com/stripe/veneur/ssf"
	"google.golang.org/grpc"
)

func main() {
	var opts []grpc.DialOption
	opts = append(opts, grpc.WithInsecure())
	conn, err := grpc.Dial("127.0.0.1:3000", opts...)
	if err != nil {
		log.Fatalf("fail to dial: %v", err)
	}
	defer conn.Close()

	client := jacquard.NewJacquardClient(conn)

	span := ssf.SSFSpan{
		Name: "fart",
	}
	log.Println("Sending spans")
	stream, err := client.BidiSpans(context.Background())
	if err != nil {
		log.Fatalf("sending span %v", err)
	}
	err = stream.Send(&span)
	if err != nil {
		log.Fatalf("Err sending: %v", err)
	}
	reply, err := stream.CloseAndRecv()
	if err != nil {
		log.Fatalf("Err closeandrecv: %v", err)
	}
	log.Printf("Got %v", reply)
}
