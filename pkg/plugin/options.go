package plugin

import (
	"encoding/json"
	"errors"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

// MeshIqDataSourceOptions holds the datasource instance configuration.
// Secrets (the access token) live in DecryptedSecureJSONData, not jsonData.
type MeshIqDataSourceOptions struct {
	ServiceUrl string `json:"serviceUrl"`
	Token      string `json:"-"`
}

// BuildMeshIqDataSourceOptions parses and validates the datasource settings.
func BuildMeshIqDataSourceOptions(settings *backend.DataSourceInstanceSettings) (*MeshIqDataSourceOptions, error) {
	var options MeshIqDataSourceOptions
	if err := json.Unmarshal(settings.JSONData, &options); err != nil {
		return nil, err
	}

	options.Token = settings.DecryptedSecureJSONData["accessToken"]

	if err := validateMeshIqDataSourceOptions(&options); err != nil {
		return nil, err
	}

	return &options, nil
}

func validateMeshIqDataSourceOptions(options *MeshIqDataSourceOptions) error {
	if options.ServiceUrl == "" {
		return errors.New("invalid service URL")
	}
	if options.Token == "" {
		return errors.New("invalid access token")
	}
	return nil
}
