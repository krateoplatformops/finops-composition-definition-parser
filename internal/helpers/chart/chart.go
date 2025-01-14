package chart

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v2"
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

func DownloadChart(chartURL, chartRepo, chartVersion, extractPath string) error {
	// Ensure URL ends with index.yaml
	if !strings.Contains(chartURL, "/index.yaml") {
		chartURL = strings.TrimRight(chartURL, "/") + "/index.yaml"
	}

	// Download and parse index.yaml
	resp, err := http.Get(chartURL)
	if err != nil {
		return fmt.Errorf("error downloading index.yaml: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("error downloading index.yaml: status code %d", resp.StatusCode)
	}

	var chartIndex ChartIndex
	decoder := yaml.NewDecoder(resp.Body)
	if err := decoder.Decode(&chartIndex); err != nil {
		return fmt.Errorf("error parsing index.yaml: %v", err)
	}

	// Find the specific chart version
	entries, exists := chartIndex.Entries[chartRepo]
	if !exists {
		return fmt.Errorf("chart repository %s not found", chartRepo)
	}

	var tgzFileURL string
	for _, chart := range entries {
		if chart.AppVersion == chartVersion && len(chart.URLs) > 0 {
			baseURL := strings.TrimSuffix(chartURL, "/index.yaml")
			tgzFileURL = baseURL + "/" + chart.URLs[0]
			break
		}
	}

	if tgzFileURL == "" {
		return fmt.Errorf("chart version %s not found", chartVersion)
	}

	return downloadAndExtractTgz(tgzFileURL, extractPath)
}

// DownloadAndExtractTgz downloads a tgz file from a URL and extracts it
func downloadAndExtractTgz(url string, extractPath string) error {
	// Download the file
	log.Debug().Msgf("Downloading from %s...", url)
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("error downloading file: %v", err)
	}
	defer resp.Body.Close()

	// Create a gzip reader
	gzr, err := gzip.NewReader(resp.Body)
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
