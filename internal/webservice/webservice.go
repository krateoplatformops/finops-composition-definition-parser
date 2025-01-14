package webservice

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"

	corev1 "k8s.io/api/core/v1"

	coreprovider "github.com/krateoplatformops/core-provider/apis/compositiondefinitions/v1alpha1"

	types "finops-composition-definition-parser/apis"
	chartHelper "finops-composition-definition-parser/internal/helpers/chart"
	kubeHelper "finops-composition-definition-parser/internal/helpers/kube/client"
	secretsHelper "finops-composition-definition-parser/internal/helpers/kube/secrets"
	notebookHelper "finops-composition-definition-parser/internal/helpers/notebook"
)

const (
	homeEndpoint      = "/"
	allEventsEndpoint = "/handle"
)

type Webservice struct {
	WebservicePort  int
	NotebookUrl     string
	AnnotationLabel string
	Config          *rest.Config
	DynClient       *dynamic.DynamicClient
	DatabaseConfig  types.NamespaceName
}

func (r *Webservice) handleHome(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (r *Webservice) handleAllEvents(c *gin.Context) {
	log.Debug().Msg("received event on /handle")
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		log.Error().Err(err).Msg("error reading request body")
		return
	}
	defer c.Request.Body.Close()

	var event corev1.Event
	err = json.Unmarshal(body, &event)
	if err != nil {
		log.Error().Err(err).Msg("error parsing JSON")
		return
	}

	gv, err := schema.ParseGroupVersion(event.InvolvedObject.APIVersion)
	if err != nil {
		log.Error().Err(err).Msg("could not parse Group Version from ApiVersion")
		return
	}

	if gv.Group != "core.krateo.io" && event.InvolvedObject.Kind != "CompositionDefinition" {
		return
	}

	log.Info().Msgf("Event %s received for composition definition %s", event.Reason, string(event.InvolvedObject.UID))

	// Composition GVK
	gr := kubeHelper.InferGroupResource(event.InvolvedObject.APIVersion, event.InvolvedObject.Kind)
	composition := &types.Reference{
		ApiVersion: event.InvolvedObject.APIVersion,
		Kind:       event.InvolvedObject.Kind,
		Resource:   gr.Resource,
		Name:       event.InvolvedObject.Name,
		Namespace:  event.InvolvedObject.Namespace,
	}

	if event.Reason == "DeletedExternalResource" {
		log.Info().Msgf("'%s' event for composition definition %s %s %s %s", event.Reason, composition.ApiVersion, composition.Resource, composition.Name, composition.Namespace)
		return
	}

	compositionObjectUnstructured, err := kubeHelper.GetObj(c.Request.Context(), composition, r.DynClient)
	if err != nil {
		log.Error().Err(err).Msg("retrieving object")
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("error while retrieving object: %s", err)})
		return
	}

	if event.Reason == "CreatedExternalResource" {
		log.Info().Msgf("'%s' event for composition definition %s %s %s %s", event.Reason, composition.ApiVersion, composition.Resource, composition.Name, composition.Namespace)
		// Transform the unstructured object into a CompositionDefinition
		compositionObject := &coreprovider.CompositionDefinition{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(compositionObjectUnstructured.Object, compositionObject); err != nil {
			log.Error().Err(err).Msg("error while converting from unstructured to composition definition")
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Error while handling %s event: %s", event.Reason, err)})
			return
		}

		// Download, extract and then cleanup the download
		defer chartHelper.CleanupDirectory(compositionObject.Spec.Chart.Repo)
		if err := chartHelper.DownloadChart(compositionObject.Spec.Chart.Url, compositionObject.Spec.Chart.Repo, compositionObject.Spec.Chart.Version, "./"); err != nil {
			log.Error().Err(err).Msg("error while downloading and extracting chart")
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Error while handling %s event: %s", event.Reason, err)})
			return
		}

		// Get the list of all annotations with the given key
		resourceMap, err := chartHelper.ProcessHelmTemplates(compositionObject.Spec.Chart.Repo, r.AnnotationLabel)
		if err != nil {
			log.Error().Err(err).Msg("error while processing chart")
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Error while handling %s event: %s", event.Reason, err)})
			return
		}

		// Transform the annotations into a JSON object to send to the finops-database-handler notebook
		jsonObject, err := json.Marshal(resourceMap)
		if err != nil {
			log.Error().Err(err).Msg("error while converting resources to json")
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Error while handling %s event: %s", event.Reason, err)})
			return
		}
		// Get the composition definition unique id, used as primary key in the database
		comppositionId := string(event.InvolvedObject.UID)

		// DatabaseConfig to access the database
		databaseConfigReference := &types.Reference{
			ApiVersion: "finops.krateo.io/v1",
			Kind:       "DatabaseConfig",
			Resource:   "databaseconfigs",
			Name:       r.DatabaseConfig.Name,
			Namespace:  r.DatabaseConfig.Namespace,
		}
		// Get the unstructured object
		databaseConfigUnstructured, err := kubeHelper.GetObj(c.Request.Context(), databaseConfigReference, r.DynClient)
		if err != nil {
			log.Error().Err(err).Msg("error while retrieving database config")
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Error while handling %s event: %s", event.Reason, err)})
			return
		}

		// Transform the unstructured object into its actual type
		databaseConfig := &types.DatabaseConfig{}
		err = runtime.DefaultUnstructuredConverter.FromUnstructured(databaseConfigUnstructured.Object, databaseConfig)
		if err != nil {
			log.Error().Err(err).Msg("unable to convert from unstructured to database config")
		}
		databaseConfigSpec := databaseConfig.Spec

		// The password field is a reference to a secret, get the secret
		dbPasswordSecret, err := secretsHelper.Get(c.Request.Context(), r.Config, &databaseConfigSpec.PasswordSecretRef)
		if err != nil {
			log.Error().Err(err).Msg("error while retrieving database password secret")
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Error while handling %s event: %s", event.Reason, err)})
			return
		}

		// Use the username and password to call the notebook
		dbPassword := string(dbPasswordSecret.Data[databaseConfigSpec.PasswordSecretRef.Key])
		dbUsername := databaseConfigSpec.Username
		if err := notebookHelper.CallNotebook(r.NotebookUrl, comppositionId, jsonObject, dbUsername, dbPassword); err != nil {
			log.Error().Err(err).Msg("error while calling notebook")
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Error while handling %s event: %s", event.Reason, err)})
			return
		}

	}
}

func (r *Webservice) Spinup() {
	var c *gin.Engine
	// gin.New() instead of gin.Default() to avoid default logging
	if zerolog.GlobalLevel() == zerolog.DebugLevel {
		c = gin.New()
		c.Use(gin.Recovery())
		c.Use(debugLoggerMiddleware())
	} else {
		gin.SetMode(gin.ReleaseMode)
		c = gin.Default()
	}

	c.GET(homeEndpoint, r.handleHome)
	c.POST(allEventsEndpoint, r.handleAllEvents)

	c.Run(fmt.Sprintf(":%d", r.WebservicePort))
}
