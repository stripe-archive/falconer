package main

import (
	"flag"
	"net"

	"github.com/sirupsen/logrus"
	"github.com/stripe/falconer"

	"google.golang.org/grpc"
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
	grpcServer := grpc.NewServer(opts...)

	falconerServer, err := falconer.NewServer(log, &config)
	if err != nil {
		log.WithError(err).Fatal("Failed to start server")
	}

	falconer.RegisterFalconerServer(grpcServer, falconerServer)
	sinks.RegisterSpanSinkServer(grpcServer, falconerServer)
	log.Debug("Falconer started")
	grpcServer.Serve(lis)
}
