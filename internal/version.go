package internal

import (
	modulesdk "github.com/Muxcore-Media/core/sdk/go/module"
	manifest "github.com/Muxcore-Media/metadata-musicbrainz"
)

// Version is the module version reported to core and in the User-Agent. It is
// read from the embedded muxcore.json (ADR-0021).
var Version = modulesdk.ManifestVersion(manifest.ManifestJSON)
