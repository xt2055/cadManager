package task

import "fmt"

// 进度刻度是产品口径，不是算法实现细节，因此集中定义在这里，
// 页面不再自己拼百分比，避免看板与首页任务面板显示不同的完成度。
const (
	// progressUpload 图纸档案已建立但还没有图纸文件。
	progressUpload = 0
	// progressDrafting 图纸文件已就位，处于编制阶段。
	progressDrafting = 30
	// progressReviewBase 进入审核就位的基础进度。
	progressReviewBase = 60
	// progressReviewSpan 审核节点进度可贡献的最大增量。
	progressReviewSpan = 30
	// progressDelivered 审核通过后的交付态。
	progressDelivered = 100
)

// reviewProgress 依审核节点完成比例给出百分比，节点信息缺失时退回审核基准值。
func reviewProgress(done, total int) int {
	if total <= 0 || done <= 0 {
		return progressReviewBase
	}
	if done > total {
		done = total
	}
	value := progressReviewBase + progressReviewSpan*done/total
	if value > progressReviewBase+progressReviewSpan {
		value = progressReviewBase + progressReviewSpan
	}
	return value
}

// Derive 由图纸生命周期推导任务进度。
//
// 推导而不人工维护，是因为图纸状态本身已经表达了真实进度：草稿、审核中、生产中、
// 已存档。再让人手工标记「进行中」只会产生两份互相矛盾的进度记录。
// 每一项都带下一步提示，符合「先给下一步，再给人看数据」。
func Derive(status string, fileCount int, reviewNode string, reviewDone, reviewTotal int) Progress {
	switch status {
	case "draft":
		if fileCount <= 0 {
			return Progress{
				Percent: progressUpload,
				Stage:   "待上传图纸",
				Detail:  "图纸档案已建立，还没有图纸文件；上传图纸文件后即可开始编制。",
			}
		}
		return Progress{
			Percent: progressDrafting,
			Stage:   "编制中",
			Detail:  "图纸文件已就位，完成编制后即可发起审核。",
		}
	case "reviewing":
		detail := "审核进行中，等待各节点签署。"
		if reviewTotal > 0 {
			detail = fmt.Sprintf("审核进行中：已完成 %d/%d 个节点。", reviewDone, reviewTotal)
		}
		if reviewNode != "" {
			detail = fmt.Sprintf("审核进行中：当前节点「%s」，已完成 %d/%d 个节点。", reviewNode, reviewDone, reviewTotal)
		}
		return Progress{
			Percent: reviewProgress(reviewDone, reviewTotal),
			Stage:   "审核中",
			Detail:  detail,
		}
	case "published":
		return Progress{
			Percent: progressDelivered,
			Stage:   "生产中",
			Detail:  "审核已通过，图纸已投入生产；归档后进入只读保护。",
			Done:    true,
		}
	case "archived":
		return Progress{
			Percent: progressDelivered,
			Stage:   "已存档",
			Detail:  "图纸已正式存档并进入只读保护；如需修改请发起变更工单。",
			Done:    true,
		}
	case "disabled":
		return Progress{
			Percent: progressUpload,
			Stage:   "已停用",
			Detail:  "图纸已停用，不再进入编制与审核流程；如需恢复请联系管理员。",
		}
	default:
		return Progress{
			Percent: progressUpload,
			Stage:   "状态未知",
			Detail:  "图纸状态无法识别，请刷新后重试。",
		}
	}
}
