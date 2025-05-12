package configuration

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	types "finops-composition-definition-parser/apis"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type Configuration struct {
	WebServicePort  int                 `json:"webServicePort" yaml:"webServicePort"`
	AnnotationLabel string              `json:"annotationLabel" yaml:"annotationLabel"`
	AnnotationTable string              `json:"annotationTable" yaml:"annotationTable"`
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

	annotationTable := os.Getenv("ANNOTATION_TABLE")
	if annotationTable == "" {
		annotationTable = "composition_definition_annotations"
		log.Warn().Msgf("annotation table is empty, using default value '%s'", annotationTable)
	}

	annotationLabel := os.Getenv("ANNOTATION_LABEL")
	if annotationLabel == "" {
		annotationLabel = "krateo-finops-focus-resource"
		log.Warn().Msgf("annotation label is empty, using default value '%s'", annotationLabel)
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
		DebugLevel:      debugLevel,
		AnnotationLabel: annotationLabel,
		AnnotationTable: annotationTable,
		WebserviceUrl:   webserviceUrl,
		DatabaseConfig:  types.NamespaceName{Name: databaseConfigName, Namespace: databaseConfigNamespace},
	}, nil
}
