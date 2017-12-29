# Jacquard

Jacquard is a demonstration of a tracing span collector and RPC service. It collects and stores spans for a limited amount of time whilst allowing searches of current spans, streaming of incoming spans and retrieval of entire traces.

It is intended to provide a shared-nothing, horizontally scaleable cluster wherein millions of spans per-second may be written and searched. A currently non-existent aggregator could act as a convenience service for

## Architecture

Jacquard creates a large number of worker goroutines each of which holds a map of spanids to an `Item`. Each `Item` is a span and it's expiration date. Writes are distributed across these workers to improve write throughput.

Searches are performed by each worker naively iterating over all of the spans in it's map and returning matches. Streaming searches are performed by adding a "watch" to each worker which matches against any incoming spans and returning them as a match.

## Performance

Localhost to localhost testing shows on a modern Macbook Pro show:
* Writes of ~1M spans/sec
* Searches over 1M spans in < 100ms

# Compatibility

Jacquard uses [SSF](https://github.com/stripe/veneur/tree/master/ssf) as it's span representation. Clients may write and read from it using [gRPC](https://github.com/gphat/jacquard/blob/master/jacquard.proto).

# Features

* gRPC for performance and streaming
* High performance writes, with local benchmarks (localhost to localhost gRPC) running at ~900K spans/sec using hundreds of goroutines and a span id hashed distribution.
* Arbitrary batch sizes
* Expiration of spans after pre-defined amount of time
* Retrieval of all spans in a given trace
* Searching of spans in current buffer
* Streaming searches that stream matching spans as they are written

# Shortcomings

This is a demonstration at present as has the following limitations:

* Server stops, spans go away as there is no storage of spans
* There is no configuration of anything
* Searches can only be performed on tags and only against exactly matches of values.
  * Searches are only `AND`
* Traces are returned as a stream and not assembled in any meaningful way.
* Watches are ignorant to client connection state and stick around for until a hard timeout of 30s
* The data structures and algorithms are intentionally naive for simplicity and avoiding premature optimization. This likely uses way too much memory and will probably grow over time.
* The server to worker channel is buffered for speed and may drop messages with heavy writes
  * The worker storage is unbounded, except by time. Retention will be governed by the incoming rate of spans and the expiration duration
