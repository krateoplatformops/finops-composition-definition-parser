package chart

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
	"k8s.io/client-go/rest"

	getter "finops-composition-definition-parser/internal/helpers/chart/getter"
	secretsHelper "finops-composition-definition-parser/internal/helpers/kube/secrets"

	coreprovider "github.com/krateoplatformops/core-provider/apis/compositiondefinitions/v1alpha1"
)

// ChartIndex represents the structure of a Helm chart index.yaml file
type ChartIndex struct {
	Entries map[string][]ChartEntry `yaml:"entries"`
}

// ChartEntry represents a single chart entry in the index
type ChartEntry struct {
	AppVersion string   `yaml:"appVersion"`
	URLs       []string `yaml:"urls"`
}

// ValuesFile represents the structure of values.yaml
type ValuesFile struct {
	Values map[string]interface{}
}

// LoadValuesFile loads and parses the values.yaml file
func LoadValuesFile(chartPath string) (*ValuesFile, error) {
	valuesPath := filepath.Join(chartPath, "values.yaml")
	content, err := os.ReadFile(valuesPath)
	if err != nil {
		return nil, fmt.Errorf("error reading values.yaml: %w", err)
	}

	values := &ValuesFile{
		Values: make(map[string]interface{}),
	}

	if err := yaml.Unmarshal(content, &values.Values); err != nil {
		return nil, fmt.Errorf("error parsing values.yaml: %w", err)
	}

	return values, nil
}

// resolveTemplateValue resolves a template expression like "{{ .Values.something.key }}"
// to its actual value from values.yaml
func resolveTemplateValue(templateExpr string, values *ValuesFile) (string, error) {
	// Remove {{ }} and spaces, handling potential whitespace between braces
	clean := strings.TrimSpace(templateExpr)
	if strings.HasPrefix(clean, "{{") && strings.HasSuffix(clean, "}}") {
		clean = clean[2 : len(clean)-2]
	}
	clean = strings.TrimSpace(clean)

	// Remove .Values. prefix
	clean = strings.TrimPrefix(clean, ".Values.")

	// If the path is empty after cleaning, return an error
	if clean == "" {
		return "", fmt.Errorf("empty template path after cleaning")
	}

	// Split the path
	parts := strings.Split(clean, ".")

	// Navigate through the values map
	var current interface{} = values.Values
	for _, part := range parts {
		part = strings.TrimSpace(part) // Handle any whitespace between path components
		if part == "" {
			continue // Skip empty parts
		}

		switch v := current.(type) {
		case map[string]interface{}:
			var ok bool
			current, ok = v[part]
			if !ok {
				return "", fmt.Errorf("key %s not found in values", part)
			}
		default:
			return "", fmt.Errorf("invalid path in values.yaml")
		}
	}

	// Convert the final value to string
	switch v := current.(type) {
	case string:
		return v, nil
	case []interface{}:
		// Handle array values
		jsonBytes, err := json.Marshal(v)
		if err != nil {
			return "", fmt.Errorf("error marshalling array value: %w", err)
		}
		return string(jsonBytes), nil
	default:
		return fmt.Sprintf("%v", v), nil
	}
}

func ChartInfoFromSpec(nfo *coreprovider.ChartInfo, extractPath string, rc *rest.Config) (rootDir string, err error) {
	if nfo == nil {
		return "", fmt.Errorf("chart infos cannot be nil")
	}

	opts := getter.GetOptions{
		URI:                   nfo.Url,
		Version:               nfo.Version,
		Repo:                  nfo.Repo,
		InsecureSkipVerifyTLS: nfo.InsecureSkipVerifyTLS,
	}

	if nfo.Credentials != nil {
		secret, err := secretsHelper.Get(context.TODO(), rc, &nfo.Credentials.PasswordRef)
		if err != nil {
			return "", fmt.Errorf("failed to get secret: %w", err)
		}
		opts.Username = nfo.Credentials.Username
		opts.Password = string(secret.Data[nfo.Credentials.PasswordRef.Key])
		opts.PassCredentialsAll = true
	}

	dat, _, err := getter.Get(opts)
	if err != nil {
		return "", err
	}
	return "", downloadAndExtractTgz(dat, extractPath)
}

// DownloadAndExtractTgz downloads a tgz file from a URL and extracts it
func downloadAndExtractTgz(tgz []byte, extractPath string) error {
	// Create a gzip reader
	bytesReader := bytes.NewReader(tgz)
	gzr, err := gzip.NewReader(bytesReader)
	if err != nil {
		return fmt.Errorf("error creating gzip reader: %v", err)
	}
	defer gzr.Close()

	// Create a tar reader
	tr := tar.NewReader(gzr)

	// Extract the contents
	log.Debug().Msgf("Extracting to %s...", extractPath)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("error reading tar: %v", err)
		}

		// Create the directory structure
		target := filepath.Join(extractPath, header.Name)
		dir := filepath.Dir(target)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("error creating directory: %v", err)
		}

		if header.Typeflag == tar.TypeDir {
			continue
		}

		// Create the file
		file, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
		if err != nil {
			return fmt.Errorf("error creating file: %v", err)
		}
		defer file.Close()

		// Copy the contents
		if _, err := io.Copy(file, tr); err != nil {
			return fmt.Errorf("error copying file contents: %v", err)
		}
	}

	log.Debug().Msg("Download and extraction completed successfully!")
	return nil
}

