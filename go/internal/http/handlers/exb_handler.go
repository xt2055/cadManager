package handlers

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"cadguanliq/internal/attachment"
	"cadguanliq/internal/cadtext"
	"cadguanliq/internal/converter"
	"cadguanliq/internal/exb"
	"cadguanliq/internal/http/middleware"
	"cadguanliq/internal/response"
	"cadguanliq/internal/storage"
)

type exbParseRequest struct {
	StorageKey string `json:"storageKey"`
}

type reidentifyPartRequest struct {
	StorageKey string `json:"storageKey"`
	PartNo     string `json:"partNo"`
}

type drawingDesignerResponse struct {
	Designer   string `json:"designer"`
	StorageKey string `json:"storageKey,omitempty"`
}

func ScanDrawingDesigner(repository attachment.Repository, objectStorage storage.ObjectStorage) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if _, ok := middleware.UserFromContext(request.Context()); !ok {
			response.WriteError(writer, http.StatusUnauthorized, "登录已失效，请重新登录")
			return
		}
		if request.Method != http.MethodPost {
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		drawingNo := strings.TrimSpace(request.URL.Query().Get("drawingNo"))
		if drawingNo == "" {
			var input struct {
				DrawingNo string `json:"drawingNo"`
			}
			if err := decodeJSON(request, &input); err != nil {
				response.WriteError(writer, http.StatusBadRequest, "drawingNo 必填且请求格式有效")
				return
			}
			drawingNo = strings.TrimSpace(input.DrawingNo)
		}
		if drawingNo == "" {
			response.WriteError(writer, http.StatusBadRequest, "drawingNo 必填")
			return
		}

		attachments, err := repository.ListByDrawing(request.Context(), drawingNo)
		if err != nil {
			response.WriteError(writer, http.StatusInternalServerError, "查询图纸附件失败")
			return
		}

		for _, item := range attachments {
			if !strings.EqualFold(filepathExt(item.Name), ".exb") && !strings.EqualFold(filepathExt(item.StorageKey), ".exb") {
				continue
			}
			reader, _, openErr := objectStorage.Open(request.Context(), item.StorageKey)
			if openErr != nil {
				continue
			}
			data, readErr := io.ReadAll(reader)
			_ = reader.Close()
			if readErr != nil {
				continue
			}
			parsed, parseErr := exb.Parse(data)
			if parseErr != nil {
				continue
			}
			designer := strings.TrimSpace(parsed.TitleBlock["设计"])
			if designer == "" || designer == "待定" {
				continue
			}
			response.WriteData(writer, http.StatusOK, drawingDesignerResponse{Designer: designer, StorageKey: item.StorageKey})
			return
		}

		response.WriteData(writer, http.StatusOK, drawingDesignerResponse{})
	}
}

func ParseEXB(repository attachment.Repository, objectStorage storage.ObjectStorage) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if _, ok := middleware.UserFromContext(request.Context()); !ok {
			response.WriteError(writer, http.StatusUnauthorized, "登录已失效，请重新登录")
			return
		}
		if request.Method != http.MethodPost {
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		var input exbParseRequest
		if err := decodeJSON(request, &input); err != nil || strings.TrimSpace(input.StorageKey) == "" {
			response.WriteError(writer, http.StatusBadRequest, "storageKey 必填且请求格式有效")
			return
		}
		if !strings.EqualFold(filepathExt(input.StorageKey), ".exb") {
			response.WriteError(writer, http.StatusBadRequest, "只支持 EXB 文件")
			return
		}
		if _, err := repository.Find(request.Context(), input.StorageKey); err != nil {
			if errors.Is(err, attachment.ErrNotFound) {
				response.WriteError(writer, http.StatusNotFound, "附件不存在")
			} else {
				response.WriteError(writer, http.StatusInternalServerError, "查询附件失败")
			}
			return
		}

		reader, _, err := objectStorage.Open(request.Context(), input.StorageKey)
		if err != nil {
			response.WriteError(writer, http.StatusNotFound, "附件文件不存在")
			return
		}
		defer reader.Close()
		data, err := io.ReadAll(reader)
		if err != nil {
			response.WriteError(writer, http.StatusInternalServerError, "读取 EXB 文件失败")
			return
		}
		result, err := exb.Parse(data)
		if err != nil {
			response.WriteError(writer, http.StatusUnprocessableEntity, "EXB 文件解析失败")
			return
		}
		response.WriteData(writer, http.StatusOK, result)
	}
}

