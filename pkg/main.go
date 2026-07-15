package main

import (
	"os"

	"github.com/Nastel/meshiq-platform-datasource/pkg/plugin"
	"github.com/grafana/grafana-plugin-sdk-go/backend/datasource"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
)

func main() {
	// Serve the plugin until Grafana shuts the process down (this call blocks). Manage
	// handles the instance life cycle: NewDatasource is called per datasource instance,
	// and a changed configuration disposes the old instance and creates a new one.
	if err := datasource.Manage("meshiq-platform-datasource", plugin.NewDatasource, datasource.ManageOpts{}); err != nil {
		log.DefaultLogger.Error("plugin serve failed", "error", err)
		os.Exit(1)
	}
}
