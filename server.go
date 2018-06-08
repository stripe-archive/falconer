package falconer

import (
	"runtime"
	"sync/atomic"
	"time"

	"github.com/VividCortex/ewma"
	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/sinks/grpsink"
	"github.com/stripe/veneur/ssf"
	"github.com/stripe/veneur/trace"
	"github.com/stripe/veneur/trace/metrics"
	context "golang.org/x/net/context"
)

func dummyTraceClient() *trace.Client {
	ch := make(chan *ssf.SSFSpan)
	cl, err := trace.NewChannelClient(ch)
	if err != nil {
		panic(err)
	}

	go func() {
		for range ch {
		}
	}()

	return cl
}

type Server struct {
	log           *logrus.Logger
	workerCount   int64
	workers       []*Worker
	receivedCount uint64
	traceClient   *trace.Client
	grCounts      goroutineCounts
	grEWMA        goroutineEWMAs
}

type goroutineCounts struct {
	sendSpans, watchSpans, findSpans, getTrace *uint32
}

type goroutineEWMAs struct {
	all, sendSpans, watchSpans, findSpans, getTrace ewma.MovingAverage
}

func NewServer(log *logrus.Logger, config *Config) (*Server, error) {
	expiration, err := time.ParseDuration(config.SpanExpirationDurationSeconds)
	if err != nil {
		return &Server{}, err
	}

	var client *trace.Client
	if config.StatsdAddress == "" {
		client = dummyTraceClient()
	} else {
		client, err = trace.NewClient(config.StatsdAddress)
		if err != nil {
			return &Server{}, err
		}
	}

	workers := make([]*Worker, config.WorkerCount)
	for i := range workers {
		workers[i] = NewWorker(log, client, config.WorkerSpanDepth, config.WorkerWatchDepth, expiration)
		go workers[i].Work()
	}

	// TODO make interval configurable
	flushInterval := 10 * time.Second
	ewmaAge := float64(flushInterval / time.Second)

	s := &Server{
		log:         log,
		workerCount: int64(config.WorkerCount),
		workers:     workers,
		traceClient: client,
		grCounts: goroutineCounts{
			sendSpans:  new(uint32),
			watchSpans: new(uint32),
			findSpans:  new(uint32),
			getTrace:   new(uint32),
		},
		grEWMA: goroutineEWMAs{
			all:        ewma.NewMovingAverage(ewmaAge),
			sendSpans:  ewma.NewMovingAverage(ewmaAge),
			watchSpans: ewma.NewMovingAverage(ewmaAge),
			findSpans:  ewma.NewMovingAverage(ewmaAge),
			getTrace:   ewma.NewMovingAverage(ewmaAge),
		},
	}

	go func() {
		ticker := time.NewTicker(flushInterval)
		for {
			select {
			case <-ticker.C:
				s.Flush()
			}
		}
	}()

	go func() {
		// TODO make interval configurable
		ticker := time.NewTicker(250 * time.Millisecond)
		for {
			select {
			case <-ticker.C:
				s.assessGoroutines()
			}
		}
	}()

	return s, nil
}

func (s *Server) Flush() {
	samples := &ssf.Samples{}
	var totalItems uint64

	for _, worker := range s.workers {
		totalItems += atomic.LoadUint64(&worker.itemCount)
	}

	samples.Add(
		ssf.Count(
			"falconer.ssfspans.received",
			float32(atomic.SwapUint64(&s.receivedCount, 0)),
			nil,
		),
		ssf.Gauge(
			"falconer.ssfspans.current",
			float32(totalItems),
			nil,
		),
		ssf.Gauge(
			"falconer.goroutines.all",
			float32(s.grEWMA.all.Value()),
			nil,
		),
		ssf.Gauge(
			"falconer.goroutines.grpc",
			float32(s.grEWMA.sendSpans.Value()),
			map[string]string{"rpcname": "sendSpans"},
		),
		ssf.Gauge(
			"falconer.goroutines.grpc",
			float32(s.grEWMA.watchSpans.Value()),
			map[string]string{"rpcname": "watchSpans"},
		),
		ssf.Gauge(
			"falconer.goroutines.grpc",
			float32(s.grEWMA.findSpans.Value()),
			map[string]string{"rpcname": "findSpans"},
		),
		ssf.Gauge(
			"falconer.goroutines.grpc",
			float32(s.grEWMA.getTrace.Value()),
			map[string]string{"rpcname": "getTrace"},
		),
	)

	metrics.Report(s.traceClient, samples)
	return
}

