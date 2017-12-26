package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"time"

	"github.com/gphat/jacquard"
	"google.golang.org/grpc"

	"github.com/boltdb/bolt"
	"github.com/stripe/veneur/ssf"
)

type server struct {
	spans []*ssf.SSFSpan
}

func (s *server) BidiSpans(stream jacquard.Jacquard_BidiSpansServer) error {
	for {
		in, err := stream.Recv()
		if err == io.EOF {
			fmt.Println("Sending response")
			return stream.Send(&jacquard.SpanResponse{
				Greeting: "fart",
			})
		}
		if err != nil {
			return err
		}
		fmt.Printf("Got %v\n", in)
	}
}

func main() {
	_, err := bolt.Open("mydb.db", 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		log.Fatal(err)
	}

	lis, err := net.Listen("tcp", fmt.Sprintf("localhost:3000"))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	var opts []grpc.ServerOption
	grpcServer := grpc.NewServer(opts...)
	jacquard.RegisterJacquardServer(grpcServer, &server{})
	grpcServer.Serve(lis)

	foo := ssf.SSFSpan{}
	fmt.Printf("Got %v\n", foo)

	fmt.Println("Hello Jacquard")
}
