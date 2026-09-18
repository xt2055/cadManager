package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"cadguanliq/internal/annotation"
	"cadguanliq/internal/http/middleware"
	"cadguanliq/internal/response"
)

func AnnotationWorkspace(repo *annotation.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := middleware.UserFromContext(r.Context())
		if !ok {
			response.WriteError(w, 401, "请先登录")
			return
		}
		switch r.Method {
		case http.MethodGet:
			// history=1 表示显式回看历史轮次（只读）；缺省只服务当前轮次。
			item, err := repo.Load(r.Context(), r.URL.Query().Get("caseId"), r.URL.Query().Get("attachmentId"), user.ID, r.URL.Query().Get("history") == "1")
			if err != nil {
				annotationError(w, err)
				return
			}
			response.WriteData(w, 200, item)
		case http.MethodPut:
			var in annotation.SaveInput
			if err := decodeAnnotation(w, r, &in); err != nil {
				response.WriteError(w, 400, "批注数据无效或超过 4 MB")
				return
			}
			if err := annotation.Validate(in.Content); err != nil {
				response.WriteError(w, 400, err.Error())
				return
			}
			item, err := repo.Save(r.Context(), in, user.ID)
			if err != nil {
				annotationError(w, err)
				return
			}
			response.WriteData(w, 200, item)
		default:
			response.WriteError(w, 405, "method not allowed")
		}
	}
}
func AnnotationTemplates(repo *annotation.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := middleware.UserFromContext(r.Context())
		if !ok {
			response.WriteError(w, 401, "请先登录")
			return
		}
		admin := false
		for _, role := range user.Roles {
			if role == "admin" {
				admin = true
			}
		}
		switch r.Method {
		case http.MethodPut:
			var in annotation.Template
			if decodeAnnotation(w,r,&in)!=nil { response.WriteError(w,400,"话术内容无效");return }
			item,err:=repo.UpdateTemplate(r.Context(),in,user.ID,admin)
			if err!=nil { if errors.Is(err,annotation.ErrForbidden) { annotationError(w,err) } else { response.WriteError(w,400,"无法更新话术，请检查分类及内容长度") };return }
			response.WriteData(w,200,item)
		case http.MethodGet:
			items, err := repo.Templates(r.Context(), user.ID)
			if err != nil {
				annotationError(w, err)
				return
			}
			response.WriteData(w, 200, items)
		case http.MethodPost:
			var in annotation.Template
			if decodeAnnotation(w, r, &in) != nil {
				response.WriteError(w, 400, "话术内容无效")
				return
			}
			item, err := repo.CreateTemplate(r.Context(), in, user.ID, admin)
			if err != nil {
				if errors.Is(err, annotation.ErrForbidden) {
					annotationError(w, err)
				} else {
					response.WriteError(w, 400, "无法保存话术，请检查分类及内容长度")
				}
				return
			}
			response.WriteData(w, 201, item)
		case http.MethodDelete:
			if err := repo.DeleteTemplate(r.Context(), r.URL.Query().Get("id"), user.ID, admin); err != nil {
				annotationError(w, err)
				return
			}
			response.WriteData(w, 200, map[string]bool{"deleted": true})
		default:
			response.WriteError(w, 405, "method not allowed")
		}
	}
}

func AnnotationFiles(repo *annotation.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter,r *http.Request) {
		if _,ok:=middleware.UserFromContext(r.Context());!ok { response.WriteError(w,401,"请先登录");return }
		if r.Method!=http.MethodGet { response.WriteError(w,405,"method not allowed");return }
		items,err:=repo.Files(r.Context(),r.URL.Query().Get("caseId"));if err!=nil { annotationError(w,err);return };response.WriteData(w,200,items)
	}
}
func decodeAnnotation(w http.ResponseWriter, r *http.Request, target any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("unexpected trailing JSON")
	}
	return nil
}
func annotationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, annotation.ErrForbidden):
		response.WriteError(w, 403, err.Error())
	case errors.Is(err, annotation.ErrSuperseded):
		response.WriteError(w, 409, err.Error())
	case errors.Is(err, annotation.ErrConflict):
		response.WriteError(w, 409, err.Error())
	case errors.Is(err, annotation.ErrNotFound):
		response.WriteError(w, 404, err.Error())
	default:
		log.Printf("annotation: %v", err)
		response.WriteError(w, 500, "批注服务暂不可用，请稍后重试")
	}
}

// AnnotationHistory 返回逐轮批注归档：按图纸（或单个案例）列出每一轮每位审核员的批注，
// 供「标注历史」回看与只读回放。轮次是批注的隔离边界，历史不看当前轮次。
func AnnotationHistory(repo *annotation.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := middleware.UserFromContext(r.Context()); !ok {
			response.WriteError(w, 401, "请先登录")
			return
		}
		if r.Method != http.MethodGet {
			response.WriteError(w, 405, "method not allowed")
			return
		}
		drawingNo := strings.TrimSpace(r.URL.Query().Get("drawingNo"))
		caseID := strings.TrimSpace(r.URL.Query().Get("caseId"))
		if drawingNo == "" && caseID == "" {
			response.WriteError(w, 400, "请提供图号或审核案例")
			return
		}
		items, err := repo.History(r.Context(), drawingNo, caseID)
		if err != nil {
			annotationError(w, err)
			return
		}
		response.WriteData(w, 200, items)
	}
}
