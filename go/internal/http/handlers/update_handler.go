package handlers

import (
	"log"
	"net/http"

	"cadguanliq/internal/config"
	"cadguanliq/internal/response"
	"cadguanliq/internal/update"
)

// UpdateLatest 供客户端检查新版本：优先读取数据库发布的最新版本，无库/无记录时回退环境变量。
func UpdateLatest(store *update.Store, cfg config.Config) http.HandlerFunc {
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
		if store != nil {
			record, err := store.Latest(request.Context())
			if err != nil {
				log.Printf("[更新] 读取最新版本失败，回退环境变量: %v", err)
			} else if record != nil {
				manifest = update.Manifest{
					Version:     record.Version,
					Notes:       record.Notes,
					PublishedAt: record.PublishedAt,
					DownloadURL: record.DownloadURL,
					Mandatory:   record.Mandatory,
				}
			}
		}
		result := update.Check(
			manifest,
			request.URL.Query().Get("current_version"),
			request.URL.Query().Get("platform"),
		)
		response.WriteData(writer, http.StatusOK, result)
	}
}
