import { existsSync, readdirSync, readFileSync, statSync } from 'node:fs'
import { join, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = resolve(fileURLToPath(new URL('.', import.meta.url)), '..')
const sourceRoot = join(root, 'src')
const forbidden = [
  /DataManager/,
  /dataManager/,
  /Json(DataProvider|AuthProvider)/,
  /JSON Debug/,
  /readDebugMode|writeDebugMode|runtime-config/,
  /VITE_DATA_PROVIDER/,
  /useDomainStore|useCadDemoStore|useDemoStore/,
  /persistedSnapshots|moduleSnapshot|reloadFromServer/,
]
const forbiddenFiles = [
  'services/data-manager',
  'services/auth/json-auth.provider.ts',
  'services/runtime-config.service.ts',
]

function filesUnder(directory) {
  if (!existsSync(directory)) return []
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name)
    return entry.isDirectory() ? filesUnder(path) : [path]
  })
}

const violations = []
for (const file of filesUnder(sourceRoot)) {
  if (!/\.(ts|vue|json)$/.test(file)) continue
  const content = readFileSync(file, 'utf8')
  for (const pattern of forbidden) {
    if (pattern.test(content)) violations.push(`${relative(sourceRoot, file)} matches ${pattern}`)
  }
}
for (const path of forbiddenFiles) {
  const fullPath = join(sourceRoot, path)
  if (existsSync(fullPath) && (!statSync(fullPath).isDirectory() || readdirSync(fullPath).length > 0)) {
    violations.push(`obsolete path exists: ${path}`)
  }
}

const required = [
  'services/api/api-data-provider.ts',
  'types/application.types.ts',
  'modules/drawing/drawing-command-service.ts',
  'modules/upload/upload-gateway.ts',
]
for (const path of required) {
  const fullPath = join(sourceRoot, path)
  if (!existsSync(fullPath) || !statSync(fullPath).isFile()) violations.push(`required path missing: ${path}`)
}

const apiClient = readFileSync(join(sourceRoot, 'services/api/api-data-provider.ts'), 'utf8')
for (const pattern of [/saveStructure\s*\(/, /saveAttributes\s*\(/, /saveBom\s*\(/, /saveCrafts\s*\(/, /saveVersions\s*\(/]) {
  if (pattern.test(apiClient)) violations.push(`API client still exposes legacy bulk method ${pattern}`)
}

// 访问令牌只能从 services/auth/access-token.ts 读取：其他位置直接读历史镜像键
// 会在「内存已登录、存储镜像被清掉」时发出无鉴权请求，后端 401 再被业务层吞成空数据
// （表现为预览页空白、变更工单读不到）。会话记录键同样只允许该模块解析。
const tokenModule = 'services/auth/access-token.ts'
const tokenLiterals = [/cad_access_token/, /cad:auth-session/]
for (const file of filesUnder(sourceRoot)) {
  if (!/\.(ts|vue)$/.test(file)) continue
  const relativePath = relative(sourceRoot, file).split('\\').join('/')
  if (relativePath === tokenModule) continue
  const content = readFileSync(file, 'utf8')
  for (const pattern of tokenLiterals) {
    if (pattern.test(content)) {
      violations.push(`${relativePath} 直接读写访问令牌存储键，请改用 services/auth/access-token`)
    }
  }
}

if (violations.length) {
  console.error('前端架构回归失败')
  for (const violation of violations) console.error(`- ${violation}`)
  process.exitCode = 1
} else {
  console.log('前端架构回归通过：旧 DataManager/JSON Debug/旧 Store 入口均不存在')
}
