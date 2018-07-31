package main

import (
	"crypto/tls"
	"crypto/x509"
	"flag"
	"io/ioutil"
	"net"

	"github.com/sirupsen/logrus"
	"github.com/stripe/falconer"
	"github.com/stripe/veneur/sinks/grpsink"
	"github.com/stripe/veneur/ssf"
	"github.com/stripe/veneur/trace"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

var (
	configFile = flag.String("f", "", "The config file to read for settings.")
	log        = logrus.New()
)

func main() {
	flag.Parse()

	if configFile == nil || *configFile == "" {
		log.Fatal("You must supply a config file argument")
	}

	config, err := falconer.ReadConfig(*configFile)
	if err != nil {
		log.WithError(err).Fatal("Unable to load config file")
	}

	if config.Debug {
		log.SetLevel(logrus.DebugLevel)
	}

	lis, err := net.Listen("tcp", config.ListenAddress)
	if err != nil {
		log.WithError(err).Fatal("Failed to listen")
	}
	var opts []grpc.ServerOption
	if len(config.TLS.Certificates) > 0 {
		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{},
			ClientCAs:    x509.NewCertPool(),
		}
		for _, certConfig := range config.TLS.Certificates {
			cert, err := tls.LoadX509KeyPair(certConfig.Cert, certConfig.Key)
			if err != nil {
				log.WithError(err).Fatal("Failed to load TLS certificate")
			}
			tlsConfig.Certificates = append(tlsConfig.Certificates, cert)
		}

		for _, caConfig := range config.TLS.ClientCas {
			caData, err := ioutil.ReadFile(caConfig)
			if err != nil {
				log.WithError(err).Fatal("Failed to read client CA")
			}
			if !tlsConfig.ClientCAs.AppendCertsFromPEM(caData) {
				log.Fatal("Failed to load client CA")
			}
			tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		}
		opts = append(opts, grpc.Creds(credentials.NewTLS(tlsConfig)))
	}
	grpcServer := grpc.NewServer(opts...)

	var client *trace.Client
	if config.StatsdAddress != "" {
		client, err = trace.NewClient(config.StatsdAddress)
		if err != nil {
			log.WithError(err).WithField("addr", config.StatsdAddress).Fatal("Failed to set up trace client for address")
		}

		trace.DefaultClient = client
		trace.Service = "falconer"
		ssf.NamePrefix = "falconer."
	}

	grpclog.SetLoggerV2(logrusWrapper{log})

	falconerServer, err := falconer.NewServer(log, client, &config)
	if err != nil {
		log.WithError(err).Fatal("Failed to start server")
	}

	falconer.RegisterFalconerServer(grpcServer, falconerServer)
	grpsink.RegisterSpanSinkServer(grpcServer, falconerServer)

	// Set up a healthcheck server.
	healthsrv := health.NewServer()
	for name, _ := range grpcServer.GetServiceInfo() {
		healthsrv.SetServingStatus(name, grpc_health_v1.HealthCheckResponse_SERVING)
	}
	grpc_health_v1.RegisterHealthServer(grpcServer, healthsrv)

	log.Debug("Falconer started")
	grpcServer.Serve(lis)
}

// Wrapper type to allow us to emit grpc logs through logrus.
type logrusWrapper struct {
	*logrus.Logger
}

// V indicates whether a particular log level is at least l. It's required to
// meet the LoggerV2 interface, but logrus handles such leveling internally, so
// it's not exposed in this way.  GRPC's logging levels are:
//
//   0=info, 1=warning, 2=error, 3=fatal
//
// logrus's are:
//
//   0=panic, 1=fatal, 2=error, 3=warn, 4=info, 5=debug
func (lw logrusWrapper) V(l int) bool {
	logrusLevel := 4 - l
	return int(lw.Level) <= logrusLevel
}
