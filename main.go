package main

import (
	parser "finops-composition-definition-parser/internal/helpers/configuration"
	kubeHelper "finops-composition-definition-parser/internal/helpers/kube/client"
	"finops-composition-definition-parser/internal/webservice"
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"k8s.io/client-go/rest"
)

func main() {
	configuration, err := parser.ParseConfig()
	if err != nil {
		configuration.Default()
	}

	// Logger configuration
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	zerolog.SetGlobalLevel(configuration.DebugLevel)
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	if err != nil {
		log.Error().Err(err).Msg("configuration missing")
		log.Info().Msg("using default configuration for webservice")
	}

	log.Debug().Msg("List of environment variables:")
	for _, s := range os.Environ() {
		log.Debug().Msg(s)
	}

	kubeHelper.PLURALIZER_URL = configuration.PluralizerUrl

	// Kubernetes configuration
	rcConfig, err := rest.InClusterConfig()
	if err != nil {
		log.Error().Err(err).Msg("resolving kubeconfig for rest client")
		return
	}

	dynClient, err := kubeHelper.NewDynamicClient(rcConfig)
	if err != nil {
		log.Error().Err(err).Msg("obtaining dynamic client for kubernetes")
		return
	}

	// // Start webservice to serve endpoints
	w := webservice.Webservice{
		Config:          rcConfig,
		WebservicePort:  configuration.WebServicePort,
		AnnotationLabel: configuration.AnnotationLabel,
		AnnotationTable: configuration.AnnotationTable,
		DynClient:       dynClient,
		NotebookUrl:     configuration.WebserviceUrl,
		DatabaseConfig:  configuration.DatabaseConfig,
	}
	w.Spinup() // blocks main thread
}
