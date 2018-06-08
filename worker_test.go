package falconer

import (
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/ssf"
)

func TestWorkerGetTrace(t *testing.T) {
	w := NewWorker(logrus.New(), dummyTraceClient(), 10, 10, time.Second+30)

	traceID := int64(1235)

	span1 := &ssf.SSFSpan{
		Id:      1234,
		TraceId: traceID,
		Tags: map[string]string{
			"foo": "bar",
		},
	}
	span2 := &ssf.SSFSpan{
		Id:      1236,
		TraceId: traceID,
		Tags: map[string]string{
			"baz": "gorch",
		},
	}

	w.AddSpan(span1)
	w.AddSpan(span2)

	spans := w.GetTrace(traceID)

	assert.Equal(t, 2, len(spans))
}

func TestWorkerFindSpans(t *testing.T) {
	w := NewWorker(logrus.New(), dummyTraceClient(), 10, 10, time.Second+30)

	traceID := int64(1235)

	span1 := &ssf.SSFSpan{
		Id:      1234,
		TraceId: traceID,
		Tags: map[string]string{
			"foo": "bar",
		},
	}
	span2 := &ssf.SSFSpan{
		Id:      1236,
		TraceId: traceID,
		Tags: map[string]string{
			"baz": "gorch",
		},
	}

	w.AddSpan(span1)
	w.AddSpan(span2)

	resultChan := make(chan []*ssf.SSFSpan, 1)
	w.FindSpans(map[string]string{"foo": "bar"}, resultChan)

	spans := <-resultChan

	assert.Equal(t, 1, len(spans))
}

func TestWorkerSweep(t *testing.T) {
	expirationDuration := time.Second * 30
	w := NewWorker(logrus.New(), dummyTraceClient(), 10, 10, expirationDuration)

	traceID := int64(1235)

	span1 := &ssf.SSFSpan{
		Id:      1234,
		TraceId: traceID,
		Tags: map[string]string{
			"foo": "bar",
		},
	}

	span2 := &ssf.SSFSpan{
		Id:      1234,
		TraceId: traceID,
		Tags: map[string]string{
			"foo": "bar",
		},
	}

	w.AddSpan(span1)
	w.AddSpan(span2)
	// Double the expiration just to be sure
	then := time.Now().Add(expirationDuration + (time.Second * 1))

	w.Sweep(then.Unix())

	var itemCount int

	w.Items.Range(func(_, _ interface{}) bool {
		itemCount++
		return true
	})

	assert.Equal(t, 0, itemCount)
}

func TestWorkerWatches(t *testing.T) {
	w := NewWorker(logrus.New(), dummyTraceClient(), 10, 10, time.Second+30)

	resultChan := make(chan *ssf.SSFSpan, 1)
	w.AddWatch("farts", map[string]string{"foo": "bar"}, resultChan)

	traceID := int64(1235)

	span1 := &ssf.SSFSpan{
		Id:      1234,
		TraceId: traceID,
		Tags: map[string]string{
			"foo": "bar",
		},
	}
	span2 := &ssf.SSFSpan{
		Id:      1236,
		TraceId: traceID,
		Tags: map[string]string{
			"baz": "gorch",
		},
	}
	w.AddSpan(span1)
	w.AddSpan(span2)

	span := <-resultChan

	assert.Equal(t, int64(1234), span.Id)
	select {
	case <-resultChan:
		assert.Fail(t, "There should be no more spans in the channel")
	default:
	}

	w.RemoveWatch("farts")
	assert.Equal(t, 0, len(w.Watches))
}
