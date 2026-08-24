package response

import (
	"encoding/json"
	"log"
	"net/http"
)

func WriteJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		log.Printf("write response failed: %v", err)
	}
}

func WriteData(writer http.ResponseWriter, status int, data any) {
	WriteJSON(writer, status, map[string]any{
		"code":    0,
		"message": "ok",
		"data":    data,
	})
}

func WriteError(writer http.ResponseWriter, status int, message string) {
	WriteJSON(writer, status, map[string]any{
		"code":    status,
		"message": message,
		"data":    nil,
	})
}