// IdentifyDrawingFile 优先从图纸标题栏读取图号，缺失时回退到文件名。
// EXB 直接解析；DWG/DXF 先经 CAXA 转为临时 EXB 后解析。
func IdentifyDrawingFile(convService *converter.Service) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if _, ok := middleware.UserFromContext(request.Context()); !ok {
			response.WriteError(writer, http.StatusUnauthorized, "登录已失效，请重新登录")
			return
		}
		if request.Method != http.MethodPost {
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		file, header, err := request.FormFile("file")
		if err != nil {
			log.Printf("[EXB识别] 获取上传文件失败: err=%v", err)
			response.WriteError(writer, http.StatusBadRequest, "缺少待识别的图纸文件")
			return
		}
		defer file.Close()

		ext := strings.ToLower(filepath.Ext(header.Filename))
		log.Printf("[EXB识别] 开始: filename=%q ext=%q size=%d", header.Filename, ext, header.Size)
		if ext != ".exb" && ext != ".dwg" && ext != ".dxf" {
			log.Printf("[EXB识别] 拒绝不支持的文件: filename=%q ext=%q", header.Filename, ext)
			response.WriteError(writer, http.StatusBadRequest, "只支持 EXB/DWG/DXF 文件读取图号")
			return
		}

		inputFile, err := os.CreateTemp("", "cadguanliq-identify-*"+ext)
		if err != nil {
			log.Printf("[EXB识别] 创建临时文件失败: filename=%q err=%v", header.Filename, err)
			response.WriteError(writer, http.StatusInternalServerError, "创建图纸识别临时文件失败")
			return
		}
		inputPath := inputFile.Name()
		defer os.Remove(inputPath)
		if _, err := io.Copy(inputFile, file); err != nil {
			log.Printf("[EXB识别] 写入临时文件失败: filename=%q temp=%q err=%v", header.Filename, inputPath, err)
			inputFile.Close()
			response.WriteError(writer, http.StatusInternalServerError, "保存图纸识别临时文件失败")
			return
		}
		if err := inputFile.Close(); err != nil {
			log.Printf("[EXB识别] 关闭临时文件失败: filename=%q temp=%q err=%v", header.Filename, inputPath, err)
			response.WriteError(writer, http.StatusInternalServerError, "关闭图纸识别临时文件失败")
			return
		}
		if ext == ".dxf" {
			if err := cadtext.NormalizeDxfFileForCaxa(inputPath); err != nil {
				log.Printf("[EXB识别] DXF 规范化失败: filename=%q temp=%q err=%v", header.Filename, inputPath, err)
				response.WriteError(writer, http.StatusUnprocessableEntity, err.Error())
				return
			}
		}

		exbPath := inputPath
		if ext != ".exb" {
			if convService == nil {
				log.Printf("[EXB识别] 转换服务未启动: filename=%q temp=%q", header.Filename, inputPath)
				response.WriteError(writer, http.StatusServiceUnavailable, "CAD 转换服务未启动")
				return
			}
			exbPath = inputPath + ".exb"
			defer os.Remove(exbPath)
			log.Printf("[EXB识别] 开始转换: filename=%q input=%q output=%q", header.Filename, inputPath, exbPath)
			if err := convService.ConvertPathToExb(request.Context(), inputPath, exbPath); err != nil {
				log.Printf("[EXB识别] 转换失败: filename=%q input=%q output=%q err=%v", header.Filename, inputPath, exbPath, err)
				response.WriteError(writer, http.StatusUnprocessableEntity, "图纸转换失败，无法读取内部图号: "+err.Error())
				return
			}
		}

		result, err := exb.NewParser("").ParseFile(request.Context(), exbPath)
		if err != nil {
			log.Printf("[EXB识别] 解析失败: filename=%q exb=%q err=%v", header.Filename, exbPath, err)
			response.WriteError(writer, http.StatusUnprocessableEntity, "读取图纸内部图号失败: "+err.Error())
			return
		}
		titleBlockOnly := request.FormValue("titleBlockOnly") == "true"
		partNo, partNoSource := resolvePartNo(header.Filename, result.TitleBlock, titleBlockOnly)
		if partNoSource != "titleBlock" {
			log.Printf("[EXB识别] 标题栏未找到图号，已回退文件名: filename=%q exb=%q titleBlock=%v fallbackPartNo=%q source=%s", header.Filename, exbPath, result.TitleBlock, partNo, partNoSource)
		} else {
			log.Printf("[EXB识别] 识别成功: filename=%q exb=%q partNo=%q source=titleBlock", header.Filename, exbPath, partNo)
		}
		response.WriteData(writer, http.StatusOK, map[string]any{
			"partNo":       partNo,
			"material":     firstTitleBlockValue(result.TitleBlock, "材料名称", "材料", "材质"),
			"titleBlock":   result.TitleBlock,
			"partNoSource": partNoSource,
		})
	}
}

