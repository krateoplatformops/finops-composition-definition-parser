package client

import (
	"context"
	"fmt"

	"github.com/krateoplatformops/plumbing/kubeutil/plurals"
	"github.com/rs/zerolog/log"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"

	types "finops-composition-definition-parser/apis"
	secretsHelper "finops-composition-definition-parser/internal/helpers/kube/secrets"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var PLURALIZER_URL = ""

func NewDynamicClient(rc *rest.Config) (*dynamic.DynamicClient, error) {
	config := *rc
	config.APIPath = "/api"
	config.NegotiatedSerializer = serializer.NewCodecFactory(scheme.Scheme)
	config.UserAgent = rest.DefaultKubernetesUserAgent()
	//config.QPS = 1000
	//config.Burst = 3000

	return dynamic.NewForConfig(&config)
}

func GetObj(ctx context.Context, cr *types.Reference, dynClient *dynamic.DynamicClient) (*unstructured.Unstructured, error) {
	gv, err := schema.ParseGroupVersion(cr.ApiVersion)
	if err != nil {
		return nil, fmt.Errorf("unable to parse GroupVersion from composition reference ApiVersion: %w", err)
	}
	gvr := schema.GroupVersionResource{
		Group:    gv.Group,
		Version:  gv.Version,
		Resource: cr.Resource,
	}
	// Get structure to send to webservice
	res, err := dynClient.Resource(gvr).Namespace(cr.Namespace).Get(ctx, cr.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve resource %s with name %s in namespace %s, with apiVersion %s: %w", cr.Resource, cr.Name, cr.Namespace, cr.ApiVersion, err)
	}
	return res, nil
}

func InferGroupResource(a, k string) schema.GroupResource {
	gv, err := schema.ParseGroupVersion(a)
	if err != nil {
		log.Error().Err(err).Msg("could not parse apiVersion for pluralizer")
		return schema.GroupResource{}
	}

	gvk := schema.GroupVersionKind{
		Group:   gv.Group,
		Version: gv.Version,
		Kind:    k,
	}

	tmp, err := plurals.Get(gvk, plurals.GetOptions{
		ResolverFunc: plurals.ResolveAPINames,
	})
	if err != nil {
		log.Error().Err(err).Msgf("could not obtain plural for %s %s %s", gvk.Group, gvk.Kind, gvk.Version)
		return schema.GroupResource{}
	}

	return schema.GroupResource{
		Resource: tmp.Plural,
		Group:    gv.Group,
	}
}

func GetDatabaseUsernamePassword(ctx context.Context, databaseConfigName, databaseConfigNamespace string, dynClient *dynamic.DynamicClient, rc *rest.Config) (string, string, error) {
	// DatabaseConfig to access the database
	databaseConfigReference := &types.Reference{
		ApiVersion: "finops.krateo.io/v1",
		Kind:       "DatabaseConfig",
		Resource:   "databaseconfigs",
		Name:       databaseConfigName,
		Namespace:  databaseConfigNamespace,
	}
	// Get the unstructured object
	databaseConfigUnstructured, err := GetObj(ctx, databaseConfigReference, dynClient)
	if err != nil {
		return "", "", fmt.Errorf("error while retrieving database config: %v", err)
	}

	// Transform the unstructured object into its actual type
	databaseConfig := &types.DatabaseConfig{}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(databaseConfigUnstructured.Object, databaseConfig)
	if err != nil {
		return "", "", fmt.Errorf("unable to convert from unstructured to database config: %v", err)
	}
	databaseConfigSpec := databaseConfig.Spec

	// The password field is a reference to a secret, get the secret
	dbPasswordSecret, err := secretsHelper.Get(ctx, rc, &databaseConfigSpec.PasswordSecretRef)
	if err != nil {
		return "", "", fmt.Errorf("error while retrieving database password secret: %v", err)
	}

	// Use the username and password to call the notebook
	dbPassword := string(dbPasswordSecret.Data[databaseConfigSpec.PasswordSecretRef.Key])
	dbUsername := databaseConfigSpec.Username

	return dbUsername, dbPassword, nil
}

type names struct {
	Plural   string   `json:"plural"`
	Singular string   `json:"singular"`
	Shorts   []string `json:"shorts"`
}
