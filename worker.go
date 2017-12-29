package falconer

import (
	"log"
	"sync"
	"time"

	"github.com/stripe/veneur/ssf"
)

type Item struct {
	span       *ssf.SSFSpan
	expiration int64
}

type Watch struct {
	FoundChan chan *ssf.SSFSpan
	Tags      map[string]string
}

type Worker struct {
	SpanChan  chan *ssf.SSFSpan
	WatchChan chan *ssf.SSFSpan
	QuitChan  chan struct{}
	// Intentionally not using pointers because of https://github.com/golang/go/issues/9477
	// which implies maps without pointers are less GC-fussy since we don't have
	// traverse the pointer
	Items      map[int64]Item
	Watches    map[string]*Watch
	mutex      sync.Mutex
	watchMutex sync.Mutex
}

func NewWorker() *Worker {
	w := &Worker{
		// We make our incoming span channel buffered so that we can use non-blocking
		// writes. This improves write speed by >= 50% but risks dropping spans if
		// the buffer is full.
		SpanChan:  make(chan *ssf.SSFSpan, 1000),
		WatchChan: make(chan *ssf.SSFSpan, 1000),
		QuitChan:  make(chan struct{}),
		Items:     make(map[int64]Item),
		Watches:   make(map[string]*Watch),
		mutex:     sync.Mutex{},
	}
	ticker := time.NewTicker(time.Second * 1)
	go func() {
		for t := range ticker.C {
			w.Sweep(t.Unix())
		}
	}()
	go func() {
		w.WatchWork()
	}()
	return w
}

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

func (w *Worker) AddSpan(span *ssf.SSFSpan) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	w.Items[span.Id] = Item{
		span:       span,
		expiration: time.Now().Add(time.Minute * 1).Unix(),
	}

	if len(w.Watches) > 0 {
		select {
		case w.WatchChan <- span:
		default:
			log.Println("Failed to write watched span")
		}
	}
}

func (w *Worker) AddWatch(name string, tags map[string]string, foundChan chan *ssf.SSFSpan) {
	w.watchMutex.Lock()
	defer w.watchMutex.Unlock()

	w.Watches[name] = &Watch{
		FoundChan: foundChan,
		Tags:      tags,
	}
}

func (w *Worker) RemoveWatch(name string) {
	w.watchMutex.Lock()
	defer w.watchMutex.Unlock()

	delete(w.Watches, name)
}

func (w *Worker) GetTrace(id int64) []*ssf.SSFSpan {
	var spans []*ssf.SSFSpan
	for _, item := range w.Items {
		if item.span.TraceId == id {
			spans = append(spans, item.span)
		}
	}
	return spans
}

func (w *Worker) FindSpans(tags map[string]string, resultChan chan []*ssf.SSFSpan) {
	var foundSpans []*ssf.SSFSpan
	for _, item := range w.Items {
		span := item.span
		// scanned++
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
	// fmt.Printf("Expired %d\n", expired)
}
