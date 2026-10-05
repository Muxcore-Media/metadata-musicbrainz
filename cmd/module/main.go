// Command module runs the metadata-musicbrainz sidecar.
package main

import (
	"log/slog"
	"os"

	modulesdk "github.com/Muxcore-Media/core/sdk/go/module"

	"github.com/Muxcore-Media/metadata-musicbrainz/internal"
)

func main() {
	cfg := internal.ConfigFromEnv()
	mod, err := internal.NewModule(cfg)
	if err != nil {
		slog.Error("create module", "error", err)
		os.Exit(1)
	}
	insecure := os.Getenv("MUXCORE_INSECURE_DISABLE_TLS") == "true" || os.Getenv("MUXCORE_GRPC_INSECURE") == "true"
	if err := modulesdk.Run(modulesdk.Config{
		Module:   mod,
		Insecure: insecure,
	}); err != nil {
		slog.Error("module exited", "error", err)
		os.Exit(1)
	}
}
