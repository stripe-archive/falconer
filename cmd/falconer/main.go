package main

import (
	"flag"
	"net"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stripe/falconer"
	"github.com/stripe/veneur/sinks/grpsink"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
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

	maxage, err := time.ParseDuration(config.SendSpanConnectionMaxAge)
	if err != nil {
		log.WithError(err).Fatal("Malformed configuration value for send_span_connection_max_age; it must be parseable by time.ParseDuration()")
	}

	qlis, err := net.Listen("tcp", config.QueryAddress)
	if err != nil {
		log.WithError(err).Fatal("Failed to listen on query address")
	}
	ilis, err := net.Listen("tcp", config.GRPCIngestAddress)
	if err != nil {
		log.WithError(err).Fatal("Failed to listen on ingest address")
	}

	var igrpcServer *grpc.Server
	if maxage > 0 {
		igrpcServer = grpc.NewServer(grpc.KeepaliveParams(keepalive.ServerParameters{MaxConnectionAge: maxage}))
	} else {
		igrpcServer = grpc.NewServer()
	}
	qgrpcServer := grpc.NewServer()

	falconerServer, err := falconer.NewServer(log, &config)
	if err != nil {
		log.WithError(err).Fatal("Failed to start server")
	}

	falconer.RegisterFalconerServer(qgrpcServer, falconerServer)
	grpsink.RegisterSpanSinkServer(igrpcServer, falconerServer)
	log.Debug("Falconer started")
	go qgrpcServer.Serve(qlis)
	igrpcServer.Serve(ilis)
}