// ExtractFinopsResources extracts the finops resources from the file content
func ExtractFinopsResources(content, annotationKey string, chartPath string) ([]string, error) {
	var resources []string

	if strings.Contains(content, annotationKey) {
		lines := strings.Split(content, "\n")
		for _, line := range lines {
			if strings.Contains(line, annotationKey) {
				// Extract the value part after the colon
				parts := strings.SplitN(line, ":", 2)
				if len(parts) != 2 {
					continue
				}
				valuePart := strings.TrimSpace(parts[1])

				// Remove surrounding quotes if present
				valuePart = strings.Trim(valuePart, "'\"")

				// First try to unmarshal as is
				err := json.Unmarshal([]byte(valuePart), &resources)
				if err == nil {
					// If successful, check each resource for templates
					for i, resource := range resources {
						if strings.Contains(resource, "{{") && strings.Contains(resource, "}}") {
							values, err := LoadValuesFile(chartPath)
							if err != nil {
								log.Warn().Err(err).Msg("Failed to load values.yaml, using template as-is")
								continue
							}

							resolved, err := resolveTemplateValue(resource, values)
							if err != nil {
								log.Warn().Err(err).Msg("Failed to resolve template value, using template as-is")
								continue
							}
							resources[i] = resolved
						}
					}
					return resources, nil
				}

				// If direct unmarshal failed, try to resolve any templates first
				if strings.Contains(valuePart, "{{") && strings.Contains(valuePart, "}}") {
					// This is for handling the entire array as a template
					values, err := LoadValuesFile(chartPath)
					if err != nil {
						log.Warn().Err(err).Msg("Failed to load values.yaml, using template as-is")
					} else {
						resolved, err := resolveTemplateValue(valuePart, values)
						if err != nil {
							log.Warn().Err(err).Msg("Failed to resolve template value, using template as-is")
						} else {
							valuePart = resolved
						}
					}
				}

				// Try to unmarshal again after template resolution
				err = json.Unmarshal([]byte(valuePart), &resources)
				if err != nil {
					return nil, fmt.Errorf("error parsing annotation value: %v", err)
				}
				return resources, nil
			}
		}
	}
	return nil, nil
}

// ProcessTemplateFile processes a single template file
func ProcessTemplateFile(filePath, annotationLabel, chartPath string) ([]string, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return []string{}, fmt.Errorf("error reading file: %v", err)
	}

	log.Debug().Msgf("Processing %s:", filepath.Base(filePath))

	resources, err := ExtractFinopsResources(string(content), annotationLabel, chartPath)
	if err != nil {
		return []string{}, fmt.Errorf("error extracting resources: %v", err)
	}

	if resources != nil {
		log.Info().Msgf("Found finops resources: %v", resources)
		for _, resource := range resources {
			log.Info().Msgf("\t Resource: %s", resource)
		}
	}

	return resources, nil
}

// ProcessHelmTemplates processes all template files in the chart
func ProcessHelmTemplates(chartPath, annotationLabel string) (map[string]int, error) {
	templatesPath := filepath.Join(chartPath, "templates")

	if _, err := os.Stat(templatesPath); os.IsNotExist(err) {
		return map[string]int{}, fmt.Errorf("templates directory not found at %s", templatesPath)
	}

	resourceMap := map[string]int{}

	err := filepath.Walk(templatesPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			ext := filepath.Ext(path)
			if ext == ".yaml" || ext == ".yml" || ext == ".tpl" {
				if resources, err := ProcessTemplateFile(path, annotationLabel, chartPath); err == nil {
					for _, resource := range resources {
						resourceMap[resource]++
					}
				} else {
					log.Error().Err(err).Msgf("Error processing %s", filepath.Base(path))
				}
			}
		}
		return nil
	})

	for key := range resourceMap {
		log.Debug().Msgf("key: %s, value: %d", key, resourceMap[key])
	}
	return resourceMap, err
}

// CleanupDirectory removes a directory and all its contents
func CleanupDirectory(directory string) error {
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		log.Debug().Msgf("Cleaning up directory: %s", directory)
		if err := os.RemoveAll(directory); err != nil {
			return fmt.Errorf("error during cleanup: %v", err)
		}
		log.Debug().Msg("Cleanup completed successfully!")
	}
	return nil
}
