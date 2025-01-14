package compositions

import (
	"context"
	types "finops-composition-definition-parser/apis"
	"fmt"

	"github.com/rs/zerolog/log"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"

	kubeHelper "finops-composition-definition-parser/internal/helpers/kube/client"
)

func GetCompositionById(compositionId string, dynClient *dynamic.DynamicClient, config *rest.Config) (*unstructured.Unstructured, *types.Reference, error) {
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create discovery client: %v", err)
	}

	// Get list of preferred versions for the group
	groups, err := discoveryClient.ServerGroups()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get server groups: %v", err)
	}

	// Find all versions for our group
	var versions []string
	for _, group := range groups.Groups {
		if group.Name == "composition.krateo.io" {
			for _, version := range group.Versions {
				versions = append(versions, version.Version)
			}
		}
	}

	if len(versions) == 0 {
		return nil, nil, fmt.Errorf("no versions found for group composition.krateo.io")
	}

	// Try each version
	for _, version := range versions {
		resources, err := discoveryClient.ServerResourcesForGroupVersion(fmt.Sprintf("composition.krateo.io/%s", version))
		if err != nil {
			log.Warn().Err(err).Msgf("error getting resources for version %s", version)
			continue
		}

		// Search through each resource type in the group
		for _, r := range resources.APIResources {
			// Skip resources that can't be listed
			if !containsString(r.Verbs, "list") {
				continue
			}

			gvr := schema.GroupVersionResource{
				Group:    "composition.krateo.io",
				Version:  version,
				Resource: r.Name,
			}

			// List objects of this resource type
			list, err := dynClient.Resource(gvr).List(context.TODO(), v1.ListOptions{})
			if err != nil {
				log.Warn().Err(err).Msgf("error listing resources of type %s", r.Name)
				continue
			}

			// Search for the object with matching UID
			for _, item := range list.Items {
				if string(item.GetUID()) == compositionId {
					conditions, ok, err := unstructured.NestedSlice(item.Object, "status", "conditions")
					if !ok {
						return nil, nil, fmt.Errorf("could not get status.Reason of composition %s: %v", compositionId, err)
					}
					if conditions[0].(map[string]interface{})["reason"].(string) == "Creating" {
						return nil, nil, fmt.Errorf("composition is creating")
					}
					ref := &types.Reference{
						ApiVersion: item.GetAPIVersion(),
						Kind:       item.GetKind(),
						Resource:   kubeHelper.InferGroupResource(item.GetAPIVersion(), item.GetKind()).Resource,
						Name:       item.GetName(),
						Namespace:  item.GetNamespace(),
					}

					return &item, ref, nil
				}
			}
		}
	}

	return nil, nil, fmt.Errorf("did not find composition with id %s in any version or resource type", compositionId)
}

// Helper function to check if a string slice contains a specific string
func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}