// IdentifyDrawingMaterial 只读取标题栏材料，不要求文件必须包含可识别的图号。
func IdentifyDrawingMaterial(convService *converter.Service) http.HandlerFunc {
	return identifyDrawingFields(convService, false)
}

func identifyDrawingFields(convService *converter.Service, requirePartNo bool) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if _, ok := middleware.UserFromContext(request.Context()); !ok {
			response.WriteError(writer, http.StatusUnauthorized, "登录已失效，请重新登录")
			return
		}
		if request.Method != http.MethodPost {
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		file, header, err := request.FormFile("file")
		if err != nil {
			log.Printf("[CAD字段识别] 获取上传文件失败: err=%v", err)
			response.WriteError(writer, http.StatusBadRequest, "缺少待识别的图纸文件")
			return
		}
		defer file.Close()

		ext := strings.ToLower(filepath.Ext(header.Filename))
		log.Printf("[CAD字段识别] 开始: filename=%q ext=%q size=%d requirePartNo=%t", header.Filename, ext, header.Size, requirePartNo)
		if ext != ".exb" && ext != ".dwg" && ext != ".dxf" {
			response.WriteError(writer, http.StatusBadRequest, "只支持 EXB/DWG/DXF 文件读取标题栏")
			return
		}
		inputFile, err := os.CreateTemp("", "cadguanliq-fields-*"+ext)
		if err != nil {
			log.Printf("[CAD字段识别] 创建临时文件失败: filename=%q err=%v", header.Filename, err)
			response.WriteError(writer, http.StatusInternalServerError, "创建图纸识别临时文件失败")
			return
		}
		inputPath := inputFile.Name()
		defer os.Remove(inputPath)
		if _, err := io.Copy(inputFile, file); err != nil {
			log.Printf("[CAD字段识别] 写入临时文件失败: filename=%q temp=%q err=%v", header.Filename, inputPath, err)
			_ = inputFile.Close()
			response.WriteError(writer, http.StatusInternalServerError, "保存图纸识别临时文件失败")
			return
		}
		if err := inputFile.Close(); err != nil {
			log.Printf("[CAD字段识别] 关闭临时文件失败: filename=%q temp=%q err=%v", header.Filename, inputPath, err)
			response.WriteError(writer, http.StatusInternalServerError, "关闭图纸识别临时文件失败")
			return
		}
		if ext == ".dxf" {
			if err := cadtext.NormalizeDxfFileForCaxa(inputPath); err != nil {
				log.Printf("[CAD字段识别] DXF 规范化失败: filename=%q temp=%q err=%v", header.Filename, inputPath, err)
				response.WriteError(writer, http.StatusUnprocessableEntity, err.Error())
				return
			}
		}
		if ext != ".exb" {
			if convService == nil {
				log.Printf("[CAD字段识别] 转换服务未启动: filename=%q temp=%q", header.Filename, inputPath)
				response.WriteError(writer, http.StatusServiceUnavailable, "CAD 转换服务未启动")
				return
			}
		}
		exbPath := inputPath
		if ext != ".exb" {
			exbPath = inputPath + ".exb"
			defer os.Remove(exbPath)
			log.Printf("[CAD字段识别] 开始转换: filename=%q input=%q output=%q", header.Filename, inputPath, exbPath)
			if err := convService.ConvertPathToExb(request.Context(), inputPath, exbPath); err != nil {
				log.Printf("[CAD字段识别] 转换失败: filename=%q input=%q output=%q err=%v", header.Filename, inputPath, exbPath, err)
				response.WriteError(writer, http.StatusUnprocessableEntity, "图纸转换失败，无法读取标题栏: "+err.Error())
				return
			}
		}
		result, err := exb.NewParser("").ParseFile(request.Context(), exbPath)
		if err != nil {
			log.Printf("[CAD字段识别] 解析失败: filename=%q exb=%q err=%v", header.Filename, exbPath, err)
			response.WriteError(writer, http.StatusUnprocessableEntity, "读取图纸标题栏失败: "+err.Error())
			return
		}
		partNo, partNoSource := resolvePartNo(header.Filename, result.TitleBlock, false)
		material := firstTitleBlockValue(result.TitleBlock, "材料名称", "材料", "材质")
		if partNoSource != "titleBlock" {
			log.Printf("[CAD字段识别] 标题栏未找到图号，已回退文件名: filename=%q exb=%q titleBlock=%v fallbackPartNo=%q source=%s", header.Filename, exbPath, result.TitleBlock, partNo, partNoSource)
		} else {
			log.Printf("[CAD字段识别] 识别成功: filename=%q exb=%q partNo=%q source=titleBlock", header.Filename, exbPath, partNo)
		}
		response.WriteData(writer, http.StatusOK, map[string]any{
			"partNo":       partNo,
			"material":     material,
			"titleBlock":   result.TitleBlock,
			"partNoSource": partNoSource,
		})
	}
}

