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

func ExtractFinopsResources(content, annotationKey string) ([]string, error) {
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

				// Parse JSON array
				err := json.Unmarshal([]byte(valuePart), &resources)
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
func ProcessTemplateFile(filePath, annotationLabel string) ([]string, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return []string{}, fmt.Errorf("error reading file: %v", err)
	}

	log.Debug().Msgf("Processing %s:", filepath.Base(filePath))

	resources, err := ExtractFinopsResources(string(content), annotationLabel)
	if err != nil {
		return []string{}, fmt.Errorf("error extracting resources: %v", err)
	}

	if resources != nil {
		log.Debug().Msgf("Found finops resources: %v", resources)
		for _, resource := range resources {
			log.Debug().Msgf("Resource: %s", resource)
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
				if resources, err := ProcessTemplateFile(path, annotationLabel); err == nil {
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
