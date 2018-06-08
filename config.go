package falconer

type Config struct {
	Debug                         bool   `yaml:"debug"`
	ListenAddress                 string `yaml:"listen_address"`
	SpanExpirationDurationSeconds string `yaml:"span_expiration_duration_seconds"`
	StatsdAddress                 string `yaml:"statsd_address"`
	WorkerCount                   int    `yaml:"worker_count"`
	WorkerSpanDepth               int    `yaml:"worker_span_depth"`
	WorkerWatchDepth              int    `yaml:"worker_watch_depth"`
}
