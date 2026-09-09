package editing

import (
	"testing"

	"cadguanliq/internal/change"
)

// 编译期断言：工单服务必须满足编辑门禁接口，否则存档编辑授权会静默失效。
// 放在 editing 包测试中（editing 已依赖 change），避免 change→editing→change 的测试循环依赖。
var _ ChangeGate = (*change.PGService)(nil)

func TestChangeGateConformance(t *testing.T) {
	var _ ChangeGate = (*change.PGService)(nil)
}
