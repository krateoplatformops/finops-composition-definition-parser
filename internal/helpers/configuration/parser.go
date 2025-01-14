package configuration

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	types "finops-composition-definition-parser/apis"

	"github.com/rs/zerolog"
)

type Configuration struct {
	WebServicePort  int                 `json:"webServicePort" yaml:"webServicePort"`
	PluralizerUrl   string              `json:"pluralizerUrl" yaml:"pluralizerUrl"`
	AnnotationLabel string              `json:"annotationLabel" yaml:"annotationLabel"`
	DebugLevel      zerolog.Level       `json:"debugLevel" yaml:"debugLevel"`
	WebserviceUrl   string              `json:"webserviceUrl" yaml:"webserviceUrl"`
	DatabaseConfig  types.NamespaceName `json:"databaseConfigName" yaml:"databaseConfigName"`
}

func (c *Configuration) Default() {
	c.WebServicePort = 8085
	c.DebugLevel = zerolog.DebugLevel
}

func ParseConfig() (Configuration, error) {
	port, err := strconv.Atoi(os.Getenv("PORT_FINOPS_COMPOSITION_DEFINITION_PARSER"))
	if err != nil {
		return Configuration{}, err
	}

	pluralizerUrl := os.Getenv("URL_PLURALS")
	if pluralizerUrl == "" {
		return Configuration{}, fmt.Errorf("pluralizer URL cannot be empty")
	}

	webserviceUrl := os.Getenv("URL_DATABASE_HANDLER_PRICING_NOTEBOOK")
	if webserviceUrl == "" {
		return Configuration{}, fmt.Errorf("database handler URL cannot be empty")
	}

	databaseConfigName := os.Getenv("DATABASE_CONFIG_NAME")
	if webserviceUrl == "" {
		return Configuration{}, fmt.Errorf("database config name cannot be empty")
	}

	databaseConfigNamespace := os.Getenv("DATABASE_CONFIG_NAMESPACE")
	if webserviceUrl == "" {
		return Configuration{}, fmt.Errorf("database config namespace cannot be empty")
	}

	annotationLabel := os.Getenv("ANNOTATION_LABEL")
	if annotationLabel == "" {
		return Configuration{}, fmt.Errorf("annotation label cannot be empty")
	}

	debugLevel := zerolog.InfoLevel
	switch strings.ToLower(os.Getenv("DEBUG_LEVEL")) {
	case "debug":
		debugLevel = zerolog.DebugLevel
	case "info":
		debugLevel = zerolog.InfoLevel
	case "error":
		debugLevel = zerolog.ErrorLevel
	}
	return Configuration{
		WebServicePort:  port,
		PluralizerUrl:   pluralizerUrl,
		DebugLevel:      debugLevel,
		AnnotationLabel: annotationLabel,
		WebserviceUrl:   webserviceUrl,
		DatabaseConfig:  types.NamespaceName{Name: databaseConfigName, Namespace: databaseConfigNamespace},
	}, nil
}