// ReidentifyPart 根据图纸内部图号校正历史附件的零件关联。
func ReidentifyPart(repository attachment.Repository) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		user, ok := middleware.UserFromContext(request.Context())
		if !ok {
			response.WriteError(writer, http.StatusUnauthorized, "登录已失效，请重新登录")
			return
		}
		if request.Method != http.MethodPost {
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var input reidentifyPartRequest
		if err := decodeJSON(request, &input); err != nil || strings.TrimSpace(input.StorageKey) == "" || strings.TrimSpace(input.PartNo) == "" {
			response.WriteError(writer, http.StatusBadRequest, "storageKey 和 partNo 必填且请求格式有效")
			return
		}
		result, err := repository.ReidentifyPart(request.Context(), strings.TrimSpace(input.StorageKey), strings.TrimSpace(input.PartNo), user.ID)
		if err != nil {
			if errors.Is(err, attachment.ErrNotFound) {
				response.WriteError(writer, http.StatusNotFound, "附件不存在")
				return
			}
			if errors.Is(err, attachment.ErrConflict) {
				response.WriteError(writer, http.StatusConflict, "目标零件图号已存在且无法合并")
				return
			}
			response.WriteError(writer, http.StatusUnprocessableEntity, err.Error())
			return
		}
		response.WriteData(writer, http.StatusOK, result)
	}
}

func firstTitleBlockValue(fields map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(fields[key]); value != "" {
			return value
		}
	}
	return ""
}

func resolvePartNo(filename string, titleBlock map[string]string, titleBlockOnly bool) (string, string) {
	titleBlockNo := firstTitleBlockValue(titleBlock, "图纸编号", "图号", "零件图号", "零件号", "零件代号", "代号")
	filenameNo := fallbackPartNoFromFilename(filename)
	if titleBlockOnly {
		if titleBlockNo != "" && isLikelyDrawingNo(titleBlockNo) {
			return titleBlockNo, "titleBlock"
		}
		return "", "none"
	}
	// 零件文件名通常是唯一可靠的工程编号；标题栏中可能出现尺寸、标准号或材料值。
	// 总图需要标题栏时由 titleBlockOnly 分支单独处理。
	if filenameNo != "" {
		return filenameNo, "filename"
	}
	if titleBlockNo != "" && isLikelyDrawingNo(titleBlockNo) {
		return titleBlockNo, "titleBlock"
	}
	return "", "none"
}

