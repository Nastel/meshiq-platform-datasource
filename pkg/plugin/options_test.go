package plugin

import (
	"testing"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

func TestBuildMeshIqDataSourceOptions_ParsesJSONDataAndSecret(t *testing.T) {
	settings := &backend.DataSourceInstanceSettings{
		JSONData:                []byte(`{"serviceUrl":"http://example.com","repositoryID":"Default$Org","enableCompletion":true,"completionServiceUrl":"http://completion.example.com"}`),
		DecryptedSecureJSONData: map[string]string{"accessToken": "secret-token"},
	}

	options, err := BuildMeshIqDataSourceOptions(settings)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if options.ServiceUrl != "http://example.com" {
		t.Errorf("ServiceUrl = %q, want %q", options.ServiceUrl, "http://example.com")
	}
	if options.RepositoryID != "Default$Org" {
		t.Errorf("RepositoryID = %q, want %q", options.RepositoryID, "Default$Org")
	}
	if !options.EnableCompletion {
		t.Error("EnableCompletion = false, want true")
	}
	if options.CompletionServiceUrl != "http://completion.example.com" {
		t.Errorf("CompletionServiceUrl = %q, want %q", options.CompletionServiceUrl, "http://completion.example.com")
	}
	if options.Token != "secret-token" {
		t.Errorf("Token = %q, want %q (must come from DecryptedSecureJSONData, not jsonData)", options.Token, "secret-token")
	}
}

func TestBuildMeshIqDataSourceOptions_MalformedJSONDataFails(t *testing.T) {
	settings := &backend.DataSourceInstanceSettings{
		JSONData:                []byte(`not valid json`),
		DecryptedSecureJSONData: map[string]string{"accessToken": "t"},
	}

	if _, err := BuildMeshIqDataSourceOptions(settings); err == nil {
		t.Error("expected an error for malformed jsonData, got nil")
	}
}

func TestBuildMeshIqDataSourceOptions_MissingServiceUrlFails(t *testing.T) {
	settings := &backend.DataSourceInstanceSettings{
		JSONData:                []byte(`{}`),
		DecryptedSecureJSONData: map[string]string{"accessToken": "t"},
	}

	_, err := BuildMeshIqDataSourceOptions(settings)
	if err == nil {
		t.Fatal("expected an error for a missing service URL, got nil")
	}
	if err.Error() != "service URL is required" {
		t.Errorf("error = %q, want %q", err.Error(), "service URL is required")
	}
}

func TestBuildMeshIqDataSourceOptions_MissingTokenFails(t *testing.T) {
	settings := &backend.DataSourceInstanceSettings{
		JSONData:                []byte(`{"serviceUrl":"http://example.com"}`),
		DecryptedSecureJSONData: map[string]string{},
	}

	_, err := BuildMeshIqDataSourceOptions(settings)
	if err == nil {
		t.Fatal("expected an error for a missing access token, got nil")
	}
	if err.Error() != "access token is required" {
		t.Errorf("error = %q, want %q", err.Error(), "access token is required")
	}
}

func TestValidateMeshIqDataSourceOptions(t *testing.T) {
	tests := []struct {
		name    string
		options MeshIqDataSourceOptions
		wantErr string
	}{
		{"valid", MeshIqDataSourceOptions{ServiceUrl: "http://x", Token: "t"}, ""},
		{"missing service URL", MeshIqDataSourceOptions{Token: "t"}, "service URL is required"},
		{"missing token", MeshIqDataSourceOptions{ServiceUrl: "http://x"}, "access token is required"},
		{"missing both (URL checked first)", MeshIqDataSourceOptions{}, "service URL is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMeshIqDataSourceOptions(&tt.options)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || err.Error() != tt.wantErr {
				t.Errorf("error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}
