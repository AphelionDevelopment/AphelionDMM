package app

import (
	"context"
	"runtime"

	"sdmm/internal/app/selfupdate"
	"sdmm/internal/env"
	"sdmm/internal/req"
	"sdmm/internal/util/slice"

	"github.com/rs/zerolog/log"
)

var remoteManifest selfupdate.Manifest

func (a *app) checkForUpdates() {
	a.checkForUpdatesV(false)
}

func (a *app) checkForUpdatesV(forceAvailable bool) {
	log.Print("checking for self updates...")

	manifest, err := selfupdate.FetchRemoteManifest()
	if err != nil {
		log.Printf("unable to fetch remote manifest: %v", err)
		return
	}

	remoteManifest = manifest

	if manifest.Version == env.Version {
		log.Print("application is up to date!")
		return
	}
	if slice.StrContains(a.config().UpdateIgnore, manifest.Version) && !forceAvailable {
		log.Print("ignoring update:", manifest.Version)
		return
	}

	log.Print("new update available:", manifest.Version)

	a.menu.SetUpdateAvailable(manifest.Version, manifest.Description)

	// Do force update only if we're using a concrete editor version.
	//goland:noinspection GoBoolExpressions
	if a.Prefs().Application.AutoUpdate && env.Version != env.Undefined {
		a.selfUpdate()
	}
}

func (a *app) selfUpdate() {
	a.menu.SetUpdating()

	// APHELION EDIT CHANGE - SECURE_UPDATER - ORIGINAL: var updateDownloadLink string
	var updateArtifact selfupdate.Artifact

	switch runtime.GOOS {
	case "windows":
		// APHELION EDIT CHANGE - SECURE_UPDATER - ORIGINAL: updateDownloadLink = remoteManifest.DownloadLinks.Windows
		updateArtifact = remoteManifest.DownloadLinks.Windows
	case "linux":
		// APHELION EDIT CHANGE - SECURE_UPDATER - ORIGINAL: updateDownloadLink = remoteManifest.DownloadLinks.Linux
		updateArtifact = remoteManifest.DownloadLinks.Linux
	case "darwin":
		// APHELION EDIT CHANGE - SECURE_UPDATER - ORIGINAL: updateDownloadLink = remoteManifest.DownloadLinks.MacOS
		updateArtifact = remoteManifest.DownloadLinks.MacOS
	}

	// APHELION EDIT CHANGE - SECURE_UPDATER - ORIGINAL: log.Print("updating with:", updateDownloadLink)
	log.Print("updating with:", updateArtifact.URL)

	go func() {
		// APHELION EDIT CHANGE - SECURE_UPDATER - ORIGINAL: latestUpdate, err := req.Get(updateDownloadLink)
		latestUpdate, err := req.GetWithClient(context.Background(), req.NewClient(), updateArtifact.URL, req.Options{
			MaxBytes: req.DefaultMaxBodyBytes,
			ContentTypes: []string{
				"application/octet-stream",
				"application/x-msdownload",
				"binary/octet-stream",
			},
		})
		if err != nil {
			log.Print("unable to get latest update:", err)
			a.menu.SetUpdateError()
			return
		}

		// APHELION EDIT CHANGE - SECURE_UPDATER - ORIGINAL: if err = selfupdate.Update(latestUpdate); err != nil {
		if err = selfupdate.Update(latestUpdate, updateArtifact); err != nil {
			log.Print("unable to complete self update:", err)
			a.menu.SetUpdateError()
			return
		}

		a.menu.SetUpdated()

		log.Print("self update completed successfully!")
	}()
}
