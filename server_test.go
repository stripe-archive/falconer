package falconer

import (
	"context"
	"net"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"github.com/stripe/veneur/sinks/grpsink"
	"github.com/stripe/veneur/ssf"
	"google.golang.org/grpc"
)

func TestSpanIngest(t *testing.T) {
	testaddr := "127.0.0.1:0"
	lis, err := net.Listen("tcp", testaddr)
	if err != nil {
		t.Fatalf("Failed to set up net listener with err %s", err)
	}
	testaddr = lis.Addr().String()

	cfg := Config{
		Debug:                         true,
		ListenAddress:                 testaddr,
		SpanExpirationDurationSeconds: "60s",
		WorkerCount:                   1,
		WorkerSpanDepth:               1,
		WorkerWatchDepth:              1,
	}

	log := logrus.New()
	falconerServer, err := NewServer(log, dummyTraceClient(), &cfg)
	if err != nil {
		log.WithError(err).Fatal("Failed to create server")
	}

	gsrv := grpc.NewServer()
	grpsink.RegisterSpanSinkServer(gsrv, falconerServer)

	go gsrv.Serve(lis)
	runtime.Gosched()

	conn, err := grpc.DialContext(context.Background(), testaddr, grpc.WithInsecure())
	if err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	end := start.Add(2 * time.Second)
	testSpan := &ssf.SSFSpan{
		TraceId:        1,
		ParentId:       1,
		Id:             2,
		StartTimestamp: int64(start.UnixNano()),
		EndTimestamp:   int64(end.UnixNano()),
		Error:          false,
		Service:        "farts-srv",
		Tags: map[string]string{
			"baz": "qux",
		},
		Indicator: false,
		Name:      "farting farty farts",
	}

	client := grpsink.NewSpanSinkClient(conn)
	client.SendSpan(context.Background(), testSpan)

	// Check that our message has arrived on the other side
	require.Equal(t, atomic.LoadUint64(&falconerServer.receivedCount), uint64(1))
}
