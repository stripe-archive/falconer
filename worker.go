package jacquard

import (
	"sync"
	"time"

	"github.com/stripe/veneur/ssf"
)

type Item struct {
	span       *ssf.SSFSpan
	expiration int64
}

type Worker struct {
	SpanChan chan *ssf.SSFSpan
	QuitChan chan struct{}
	Items    map[int64]*Item
	mutex    sync.Mutex
}

func NewWorker() *Worker {
	w := &Worker{
		SpanChan: make(chan *ssf.SSFSpan),
		QuitChan: make(chan struct{}),
		Items:    make(map[int64]*Item),
		mutex:    sync.Mutex{},
	}
	ticker := time.NewTicker(time.Second * 1)
	go func() {
		for t := range ticker.C {
			w.Sweep(t.Unix())
		}
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

func (w *Worker) AddSpan(span *ssf.SSFSpan) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	w.Items[span.Id] = &Item{
		span:       span,
		expiration: time.Now().Add(time.Minute * 1).Unix(),
	}
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