func isLikelyDrawingNo(value string) bool {
	value = strings.TrimSpace(value)
	upper := strings.ToUpper(value)
	if value == "" || !strings.ContainsAny(value, "0123456789") || !strings.ContainsAny(upper, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") {
		return false
	}
	if strings.Contains(value, ":") || strings.ContainsAny(value, "×xX") || strings.Contains(value, " ") {
		return false
	}
	if strings.HasPrefix(upper, "GB") || strings.HasPrefix(upper, "JB") || strings.HasPrefix(upper, "ISO") || strings.HasPrefix(upper, "DIN") || strings.HasPrefix(upper, "HB") {
		return false
	}
	if strings.Contains(upper, "-MN-") || strings.HasSuffix(upper, "WD") || strings.HasSuffix(upper, "UNC") || strings.HasSuffix(upper, "UNF") {
		return false
	}
	if strings.Count(value, ".") == 1 && strings.IndexByte(value, '-') < 0 {
		allNumeric := true
		for _, character := range value {
			if character != '.' && (character < '0' || character > '9') {
				allNumeric = false
				break
			}
		}
		if allNumeric {
			return false
		}
	}
	return true
}

func fallbackPartNoFromFilename(filename string) string {
	name := filepath.Base(strings.TrimSpace(filename))
	name = strings.TrimSuffix(name, filepath.Ext(name))
	if name == "" {
		return ""
	}
	var builder strings.Builder
	for index, character := range name {
		if index == 0 {
			if !isASCIIAlphaNumeric(character) {
				return ""
			}
		} else if !isASCIIAlphaNumeric(character) && character != '.' && character != '-' && character != '/' {
			break
		}
		builder.WriteRune(character)
	}
	candidate := strings.TrimRight(builder.String(), ".-/")
	if candidate == "" || !strings.ContainsAny(candidate, "0123456789") || !strings.ContainsAny(candidate, ".-/") {
		return ""
	}
	return candidate
}

func isASCIIAlphaNumeric(character rune) bool {
	return character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9'
}

