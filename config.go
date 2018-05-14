package falconer

type Config struct {
	Debug                         bool   `yaml:"debug"`
	GRPCIngestAddress             string `yaml:"grpc_ingest_address"`
	QueryAddress                  string `yaml:"query_address"`
	SendSpanConnectionMaxAge      string `yaml:"send_span_connection_max_age"`
	SpanExpirationDurationSeconds string `yaml:"span_expiration_duration_seconds"`
	WorkerCount                   int    `yaml:"worker_count"`
	WorkerSpanDepth               int    `yaml:"worker_span_depth"`
	WorkerWatchDepth              int    `yaml:"worker_watch_depth"`
}
