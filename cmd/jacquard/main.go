package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"time"

	"github.com/gphat/falconer"
	"google.golang.org/grpc"

	"github.com/stripe/veneur/ssf"
)

type FalconerServer struct {
	workerCount int64
	workers     []*falconer.Worker
}

func NewServer(workerCount int64) *FalconerServer {
	workers := make([]*falconer.Worker, workerCount)
	for i := range workers {
		workers[i] = falconer.NewWorker()
		go workers[i].Work()
	}
	return &FalconerServer{
		workerCount: workerCount,
		workers:     workers,
	}
}

func (s *FalconerServer) DispatchSpan(span *ssf.SSFSpan) {
	spanChan := s.workers[span.TraceId%s.workerCount].SpanChan
	select {
	case spanChan <- span:
	default:
		log.Println("Failed to send message")
	}
}

func (s *FalconerServer) SendSpans(stream falconer.Falconer_SendSpansServer) error {
	count := 0
	start := time.Now()
	for {
		batch, err := stream.Recv()
		if err == io.EOF {
			d := time.Since(start)
			fmt.Printf("Sending response: %d in %f @ %f/sec\n", count, d.Seconds(), float64(count)/d.Seconds())
			return stream.SendMsg(&falconer.SpanResponse{
				Greeting: "fart",
			})
		}
		if err != nil {
			return err
		}
		for _, span := range batch.Spans {
			s.DispatchSpan(span)
		}
		count = count + len(batch.Spans)
	}
}

func (s *FalconerServer) GetTrace(req *falconer.TraceRequest, stream falconer.Falconer_GetTraceServer) error {
	log.Printf("Looking for %v\n", req.GetTraceID())
	worker := s.workers[req.GetTraceID()%s.workerCount]
	spans := worker.GetTrace(req.GetTraceID())
	log.Printf("Found %v", spans)
	for _, span := range spans {
		stream.Send(span)
	}
	fmt.Println("Done with GetTrace")
	return nil
}

func (s *FalconerServer) FindSpans(req *falconer.FindSpanRequest, stream falconer.Falconer_FindSpansServer) error {
	log.Printf("Looking for %v", req)

	tagsToFind := req.GetTags()

	resultChan := make(chan []*ssf.SSFSpan)

	start := time.Now()

	for _, worker := range s.workers {
		go func(w *falconer.Worker, tagsToFind map[string]string, resultChan chan []*ssf.SSFSpan) {
			w.FindSpans(tagsToFind, resultChan)
		}(worker, tagsToFind, resultChan)
	}

	var foundSpans []*ssf.SSFSpan
	for i := int64(0); i < s.workerCount; i++ {
		foundSpans = append(foundSpans, <-resultChan...)
	}

	duration := time.Since(start)
	log.Printf("Scanned for %f seconds", duration.Seconds())

	for _, span := range foundSpans {
		stream.Send(span)
	}

	return nil
}

func (s *FalconerServer) WatchSpans(req *falconer.FindSpanRequest, stream falconer.Falconer_WatchSpansServer) error {
	foundChan := make(chan *ssf.SSFSpan)

	log.Printf("Adding watch for %v\n", req.GetTags())
	for _, worker := range s.workers {
		worker.AddWatch("farts", req.GetTags(), foundChan)
	}
	defer func(s *FalconerServer) {
		for _, worker := range s.workers {
			fmt.Printf("Removing watch for %v\n", req.GetTags())
			worker.RemoveWatch("farts")
		}
	}(s)

	done := false
	for {
		select {
		case found := <-foundChan:
			stream.Send(found)
		case <-time.After(time.Second * 30):
			// Hacky timer because I'm not sure how to detect a disconnected client
			done = true
		}
		if done {
			break
		}
	}
	// How do we know when this is done?
	return nil
}

func main() {
	lis, err := net.Listen("tcp", fmt.Sprintf("localhost:3000"))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	var opts []grpc.ServerOption
	grpcServer := grpc.NewServer(opts...)
	falconer.RegisterFalconerServer(grpcServer, NewServer(256))
	grpcServer.Serve(lis)
}
