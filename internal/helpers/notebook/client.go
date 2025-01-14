package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func CallNotebook(webserviceUrl string, compositionDefinitionId string, jsonObject []byte, dbUsername string, dbPassword string) error {
	parameters := map[string]string{
		"composition_id": compositionDefinitionId,
		"json_list":      string(jsonObject),
	}

	parametersJson, err := json.Marshal(parameters)
	if err != nil {
		return fmt.Errorf("error marshaling parameters: %v", err)
	}

	req, err := http.NewRequest("POST", webserviceUrl, bytes.NewBuffer(parametersJson))
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(dbUsername, dbPassword)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("error sending request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned non-200 status code: %d, body: %s", resp.StatusCode, string(body))
	}

	return nil
}
