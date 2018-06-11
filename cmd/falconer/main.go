package main

import (
	"flag"
	"net"

	"github.com/sirupsen/logrus"
	"github.com/stripe/falconer"
	"github.com/stripe/veneur/sinks/grpsink"
	"github.com/stripe/veneur/trace"

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

	var client *trace.Client
	if config.StatsdAddress != "" {
		client, err = trace.NewClient(config.StatsdAddress)
		if err != nil {
			log.WithError(err).WithField("addr", config.StatsdAddress).Fatal("Failed to set up trace client for address")
		}

		trace.DefaultClient = client
		trace.Service = "falconer"
	}

	falconerServer, err := falconer.NewServer(log, client, &config)
	if err != nil {
		log.WithError(err).Fatal("Failed to start server")
	}

	falconer.RegisterFalconerServer(grpcServer, falconerServer)
	grpsink.RegisterSpanSinkServer(grpcServer, falconerServer)
	log.Debug("Falconer started")
	grpcServer.Serve(lis)
}