// PreviewEXB 保留旧版 DXF 预览接口，当前 MLightCAD 直接读取 /api/cad/source 返回的 DWG。
func PreviewEXB(repository attachment.Repository, objectStorage storage.ObjectStorage, convService *converter.Service) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if _, ok := middleware.UserFromContext(request.Context()); !ok {
			response.WriteError(writer, http.StatusUnauthorized, "登录已失效，请重新登录")
			return
		}
		if request.Method != http.MethodGet {
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		storageKey := strings.TrimSpace(request.URL.Query().Get("storageKey"))
		if storageKey != "" && !strings.EqualFold(filepathExt(storageKey), ".exb") && !strings.EqualFold(filepathExt(storageKey), ".dwg") && !strings.EqualFold(filepathExt(storageKey), ".dxf") {
			response.WriteError(writer, http.StatusBadRequest, "只支持 EXB/DWG/DXF 文件")
			return
		}

		var item attachment.Attachment
		var err error
		if storageKey != "" {
			item, err = repository.Find(request.Context(), storageKey)
		}
		if err != nil || storageKey == "" {
			fileName := strings.TrimSpace(request.URL.Query().Get("fileName"))
			drawingNo := strings.TrimSpace(request.URL.Query().Get("drawingNo"))
			partNo := strings.TrimSpace(request.URL.Query().Get("partNo"))
			if fileName != "" && drawingNo != "" {
				if byOwnerItem, byOwnerErr := repository.FindByOwnerAndName(request.Context(), drawingNo, partNo, fileName); byOwnerErr == nil {
					item = byOwnerItem
					storageKey = item.StorageKey
					err = nil
				}
			}
		}
		if err != nil {
			if errors.Is(err, attachment.ErrNotFound) {
				response.WriteError(writer, http.StatusNotFound, "附件不存在")
			} else {
				response.WriteError(writer, http.StatusInternalServerError, "查询附件失败")
			}
			return
		}

		ext := filepathExt(storageKey)
		if ext == "" {
			ext = filepathExt(item.Name)
		}

		dxfKey := strings.TrimSuffix(storageKey, ext) + ".dxf"
		if strings.EqualFold(ext, ".dxf") {
			dxfKey = storageKey
		}

		// 已废弃：前端不再请求 DXF 预览；以下检查逻辑保留，供旧客户端迁移回溯。
		// needConvert := false
		// if reader, info, err := objectStorage.Open(request.Context(), dxfKey); err == nil {
		// 	reader.Close()
		// 	if info.Size == 0 {
		// 		_ = objectStorage.Delete(request.Context(), dxfKey)
		// 		needConvert = true
		// 	}
		// } else {
		// 	needConvert = true
		// }

		// 已废弃：旧 Canvas 预览曾在此处把任务插入 DXF 转换队列。
		// 当前后端队列只生成 MLightCAD 使用的 DWG，不能再由此接口触发 DXF 转换。
		// if needConvert {
		// 	if convService != nil && (strings.EqualFold(ext, ".exb") || strings.EqualFold(ext, ".dwg")) {
		// 		doneChan := convService.PushJob(item, converter.PriorityHigh)
		// 		select {
		// 		case convertErr := <-doneChan:
		// 			if convertErr != nil {
		// 				response.WriteError(writer, http.StatusUnprocessableEntity, convertErr.Error())
		// 				return
		// 			}
		// 		case <-time.After(60 * time.Second):
		// 			response.WriteError(writer, http.StatusGatewayTimeout, "CAD 转换 DXF 超时")
		// 			return
		// 		}
		// 	}
		// }

		// 读取 DXF
		reader, obj, err := objectStorage.Open(request.Context(), dxfKey)
		if err != nil || obj.Size == 0 {
			if reader != nil {
				reader.Close()
			}
			response.WriteError(writer, http.StatusNotFound, "高清矢量 DXF 文件未就绪或为空")
			return
		}
		defer reader.Close()

		writer.Header().Set("Content-Type", "application/dxf; charset=utf-8")
		writer.Header().Set("Content-Disposition", "inline; filename*=UTF-8''"+filepath.Base(dxfKey))
		writer.Header().Set("Cache-Control", "private, max-age=86400")
		writer.WriteHeader(http.StatusOK)
		_, _ = io.Copy(writer, reader)
		_ = obj
	}
}

// ConvertToEXB 将 DWG/DXF 附件转换为 EXB 格式。
// 支持返回 JSON 元数据或直接下载二进制流。
func ConvertToEXB(repository attachment.Repository, objectStorage storage.ObjectStorage, convService *converter.Service) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if _, ok := middleware.UserFromContext(request.Context()); !ok {
			response.WriteError(writer, http.StatusUnauthorized, "登录已失效，请重新登录")
			return
		}
		if request.Method != http.MethodPost && request.Method != http.MethodGet {
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		if convService == nil {
			response.WriteError(writer, http.StatusServiceUnavailable, "CAD 转换服务未启动")
			return
		}

		storageKey := strings.TrimSpace(request.URL.Query().Get("storageKey"))
		if storageKey == "" && request.Method == http.MethodPost {
			var input struct {
				StorageKey string `json:"storageKey"`
			}
			if err := decodeJSON(request, &input); err == nil {
				storageKey = strings.TrimSpace(input.StorageKey)
			}
		}

		if storageKey == "" {
			response.WriteError(writer, http.StatusBadRequest, "storageKey 必填")
			return
		}

		item, err := repository.Find(request.Context(), storageKey)
		if err != nil {
			writeAttachmentError(writer, err)
			return
		}

		exbKey, err := convService.ConvertToExb(request.Context(), item)
		if err != nil {
			response.WriteError(writer, http.StatusUnprocessableEntity, fmt.Sprintf("转换为 EXB 失败: %v", err))
			return
		}

		reader, object, err := objectStorage.Open(request.Context(), exbKey)
		if err != nil || object.Size == 0 {
			if reader != nil {
				reader.Close()
			}
			response.WriteError(writer, http.StatusInternalServerError, "读取生成的 EXB 文件失败")
			return
		}
		defer reader.Close()

		ext := filepathExt(item.Name)
		exbFileName := strings.TrimSuffix(item.Name, ext) + ".exb"

		// 如果请求要求下载二进制流 (通过 download 参数或 Accept 头)
		isDownload := request.URL.Query().Get("download") == "true" || request.URL.Query().Get("download") == "1" || request.Method == http.MethodGet
		if isDownload {
			writer.Header().Set("Content-Type", "application/octet-stream")
			writer.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(exbFileName))
			writer.Header().Set("Content-Length", fmt.Sprintf("%d", object.Size))
			_, _ = io.Copy(writer, reader)
			return
		}

		response.WriteData(writer, http.StatusOK, map[string]interface{}{
			"storageKey": exbKey,
			"name":       exbFileName,
			"size":       object.Size,
		})
	}
}