func (s *Server) assessGoroutines() {
	s.grEWMA.all.Add(float64(runtime.NumGoroutine()))
	s.grEWMA.sendSpans.Add(float64(atomic.LoadUint32(s.grCounts.sendSpans)))
	s.grEWMA.watchSpans.Add(float64(atomic.LoadUint32(s.grCounts.watchSpans)))
	s.grEWMA.findSpans.Add(float64(atomic.LoadUint32(s.grCounts.findSpans)))
	s.grEWMA.getTrace.Add(float64(atomic.LoadUint32(s.grCounts.getTrace)))
}

func (s *Server) DispatchSpan(span *ssf.SSFSpan) {
	select {
	case s.workers[span.TraceId%s.workerCount].SpanChan <- span:
	default:
		// TODO This means the worker channel buffer is full. Do something about
		// this.
		s.log.Warn("Failed to send message")
	}
}

func (s *Server) SendSpan(ctx context.Context, span *ssf.SSFSpan) (*grpsink.Empty, error) {
	atomic.AddUint32(s.grCounts.sendSpans, 1)
	s.DispatchSpan(span)
	atomic.AddUint64(&s.receivedCount, 1)
	atomic.AddUint32(s.grCounts.sendSpans, ^uint32(0))
	return &grpsink.Empty{}, nil
}

func (s *Server) GetTrace(req *TraceRequest, stream Falconer_GetTraceServer) error {
	atomic.AddUint32(s.grCounts.getTrace, 1)
	s.log.WithField("trace_id", req.GetTraceID()).Debug("Looking for trace")
	worker := s.workers[req.GetTraceID()%s.workerCount]

	start := time.Now()
	spans := worker.GetTrace(req.GetTraceID())

	// TODO convert to histo metrics
	duration := time.Since(start)
	s.log.WithFields(logrus.Fields{
		"span_count":       len(spans),
		"duration_seconds": duration.Seconds(),
	}).Debug("Completed GetTrace")

	for _, span := range spans {
		stream.Send(span)
	}

	atomic.AddUint32(s.grCounts.getTrace, ^uint32(0))
	return nil
}

func (s *Server) FindSpans(req *FindSpanRequest, stream Falconer_FindSpansServer) error {
	atomic.AddUint32(s.grCounts.findSpans, 1)
	s.log.WithField("request", req).Debug("Looking for spans")

	tagsToFind := req.GetTags()

	resultChan := make(chan []*ssf.SSFSpan)

	start := time.Now()

	for _, worker := range s.workers {
		go func(w *Worker, tagsToFind map[string]string, resultChan chan []*ssf.SSFSpan) {
			w.FindSpans(tagsToFind, resultChan)
		}(worker, tagsToFind, resultChan)
	}

	var foundSpans []*ssf.SSFSpan
	for i := int64(0); i < s.workerCount; i++ {
		foundSpans = append(foundSpans, <-resultChan...)
	}

	duration := time.Since(start)
	s.log.WithFields(logrus.Fields{
		"span_count":       len(foundSpans),
		"duration_seconds": duration.Seconds(),
	}).Debug("Completed FindSpans")

	for _, span := range foundSpans {
		stream.Send(span)
	}

	atomic.AddUint32(s.grCounts.findSpans, ^uint32(0))
	return nil
}

func (s *Server) WatchSpans(req *FindSpanRequest, stream Falconer_WatchSpansServer) error {
	atomic.AddUint32(s.grCounts.watchSpans, 1)
	foundChan := make(chan *ssf.SSFSpan)

	s.log.WithField("search", req.GetTags()).Debug("Removing watch")
	for _, worker := range s.workers {
		worker.AddWatch("farts", req.GetTags(), foundChan)
	}
	defer func(s *Server) {
		for _, worker := range s.workers {
			s.log.WithField("search", req.GetTags()).Debug("Removing watch")
			worker.RemoveWatch("farts")
		}
	}(s)

	done := false
	for {
		select {
		case found := <-foundChan:
			stream.Send(found)
		case <-time.After(time.Second * 30):
			// TODO Hacky timer because I'm not sure how to detect a disconnected client
			done = true
		}
		if done {
			break
		}
	}
	// TODO How do we know when this is done?

	atomic.AddUint32(s.grCounts.watchSpans, ^uint32(0))
	return nil
}
