package falconer

type Config struct {
	Debug                         bool   `yaml:"debug"`
	QueryAddress                  string `yaml:"query_address"`
	GRPCIngestAddress             string `yaml:"grpc_ingest_address"`
	SpanExpirationDurationSeconds string `yaml:"span_expiration_duration_seconds"`
	SendSpanConnectionMaxAge      string `yaml:"send_span_connection_max_age"`
	WorkerCount                   int    `yaml:"worker_count"`
	WorkerSpanDepth               int    `yaml:"worker_span_depth"`
	WorkerWatchDepth              int    `yaml:"worker_watch_depth"`
}
