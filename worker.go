package jacquard

import (
	"sync"

	"github.com/stripe/veneur/ssf"
)

type Worker struct {
	SpanChan chan *ssf.SSFSpan
	QuitChan chan struct{}
	Spans    map[int64][]*ssf.SSFSpan
	mutex    sync.Mutex
}

func NewWorker() *Worker {
	return &Worker{
		SpanChan: make(chan *ssf.SSFSpan),
		QuitChan: make(chan struct{}),
		Spans:    make(map[int64][]*ssf.SSFSpan),
		mutex:    sync.Mutex{},
	}
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

	if _, present := w.Spans[span.Id]; !present {
		w.Spans[span.TraceId] = []*ssf.SSFSpan{}
	}

	w.Spans[span.TraceId] = append(w.Spans[span.TraceId], span)
}

func (w *Worker) GetTrace(id int64) []*ssf.SSFSpan {
	return w.Spans[id]
}
