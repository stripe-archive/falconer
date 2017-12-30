# Falconer

Falconer is a tracing span collector, buffer and RPC service. It collects and stores spans for a limited amount of time whilst allowing searches of current spans, streaming search of incoming spans and retrieval of entire traces. It would be best paired with [Veneur](https://github.com/stripe/veneur).

![Diagram](https://raw.githubusercontent.com/gphat/falconer/master/diagram.png)

Falconer provides an unsampled, shared-nothing, horizontally scalable cluster wherein millions of spans per second may be written and searched. In other words, throw millions of spans at it and let it keep them *all* around for a configurable amount of time. Run more boxes to keep more spans.

For example if you emit 100K spans/sec averaging 2K per span, run a dozen or so instances of Falconer, each keeping a dozen or so GB of spans per instance[0]. Now you can recall any given span for the last 15M. You pick the amount you want based in # of spans `a`, size of spans `b` and retention period `c`: `a * b * c / number of instances = per instance memory required`.

0 - Note that there's likely a lot of wasted memory overhead here and it will likely OOM or grow ridiculously until it is optimized with both improved data structures and better tuning knobs. See [Shortcomings](https://github.com/gphat/falconer#shortcomings) below!

# Features

* gRPC for performance and streaming
* High performance writes, with local benchmarks (localhost to localhost gRPC) running at ~900K spans/sec using hundreds of goroutines and a span id hashed distribution.
* Arbitrary batch sizes
* Expiration of spans after pre-defined amount of time
* Retrieval of all spans in a given trace
* Searching of spans in current buffer
* Streaming searches that stream matching spans as they are written

## Architecture And Operation

Falconer creates a large number of worker goroutines each of which holds a map of spanids to an `Item`. Each `Item` is a span and it's expiration time. Writes are distributed across these workers to improve write throughput.

* **Writes** are striped across goroutines using modulus and the span's ID. Each worker adds the incoming spans to it's store. Very little happens in this write path so as to keep speed.
* **Searches** are performed by each worker naively iterating over all of the spans in it's map and returning matches.
* **Streaming searches** are performed outside from the write path by adding a "watch" to each worker. Incoming spans are submitted to a watch channel which matches against any incoming spans and returns them as a match.
* **Expiration** is handled periodically by the workers, deleting any expired spans.

## Performance

Localhost to localhost testing shows on a modern Macbook Pro shows:
* Writes of ~1M spans/sec
* Searches over 1M spans in < 100ms

# API and Usage

Falconer uses [SSF](https://github.com/stripe/veneur/tree/master/ssf) as it's span representation. Clients may write and read from it using [gRPC](https://github.com/gphat/falconer/blob/master/falconer.proto).

# Future Direction

* Improved data structures and algorithms for more efficient operation
* Programmatic sampling for writing to storage
* Per-service, hot-configurable expirations
* Separate service for querying/aggregating spans
* Output to long term storage (Zipkin, Jaeger, etc?)

## Shortcomings

This is a demonstration at present and has the following limitations:

* The data structures and algorithms are intentionally naive for simplicity and to avoid premature optimization. This likely uses way too much memory and will probably grow over time.
  * The use of a `map` is largely for convenience, as it does not require managing or compacting.
* Little attention is currently paid to managing goroutine life cycles (watches, etc)
  * Watches are ignorant to client connection state and stick around for until a hard timeout of 30s
* Server stops, spans go away as there is no storage of spans
  * The worker storage is unbounded, except by time. Retention will be governed by the incoming rate of spans and the expiration duration
* Searches can only be performed on tags and only against exact matches of values. (Although this is trivial to change)
  * Searches are only `AND`
* The server to worker channel is buffered for speed and may drop messages with heavy writes
  * Same for the watch channel
