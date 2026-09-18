import assert from 'node:assert/strict'
import test from 'node:test'
import { NEW_DRAWING_NAME_PRESETS } from '../src/features/drawings/detail-tabs/preview/new-drawing-name-presets.ts'

// 清单来自工艺给的零件名称表：用户要求去掉清单序号和「注：…」这类说明文字后，
// 作为「新建图纸」名称栏的预设。名称会拼进文件名 `${图号}(${名称})`，所以格式也要能被文件名接受。

test('预设名称去掉了清单序号、说明文字与括号注释', () => {
  for (const name of NEW_DRAWING_NAME_PRESETS) {
    assert.doesNotMatch(name, /^\s*\d+\s*[.、．)）]/, `${name} 还带着清单序号`)
    assert.doesNotMatch(name, /[注:\s（）()]/, `${name} 还带着说明文字、空格或括号注释`)
  }
})

test('预设名称不重复，且都能直接进文件名', () => {
  assert.ok(NEW_DRAWING_NAME_PRESETS.length >= 20, '预设数量过少，像是漏抄了清单')
  assert.equal(new Set(NEW_DRAWING_NAME_PRESETS).size, NEW_DRAWING_NAME_PRESETS.length, '存在重复名称')
  for (const name of NEW_DRAWING_NAME_PRESETS) {
    assert.doesNotMatch(name, /[<>:"/\\|?*\u0000-\u001f]/, `${name} 含文件名非法字符`)
  }
})

test('清单里的零件名称都在，没被误删', () => {
  const expected = [
    '活塞杆', '活塞杆体', '杆坯1', '杆坯2', '活塞', '杆头坯',
    '整体导向套', '螺孔导向套', '导向套',
    '耳环缸头', '平底缸头', '法兰缸头', '铰轴缸头', '缸头坯A', '缸头坯B',
    '短杆头', '长杆头',
    '丝圈', '轴卡', '卡键', '孔卡', '挡环', '隔套', '压帽', '缸帽', '锁紧螺母',
    '铜套', '钢套', '衬套',
  ]
  for (const name of expected) assert.ok(NEW_DRAWING_NAME_PRESETS.includes(name), `缺少 ${name}`)
  assert.equal(NEW_DRAWING_NAME_PRESETS.length, expected.length, '预设数量与清单对不上')
})
