package falconer

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/ssf"
	"github.com/stripe/veneur/trace"
	"github.com/stripe/veneur/trace/metrics"
)

// Item is a span with an expiration time.
type Item struct {
	span       *ssf.SSFSpan
	expiration int64
}

// Watch is a streaming query against incoming spans.
type Watch struct {
	FoundChan chan *ssf.SSFSpan
	Tags      map[string]string
}

// Worker is a repository for spans.
type Worker struct {
	SpanChan  chan *ssf.SSFSpan
	WatchChan chan *ssf.SSFSpan
	QuitChan  chan struct{}
	// Using sync.Map because it matches our workload - write-once, read-many.
	Items   *sync.Map
	Watches map[string]*Watch
	// protects only Watches
	watchMutex         sync.Mutex
	expirationDuration time.Duration
	log                *logrus.Logger
	traceClient        *trace.Client
	// Count of items in the sync.Map. Kept separately because sync.Map doesn't
	// have a method for computing this efficiently.
	itemCount uint64
	// Number of items expired since last flush.
	expiredCount uint64
}

// NewWorker creates a new worker which stores spans, handles queries and expires
// old spans.
func NewWorker(log *logrus.Logger, trace *trace.Client, spanDepth int, watchDepth int, expirationDuration time.Duration) *Worker {
	w := &Worker{
		// We make our incoming span channel buffered so that we can use non-blocking
		// writes. This improves write speed by >= 50% but risks dropping spans if
		// the buffer is full.
		SpanChan:           make(chan *ssf.SSFSpan, spanDepth),
		WatchChan:          make(chan *ssf.SSFSpan, watchDepth),
		QuitChan:           make(chan struct{}),
		Items:              &sync.Map{},
		Watches:            make(map[string]*Watch),
		watchMutex:         sync.Mutex{},
		expirationDuration: expirationDuration,
		log:                log,
		traceClient:        trace,
	}
	ticker := time.NewTicker(expirationDuration)
	// Use a ticker to periodically expire spans.
	go func() {
		for t := range ticker.C {
			w.Sweep(t.Unix())
		}
	}()
	// Monitor the watch channel and catch any spans that might match. We do this
	// separately from the write loop such that watches to not overly impact
	// write throughput.
	go func() {
		w.WatchWork()
	}()
	return w
}

// Work monitors the worker's channel and processes any incoming spans.
func (w *Worker) Work() {
	for {
		select {
		case span := <-w.SpanChan:
			w.AddSpan(span)
		case <-w.QuitChan:
			return
		}
	}
}

// WatchWork iterates over watches channel comparing any incoming spans to this
// workers watches.
func (w *Worker) WatchWork() {
	for {
		select {
		case span := <-w.WatchChan:
			w.watchMutex.Lock()
			for _, watch := range w.Watches {
				for tagKey, tagValue := range watch.Tags {
					if val, ok := span.Tags[tagKey]; ok {
						if val == tagValue {
							watch.FoundChan <- span
							continue // Found a hit, no need to keep looking at this span
						}
					}
				}
			}
			w.watchMutex.Unlock()
		}
	}
}

// AddSpan adds the span to this worker.
func (w *Worker) AddSpan(span *ssf.SSFSpan) {
	_, dup := w.Items.LoadOrStore(span.Id, Item{
		span:       span,
		expiration: time.Now().Add(w.expirationDuration).Unix(),
	})

	atomic.AddUint64(&w.itemCount, 1)

	if dup {
		w.log.WithField("span-id", span.Id).Error("Collision on span, discarding new span")
	}

	if len(w.Watches) > 0 {
		select {
		case w.WatchChan <- span:
		default:
			w.log.Warn("Failed to write watched span")
		}
	}
}

// AddWatch adds a watch with the given name and configures it to return matches
// to the supplied channel.
func (w *Worker) AddWatch(name string, tags map[string]string, foundChan chan *ssf.SSFSpan) {
	w.watchMutex.Lock()
	defer w.watchMutex.Unlock()

	w.Watches[name] = &Watch{
		FoundChan: foundChan,
		Tags:      tags,
	}
}

// RemoveWatch removes the watch with the given name.
func (w *Worker) RemoveWatch(name string) {
	w.watchMutex.Lock()
	defer w.watchMutex.Unlock()

	delete(w.Watches, name)
}

// GetTrace returns all spans with the specified trace id.
func (w *Worker) GetTrace(id int64) []*ssf.SSFSpan {
	var spans []*ssf.SSFSpan

	w.Items.Range(func(_, v interface{}) bool {
		item := v.(Item)
		if item.span.TraceId == id {
			spans = append(spans, item.span)
		}

		return true
	})

	return spans
}

// FindSpans returns all spans matching the specified tags to the channel provided.
func (w *Worker) FindSpans(tags map[string]string, resultChan chan []*ssf.SSFSpan) {
	var foundSpans []*ssf.SSFSpan

	w.Items.Range(func(_, v interface{}) bool {
		span := v.(Item).span
		for fk, fv := range tags {
			if v, ok := span.Tags[fk]; ok {
				if v == fv {
					foundSpans = append(foundSpans, span)
					continue
				}
			}
		}

		return true
	})

	resultChan <- foundSpans
}

// Sweep deletes any expired spans. The provided time is used for comparison to
// facilitate testing.
func (w *Worker) Sweep(expireTime int64) {
	expired := 0
	w.Items.Range(func(k, v interface{}) bool {
		item := v.(Item)

		if item.expiration <= expireTime {
			w.watchMutex.Lock()
			w.Items.Delete(k.(int64))
			w.watchMutex.Unlock()
			expired++
		}

		return true
	})

	atomic.AddUint64(&w.itemCount, ^uint64(expired-1))
	metrics.ReportOne(w.traceClient, ssf.Count("ssfspans.expired", float32(expired), nil))
}
