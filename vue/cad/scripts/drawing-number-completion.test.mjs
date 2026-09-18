import assert from 'node:assert/strict'
import test from 'node:test'
import { composeNewDrawingNo, composePartNo, defaultProjectNo, isBorrowedNumber, parentPartNoOf, validateManualPartNo } from '../src/utils/drawing-number-completion.ts'

// 图号的主来源是图幅，手工补录只是兜底；这里锁定补录时的拼接规则：
// 用户只填后几位，项目号自动带出来，借用件允许改成别的项目号。

test('composePartNo：只填后几位时自动补上项目号与前导分隔符', () => {
  assert.equal(composePartNo('2000W.02.03d', '-01-01c'), '2000W.02.03d-01-01c')
  assert.equal(composePartNo('2000W.02.03d', '01-01c'), '2000W.02.03d-01-01c')
  assert.equal(composePartNo('2000W.02.03d', '  01-01c  '), '2000W.02.03d-01-01c')
  assert.equal(composePartNo('2000W.02.03d', '-02'), '2000W.02.03d-02')
})

test('composePartNo：不重复拼接已有的项目号', () => {
  assert.equal(composePartNo('2000W.02.03d', '2000W.02.03d-01-01c'), '2000W.02.03d-01-01c')
  assert.equal(composePartNo('2000W.02.03d', '2000w.02.03d-01-01c'), '2000w.02.03d-01-01c')
})

test('composePartNo：借用件直接粘别族图号时原样接受，不改写', () => {
  // JG9055e 属于别的图号族，不能给它硬拼当前项目号。
  assert.equal(composePartNo('2000W.02.03d', 'JG9055e-5032-01'), 'JG9055e-5032-01')
  assert.equal(composePartNo('2000W.02.03d', 'JG1285-250/180-3255%x4070'), 'JG1285-250/180-3255%x4070')
})

test('composePartNo：短零件段不会被误判成自带根号', () => {
  // 01c 只有 3 个字符，必须补项目号，否则会生成不属于任何族的图号。
  assert.equal(composePartNo('2000W.02.03d', '01c'), '2000W.02.03d-01c')
  assert.equal(composePartNo('2000W.02.03d', '-01c'), '2000W.02.03d-01c')
})

test('composePartNo：单边缺失时退化为另一边的原值', () => {
  assert.equal(composePartNo('', '-01-01c'), '01-01c')
  assert.equal(composePartNo('2000W.02.03d-', '-01-01c'), '2000W.02.03d-01-01c')
  assert.equal(composePartNo('2000W.02.03d', ''), '2000W.02.03d')
  assert.equal(composePartNo('', ''), '')
})

// 详情页「新建图纸」：项目号已经由当前图纸带出，用户只填后几位。
test('composeNewDrawingNo：带出的项目号直接架在前面，只补后几位', () => {
  assert.equal(composeNewDrawingNo('2000W.02.03d', '01'), '2000W.02.03d-01')
  assert.equal(composeNewDrawingNo('2000W.02.03d', '-01'), '2000W.02.03d-01')
  assert.equal(composeNewDrawingNo('2000W.02.03d', '01-01c'), '2000W.02.03d-01-01c')
  // 项目号自己是完整编号（可能含层级与斜杠）时整段保留，新零件挂在它下面。
  assert.equal(composeNewDrawingNo('JG9055e-50/32-00', '01'), 'JG9055e-50/32-00-01')
  // 借用件：用户整段粘别族图号，原样接受。
  assert.equal(composeNewDrawingNo('2000W.02.03d', 'JG9055e-5032-01'), 'JG9055e-5032-01')
})

test('composeNewDrawingNo：后几位没填时返回空串，不能把项目号当新图号', () => {
  assert.equal(composeNewDrawingNo('2000W.02.03d', ''), '')
  assert.equal(composeNewDrawingNo('2000W.02.03d', '   '), '')
  assert.equal(composeNewDrawingNo('2000W.02.03d', '-'), '')
  assert.equal(composeNewDrawingNo('2000W.02.03d', '2000W.02.03d'), '')
  assert.equal(composeNewDrawingNo('2000W.02.03d-', '2000w.02.03d'), '')
  // 没有项目号兜底时（图纸数据缺失），后几位本身就是完整图号。
  assert.equal(composeNewDrawingNo('', '01'), '01')
  assert.equal(composeNewDrawingNo('', ''), '')
})

test('defaultProjectNo：项目号默认取图号族根', () => {
  assert.equal(defaultProjectNo('2000W.02.03d'), '2000W.02.03d')
  assert.equal(defaultProjectNo('2000W.02.03d-01'), '2000W.02.03d')
  assert.equal(defaultProjectNo('  JG9063d-90-50-01  '), 'JG9063d')
  assert.equal(defaultProjectNo(''), '')
})

test('isBorrowedNumber：跨图号族才算借用件', () => {
  assert.equal(isBorrowedNumber('2000W.02.03d-01-01c', '2000W.02.03d'), false)
  assert.equal(isBorrowedNumber('2000W.02.03d', '2000W.02.03d'), false)
  assert.equal(isBorrowedNumber('JG9055e-5032-01', '2000W.02.03d'), true)
  assert.equal(isBorrowedNumber('', '2000W.02.03d'), false)
  assert.equal(isBorrowedNumber('2000W.02.03d-01', ''), false)
})

test('parentPartNoOf：末段是纯数字层级时才取上级图号', () => {
  // 与 parseDrawingNumber / directParentDrawingNo 保持同一语义：`01c` 是零件段而非层级，不算上级。
  assert.equal(parentPartNoOf('2000W.02.03d-01'), '2000W.02.03d')
  assert.equal(parentPartNoOf('2000W.02.03d-01-02'), '2000W.02.03d-01')
  assert.equal(parentPartNoOf('2000W.02.03d-01-01c'), null)
  assert.equal(parentPartNoOf('2000W.02.03d'), null)
  assert.equal(parentPartNoOf(''), null)
})

test('validateManualPartNo：拦住空值、空格与无意义输入', () => {
  assert.equal(validateManualPartNo('2000W.02.03d-01-01c'), '')
  assert.equal(validateManualPartNo('JG9055e-5032-01'), '')
  assert.match(validateManualPartNo(''), /请输入图号/)
  assert.match(validateManualPartNo('   '), /请输入图号/)
  assert.match(validateManualPartNo('2000W 02 03d'), /空格/)
  assert.match(validateManualPartNo('工程图'), /数字/)
  assert.match(validateManualPartNo('12345'), /格式不正确/)
})
