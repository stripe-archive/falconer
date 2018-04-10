package falconer

import (
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/sinks/grpsink"
	"github.com/stripe/veneur/ssf"
	context "golang.org/x/net/context"
)

type Server struct {
	log         *logrus.Logger
	workerCount int64
	workers     []*Worker
	spanCount   uint64
}

func NewServer(log *logrus.Logger, config *Config) (*Server, error) {
	expiration, err := time.ParseDuration(config.SpanExpirationDurationSeconds)
	if err != nil {
		return &Server{}, err
	}

	workers := make([]*Worker, config.WorkerCount)
	for i := range workers {
		workers[i] = NewWorker(log, config.WorkerSpanDepth, config.WorkerWatchDepth, expiration)
		go workers[i].Work()
	}
	return &Server{
		log:         log,
		workerCount: int64(config.WorkerCount),
		workers:     workers,
	}, nil
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
	s.DispatchSpan(span)
	atomic.AddUint64(&s.spanCount, 1)
	return &grpsink.Empty{}, nil
}

func (s *Server) GetTrace(req *TraceRequest, stream Falconer_GetTraceServer) error {
	s.log.WithField("trace_id", req.GetTraceID()).Debug("Looking for trace")
	worker := s.workers[req.GetTraceID()%s.workerCount]

	start := time.Now()
	spans := worker.GetTrace(req.GetTraceID())

	duration := time.Since(start)
	s.log.WithFields(logrus.Fields{
		"span_count":       len(spans),
		"duration_seconds": duration.Seconds(),
	}).Debug("Completed GetTrace")

	for _, span := range spans {
		stream.Send(span)
	}
	return nil
}

func (s *Server) FindSpans(req *FindSpanRequest, stream Falconer_FindSpansServer) error {
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
	return nil
}

func (s *Server) WatchSpans(req *FindSpanRequest, stream Falconer_WatchSpansServer) error {
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
	return nil
}
