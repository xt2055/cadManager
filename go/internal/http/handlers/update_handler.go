package handlers

import (
	"net/http"

	"cadguanliq/internal/config"
	"cadguanliq/internal/response"
	"cadguanliq/internal/update"
)

func UpdateLatest(cfg config.Config) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		manifest := update.Manifest{
			Version:     cfg.UpdateVersion,
			Notes:       cfg.UpdateNotes,
			PublishedAt: cfg.UpdatePublished,
			DownloadURL: cfg.UpdateURL,
			Mandatory:   cfg.UpdateMandatory,
		}
		result := update.Check(
			manifest,
			request.URL.Query().Get("current_version"),
			request.URL.Query().Get("platform"),
		)
		response.WriteData(writer, http.StatusOK, result)
	}
}