// CADSource 返回 CAD 引擎应直接读取的原始或转换后文件。
// EXB 返回后台生成的 DWG，DWG/DXF 返回原文件。
func CADSource(repository attachment.Repository, objectStorage storage.ObjectStorage, convService *converter.Service) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if _, ok := middleware.UserFromContext(request.Context()); !ok {
			response.WriteError(writer, http.StatusUnauthorized, "登录已失效，请重新登录")
			return
		}
		if request.Method != http.MethodGet {
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		storageKey := strings.TrimSpace(request.URL.Query().Get("storageKey"))
		if storageKey == "" {
			response.WriteError(writer, http.StatusBadRequest, "storageKey 必填")
			return
		}
		item, err := repository.Find(request.Context(), storageKey)
		if err != nil {
			writeAttachmentError(writer, err)
			return
		}

		ext := filepathExt(storageKey)
		if ext == "" {
			ext = filepathExt(item.Name)
		}
		sourceKey := storageKey
		fileName := item.Name

		// 优先使用已就绪的当前 DWG 工作副本（需确认存储中物理存在）
		if item.CurrentStorageKey != "" && strings.EqualFold(filepathExt(item.CurrentStorageKey), ".dwg") {
			if reader, info, statErr := objectStorage.Open(request.Context(), item.CurrentStorageKey); statErr == nil && info.Size > 0 {
				_ = reader.Close()
				sourceKey = item.CurrentStorageKey
				fileName = item.CurrentName
				if fileName == "" {
					fileName = strings.TrimSuffix(item.Name, ext) + ".dwg"
				}
			} else {
				log.Printf("[CADSource] currentStorageKey 不存在或为空: %s，回退计算", item.CurrentStorageKey)
			}
		}

		if sourceKey == storageKey && strings.EqualFold(ext, ".exb") {
			if convService == nil {
				response.WriteError(writer, http.StatusServiceUnavailable, "CAD 转换服务未启动")
				return
			}
			sourceKey, err = convService.EnsureDwg(request.Context(), item)
			if err != nil {
				response.WriteError(writer, http.StatusUnprocessableEntity, err.Error())
				return
			}
			fileName = strings.TrimSuffix(fileName, filepathExt(fileName)) + ".dwg"
		}

		reader, object, err := objectStorage.Open(request.Context(), sourceKey)
		if err != nil || object.Size == 0 {
			if reader != nil {
				reader.Close()
			}
			response.WriteError(writer, http.StatusNotFound, "CAD 渲染源文件不存在或为空")
			return
		}
		defer reader.Close()
		writer.Header().Set("Content-Type", firstNonEmpty(object.MimeType, item.MimeType, "application/octet-stream"))
		writer.Header().Set("Content-Disposition", "inline; filename*=UTF-8''"+url.PathEscape(fileName))
		writer.Header().Set("Content-Length", fmt.Sprintf("%d", object.Size))
		writer.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		_, _ = io.Copy(writer, reader)
	}
}

func filepathExt(value string) string {
	value = strings.ReplaceAll(value, "\\", "/")
	index := strings.LastIndexByte(value, '.')
	if index < 0 {
		return ""
	}
	return value[index:]
}
