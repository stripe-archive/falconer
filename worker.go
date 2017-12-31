package falconer

import (
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/ssf"
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
	// Intentionally not using pointers because of https://github.com/golang/go/issues/9477
	// which implies maps without pointers are less GC-fussy since we don't have
	// traverse the pointer
	Items              map[int64]Item
	Watches            map[string]*Watch
	mutex              sync.Mutex
	watchMutex         sync.Mutex
	expirationDuration time.Duration
	log                *logrus.Logger
}

// NewWorker creates a new worker which stores spans, handles queries and expires
// old spans.
func NewWorker(log *logrus.Logger, spanDepth int, watchDepth int, expirationDuration time.Duration) *Worker {
	w := &Worker{
		// We make our incoming span channel buffered so that we can use non-blocking
		// writes. This improves write speed by >= 50% but risks dropping spans if
		// the buffer is full.
		SpanChan:           make(chan *ssf.SSFSpan, spanDepth),
		WatchChan:          make(chan *ssf.SSFSpan, watchDepth),
		QuitChan:           make(chan struct{}),
		Items:              make(map[int64]Item),
		Watches:            make(map[string]*Watch),
		mutex:              sync.Mutex{},
		watchMutex:         sync.Mutex{},
		expirationDuration: expirationDuration,
		log:                log,
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
		}
	}
}

// AddSpan adds the span to this worker.
func (w *Worker) AddSpan(span *ssf.SSFSpan) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	w.Items[span.Id] = Item{
		span:       span,
		expiration: time.Now().Add(w.expirationDuration).Unix(),
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
	for _, item := range w.Items {
		if item.span.TraceId == id {
			spans = append(spans, item.span)
		}
	}
	return spans
}

// FindSpans returns all spans matching the specified tags to the channel provided.
func (w *Worker) FindSpans(tags map[string]string, resultChan chan []*ssf.SSFSpan) {
	var foundSpans []*ssf.SSFSpan
	for _, item := range w.Items {
		span := item.span
		for fk, fv := range tags {
			if v, ok := span.Tags[fk]; ok {
				if v == fv {
					foundSpans = append(foundSpans, span)
					continue
				}
			}
		}
	}

	resultChan <- foundSpans
}

// Sweep deletes any expired spans. The provided time is used for comparisong to
// facilitate testing.
func (w *Worker) Sweep(expireTime int64) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	expired := 0
	for key, item := range w.Items {
		if item.expiration <= expireTime {
			delete(w.Items, key)
			expired++
		}
	}
	w.log.WithField("expired_count", expired).Debug("Expired spans")
}
