use std::{
  fs,
  io::Write,
  path::{Path, PathBuf},
};

use serde_json::Value;

fn config_file_path(app: &tauri::AppHandle) -> Result<PathBuf, String> {
  use tauri::Manager;

  let data_dir = app.path().app_data_dir().map_err(|error| error.to_string())?;
  fs::create_dir_all(&data_dir).map_err(|error| error.to_string())?;
  Ok(data_dir.join("config.toml"))
}

fn default_config() -> &'static str {
  "# 图枢运行配置\n# debug_mode = true 时使用本地 JSON 数据；false 时请求正式 API。\ndebug_mode = false\n"
}

/// 用户手动指定的本机 CAXA 程序路径（进程内共享，优先于自动扫描结果）。
static CAXA_OVERRIDE: std::sync::Mutex<Option<PathBuf>> = std::sync::Mutex::new(None);

fn set_caxa_override(path: PathBuf) {
  *CAXA_OVERRIDE.lock().unwrap_or_else(|poisoned| poisoned.into_inner()) = Some(path);
}

fn caxa_override() -> Option<PathBuf> {
  CAXA_OVERRIDE.lock().unwrap_or_else(|poisoned| poisoned.into_inner()).clone()
}

fn caxa_config_path(app: &tauri::AppHandle) -> Result<PathBuf, String> {
  use tauri::Manager;

  let data_dir = app.path().app_data_dir().map_err(|error| error.to_string())?;
  fs::create_dir_all(&data_dir).map_err(|error| error.to_string())?;
  Ok(data_dir.join("caxa-path.json"))
}

fn stored_caxa_path(app: &tauri::AppHandle) -> Result<String, String> {
  let path = caxa_config_path(app)?;
  if !path.exists() {
    return Ok(String::new());
  }
  let content = fs::read_to_string(&path).map_err(|error| format!("读取 caxa-path.json 失败：{}", error))?;
  let value: Value = serde_json::from_str(&content).map_err(|error| format!("解析 caxa-path.json 失败：{}", error))?;
  Ok(value.get("caxaPath").and_then(|item| item.as_str()).unwrap_or("").to_string())
}

#[tauri::command]
fn get_local_caxa_path(app: tauri::AppHandle) -> Result<String, String> {
  stored_caxa_path(&app)
}

#[tauri::command]
fn save_local_caxa_path(app: tauri::AppHandle, path: String) -> Result<(), String> {
  let trimmed = path.trim().trim_matches('"').to_string();
  if trimmed.is_empty() {
    return Err("请选择 CAXA 程序文件（如 CDRAFT_M.exe）".to_string());
  }
  let candidate = PathBuf::from(&trimmed);
  if !candidate.is_file() {
    return Err(format!("文件不存在：{}", trimmed));
  }
  let config = serde_json::json!({ "caxaPath": trimmed });
  let config_path = caxa_config_path(&app)?;
  fs::write(&config_path, format!("{}\n", serde_json::to_string_pretty(&config).map_err(|error| error.to_string())?))
    .map_err(|error| format!("保存 caxa-path.json 失败：{}", error))?;
  set_caxa_override(candidate);
  println!("[CAD] 已记录用户手动指定的 CAXA 路径：{}", trimmed);
  Ok(())
}

#[tauri::command]
fn pick_caxa_executable() -> Result<String, String> {
  #[cfg(windows)]
  {
    match native_open_file_dialog(
      "请选择 CAXA 程序（CDRAFT_M.exe）",
      "CAXA 程序 (CDRAFT_M.exe)\0CDRAFT_M.exe\0程序 (*.exe)\0*.exe\0所有文件 (*.*)\0*.*\0",
    ) {
      Some(path) => Ok(path),
      None => Ok(String::new()),
    }
  }
  #[cfg(not(windows))]
  {
    Err("文件选择功能目前仅支持 Windows".to_string())
  }
}

/// 弹出原生“打开文件”对话框，返回所选文件路径；用户取消时返回 None。
#[cfg(windows)]
fn native_open_file_dialog(title: &str, filter: &str) -> Option<String> {
  use std::os::windows::ffi::OsStrExt;
  use windows_sys::Win32::UI::Controls::Dialogs::{GetOpenFileNameW, OFN_FILEMUSTEXIST, OFN_PATHMUSTEXIST, OPENFILENAMEW};

  fn to_wide(value: &str) -> Vec<u16> {
    std::ffi::OsStr::new(value).encode_wide().chain(std::iter::once(0)).collect()
  }
  let title_wide = to_wide(title);
  // 文件名过滤器以 \0 分隔、双 \0 结尾；to_wide 已补一个 \0，字面量末尾自带一个。
  let filter_wide = to_wide(filter);
  let mut file_buffer = [0u16; 1024];
  let mut dialog: OPENFILENAMEW = unsafe { std::mem::zeroed() };
  dialog.lStructSize = std::mem::size_of::<OPENFILENAMEW>() as u32;
  dialog.lpstrFilter = filter_wide.as_ptr();
  dialog.lpstrFile = file_buffer.as_mut_ptr();
  dialog.nMaxFile = file_buffer.len() as u32;
  dialog.lpstrTitle = title_wide.as_ptr();
  dialog.Flags = OFN_FILEMUSTEXIST | OFN_PATHMUSTEXIST;
  if unsafe { GetOpenFileNameW(&mut dialog) } == 0 {
    return None;
  }
  let end = file_buffer.iter().position(|&character| character == 0).unwrap_or(0);
  if end == 0 {
    return None;
  }
  Some(String::from_utf16_lossy(&file_buffer[..end]))
}

/// 弹出原生“另存为文件”对话框，返回所选文件完整路径；用户取消时返回 None。
#[cfg(windows)]
fn native_save_file_dialog(title: &str, default_name: &str, filter: &str) -> Option<String> {
  use std::os::windows::ffi::OsStrExt;
  use windows_sys::Win32::UI::Controls::Dialogs::{GetSaveFileNameW, OFN_OVERWRITEPROMPT, OFN_PATHMUSTEXIST, OPENFILENAMEW};

  fn to_wide(value: &str) -> Vec<u16> {
    std::ffi::OsStr::new(value).encode_wide().chain(std::iter::once(0)).collect()
  }
  let title_wide = to_wide(title);
  let filter_wide = to_wide(filter);
  let mut file_buffer = [0u16; 1024];
  let default_wide = to_wide(default_name);
  let copy_len = default_wide.len().min(file_buffer.len() - 1);
  file_buffer[..copy_len].copy_from_slice(&default_wide[..copy_len]);

  let mut dialog: OPENFILENAMEW = unsafe { std::mem::zeroed() };
  dialog.lStructSize = std::mem::size_of::<OPENFILENAMEW>() as u32;
  dialog.lpstrFilter = filter_wide.as_ptr();
  dialog.lpstrFile = file_buffer.as_mut_ptr();
  dialog.nMaxFile = file_buffer.len() as u32;
  dialog.lpstrTitle = title_wide.as_ptr();
  dialog.Flags = OFN_OVERWRITEPROMPT | OFN_PATHMUSTEXIST;
  if unsafe { GetSaveFileNameW(&mut dialog) } == 0 {
    return None;
  }
  let end = file_buffer.iter().position(|&character| character == 0).unwrap_or(0);
  if end == 0 {
    return None;
  }
  Some(String::from_utf16_lossy(&file_buffer[..end]))
}

#[cfg(windows)]
fn save_filter_and_extension(default_name: &str) -> (&'static str, Option<&'static str>) {
  match Path::new(default_name).extension().and_then(|value| value.to_str()).map(|value| value.to_ascii_lowercase()).as_deref() {
    Some("zip") => ("压缩文件 (*.zip)\0*.zip\0所有文件 (*.*)\0*.*\0", Some("zip")),
    Some("xlsx") => ("Excel 工作簿 (*.xlsx)\0*.xlsx\0所有文件 (*.*)\0*.*\0", Some("xlsx")),
    Some("docx") => ("Word 文档 (*.docx)\0*.docx\0所有文件 (*.*)\0*.*\0", Some("docx")),
    Some("pdf") => ("PDF 文件 (*.pdf)\0*.pdf\0所有文件 (*.*)\0*.*\0", Some("pdf")),
    Some("dwg") => ("DWG 图纸 (*.dwg)\0*.dwg\0所有文件 (*.*)\0*.*\0", Some("dwg")),
    Some("dxf") => ("DXF 图纸 (*.dxf)\0*.dxf\0所有文件 (*.*)\0*.*\0", Some("dxf")),
    Some("exb") => ("EXB 图纸 (*.exb)\0*.exb\0所有文件 (*.*)\0*.*\0", Some("exb")),
    _ => ("所有文件 (*.*)\0*.*\0", None),
  }
}

#[tauri::command]
fn save_download_file(default_name: String, bytes: Vec<u8>) -> Result<Option<String>, String> {
  #[cfg(windows)]
  {
    let (filter, default_extension) = save_filter_and_extension(&default_name);
    match native_save_file_dialog("选择保存路径", &default_name, filter) {
      Some(chosen_path) => {
        let mut final_path = PathBuf::from(&chosen_path);
        if final_path.extension().is_none() {
          if let Some(extension) = default_extension {
            final_path.set_extension(extension);
          }
        }
        fs::write(&final_path, &bytes).map_err(|e| format!("保存文件失败: {}", e))?;
        Ok(Some(final_path.to_string_lossy().to_string()))
      }
      None => Ok(None),
    }
  }
  #[cfg(not(windows))]
  {
    Err("原生文件选择仅支持 Windows 客户端".to_string())
  }
}

#[tauri::command]
fn open_default_apps_settings() -> Result<(), String> {
  silent_command("cmd")
    .args(["/C", "start", "", "ms-settings:defaultapps"])
    .spawn()
    .map(|_| ())
    .map_err(|error| format!("打开系统设置失败：{}", error))
}

#[derive(serde::Serialize, serde::Deserialize)]
#[serde(rename_all = "camelCase")]
struct ApiConfig {
  api_base_url: String,
}

#[derive(serde::Serialize)]
#[serde(rename_all = "camelCase")]
struct ApiConfigResult {
  api_base_url: String,
  path: String,
}

#[tauri::command]
fn ensure_api_config(app: tauri::AppHandle, default_api_base_url: String) -> Result<ApiConfigResult, String> {
  use tauri::Manager;

  let data_dir = app.path().app_data_dir().map_err(|error| error.to_string())?;
  fs::create_dir_all(&data_dir).map_err(|error| error.to_string())?;
  let path = data_dir.join("api-config.json");
  if !path.exists() {
    let config = ApiConfig { api_base_url: default_api_base_url };
    let content = serde_json::to_string_pretty(&config).map_err(|error| error.to_string())?;
    fs::write(&path, format!("{}\n", content)).map_err(|error| error.to_string())?;
  }

  let content = fs::read_to_string(&path).map_err(|error| format!("读取 api-config.json 失败：{}", error))?;
  let config = serde_json::from_str::<ApiConfig>(&content)
    .map_err(|error| format!("解析 api-config.json 失败：{}", error))?;
  Ok(ApiConfigResult {
    api_base_url: config.api_base_url,
    path: path.to_string_lossy().into_owned(),
  })
}

#[derive(serde::Deserialize)]
struct EditTicketResponse {
  data: EditTicketData,
}

#[derive(serde::Deserialize)]
#[serde(rename_all = "camelCase")]
struct EditTicketData {
  #[serde(default)]
  smb_root: String,
  unc_path: String,
  #[serde(default)]
  caxa_path: String,
  smb_username: String,
  smb_password: String,
}

fn ticket_from_url(open_url: &str) -> Result<String, String> {
  let query = open_url
    .split_once('?')
    .map(|(_, query)| query)
    .ok_or_else(|| "CAD 打开链接缺少票据".to_string())?;
  query
    .split('&')
    .find_map(|part| part.strip_prefix("ticket=").map(ToString::to_string))
    .filter(|ticket| !ticket.is_empty())
    .ok_or_else(|| "CAD 打开链接缺少有效票据".to_string())
}

async fn exchange_edit_ticket(api_base_url: &str, access_token: &str, open_url: &str) -> Result<EditTicketData, String> {
  let ticket = ticket_from_url(open_url)?;
  let endpoint = format!("{}/edit-tickets/exchange", api_base_url.trim_end_matches('/'));
  let response = reqwest::Client::new()
    .post(endpoint)
    .bearer_auth(access_token)
    .json(&serde_json::json!({ "ticket": ticket }))
    .send()
    .await
    .map_err(|error| format!("交换 CAD 打开票据失败：{}", error))?;
  if !response.status().is_success() {
    return Err(format!("交换 CAD 打开票据失败：HTTP {}", response.status()));
  }
  response
    .json::<EditTicketResponse>()
    .await
    .map(|body| body.data)
    .map_err(|error| format!("解析 CAD 打开票据失败：{}", error))
}

#[cfg(windows)]
fn connect_smb_and_open_file(data: &EditTicketData) -> Result<(), String> {
  use std::os::windows::ffi::OsStrExt;
  use windows_sys::Win32::NetworkManagement::WNet::{WNetAddConnection2W, WNetCancelConnection2W, NETRESOURCEW, RESOURCETYPE_DISK};

  let smb_root = if data.smb_root.is_empty() {
    data.unc_path.rsplit_once('\\').map(|(root, _)| root.to_string()).unwrap_or_else(|| data.unc_path.clone())
  } else {
    data.smb_root.clone()
  };
  let remote_path: Vec<u16> = std::ffi::OsStr::new(&smb_root).encode_wide().chain(std::iter::once(0)).collect();
  let username: Vec<u16> = std::ffi::OsStr::new(&data.smb_username).encode_wide().chain(std::iter::once(0)).collect();
  let password: Vec<u16> = std::ffi::OsStr::new(&data.smb_password).encode_wide().chain(std::iter::once(0)).collect();
  let resource = NETRESOURCEW {
    dwScope: 0,
    dwType: RESOURCETYPE_DISK,
    dwDisplayType: 0,
    dwUsage: 0,
    lpLocalName: std::ptr::null_mut(),
    lpRemoteName: remote_path.as_ptr() as *mut u16,
    lpComment: std::ptr::null_mut(),
    lpProvider: std::ptr::null_mut(),
  };
  let username_ptr = if data.smb_username.trim().is_empty() { std::ptr::null() } else { username.as_ptr() };
  let password_ptr = if data.smb_password.is_empty() { std::ptr::null() } else { password.as_ptr() };
  let mut result = unsafe { WNetAddConnection2W(&resource, password_ptr, username_ptr, 0) };
  if result == 1219 {
    // 同一服务器已存在其他凭据的连接（Windows 不允许多凭据并存）：
    // 强制断开旧的无盘符连接后，用本次票据凭据重连。
    eprintln!("[CAD] 检测到旧凭据连接（错误码 1219），断开后重连");
    unsafe { WNetCancelConnection2W(remote_path.as_ptr(), 0, 1) };
    result = unsafe { WNetAddConnection2W(&resource, password_ptr, username_ptr, 0) };
  }
  if result != 0 && result != 85 {
    return Err(format!("建立 SMB 连接失败：Windows 错误码 {}", result));
  }
  let configured_caxa = (!data.caxa_path.trim().is_empty()).then(|| PathBuf::from(&data.caxa_path));
  open_cad_file(configured_caxa.as_deref(), Path::new(&data.unc_path), false)
}

#[cfg(not(windows))]
fn connect_smb_and_open_file(_data: &EditTicketData) -> Result<(), String> {
  Err("本地 CAD SMB 打开功能目前仅支持 Windows".to_string())
}

#[tauri::command]
async fn open_cad_edit_session(api_base_url: String, access_token: String, open_url: String) -> Result<(), String> {
  if access_token.trim().is_empty() {
    return Err("当前登录会话无效，请重新登录".to_string());
  }
  let data = exchange_edit_ticket(&api_base_url, &access_token, &open_url).await?;
  connect_smb_and_open_file(&data)
}

#[derive(serde::Deserialize)]
struct ReadOnlyOpenResponse {
  data: ReadOnlyOpenData,
}

#[derive(serde::Deserialize)]
#[serde(rename_all = "camelCase")]
struct ReadOnlyOpenData {
  download_path: String,
  file_name: String,
  #[serde(default)]
  caxa_path: String,
}

/// Windows 后台辅助命令一律隐藏控制台窗口：否则每秒一次的 attrib 等调用会不停闪黑窗，
/// 还会抢走系统前台焦点（例如打断用户在资源管理器地址栏的输入）。
#[cfg(windows)]
const CREATE_NO_WINDOW: u32 = 0x0800_0000;

#[cfg(windows)]
fn silent_command(program: &str) -> std::process::Command {
  use std::os::windows::process::CommandExt;
  let mut command = std::process::Command::new(program);
  command.creation_flags(CREATE_NO_WINDOW);
  command
}

/// 只读副本生命周期：等待 CAXA 打开文件 → 恢复只读提示属性 → CAXA 退出后销毁整个临时目录。
#[cfg(windows)]
fn cleanup_readonly_dir(temp_dir: PathBuf, file_path: PathBuf) {
  // 1. 等待 CAXA 打开工作文件（最多 120 秒）：文件被写占用即视为已成功打开。
  let mut opened = false;
  for _ in 0..240 {
    if file_write_locked(&file_path) {
      opened = true;
      break;
    }
    std::thread::sleep(std::time::Duration::from_millis(500));
  }
  if !opened {
    // CAXA 始终未打开（启动失败/用户取消），直接销毁副本，不留本地残留。
    let _ = silent_command("attrib")
      .args(["-R", "/S", "/D", &format!("{}\\*.*", temp_dir.display())])
      .status();
    if fs::remove_dir_all(&temp_dir).is_err() {
      eprintln!("[CAD] 临时目录清理失败（可能仍被占用）：{}", temp_dir.display());
    }
    return;
  }
  // 2. 恢复只读属性，提示（不阻止）CAD 覆盖保存；对已打开的句柄无影响。
  let _ = silent_command("attrib")
    .args(["+R", file_path.to_str().unwrap_or_default()])
    .status();
  // 3. CAXA 退出（文件解锁）后销毁整个目录；只读属性已清除，删除失败时静默重试最多 120 秒。
  for _ in 0..120 {
    std::thread::sleep(std::time::Duration::from_secs(1));
    if fs::remove_dir_all(&temp_dir).is_ok() {
      return;
    }
  }
  eprintln!("[CAD] 临时目录清理失败（可能仍被占用）：{}", temp_dir.display());
}

/// 尝试以可写方式打开文件：被占用（如 CAXA 独占打开）时返回 true。
#[cfg(windows)]
fn file_write_locked(path: &Path) -> bool {
  if !path.is_file() {
    return false;
  }
  std::fs::OpenOptions::new().append(true).open(path).is_err()
}

#[cfg(windows)]
fn open_cad_file(caxa_path: Option<&Path>, file_path: &Path, wait: bool) -> Result<(), String> {
  // 服务端路径只作为候选，与本机安装一起按年份排序，不能抢占新版。
  let mut candidates = find_local_caxa();
  if let Some(path) = caxa_path.filter(|path| path.is_file()) {
    candidates.push(path.to_path_buf());
  }
  prioritize_caxa(&mut candidates, caxa_override().filter(|path| path.is_file()));
  for candidate in &candidates {
    println!("[CAD] 使用本机 CAXA 打开文件：{} -> {}", candidate.display(), file_path.display());
    let mut command = std::process::Command::new(candidate);
    // 本地编辑实例不得消费服务端转图任务或自动取消用户的字体/恢复弹窗。
    command.env("CADGUANLIQ_CAXA_INTERACTIVE", "1");
    if let Some(parent) = candidate.parent() {
      command.current_dir(parent);
    }
    let result = if wait {
      command.arg(file_path).status().map(|_| ())
    } else {
      command.arg(file_path).spawn().map(|_| ())
    };
    match result {
      Ok(()) => return Ok(()),
      Err(error) => eprintln!("[CAD] 直接启动失败，尝试下一方式：{}（{}）", candidate.display(), error),
    }
  }

  // 与资源管理器地址栏回车完全等价：交给系统文件关联，无 cmd 解析风险。
  println!("[CAD] 使用 Windows 文件关联打开图纸：{}", file_path.display());
  open_with_shell_execute(file_path, wait)
}

#[cfg(windows)]
fn open_with_shell_execute(file_path: &Path, wait: bool) -> Result<(), String> {
  use std::os::windows::ffi::OsStrExt;
  use windows_sys::Win32::Foundation::CloseHandle;
  use windows_sys::Win32::System::Threading::{WaitForSingleObject, INFINITE};
  use windows_sys::Win32::UI::Shell::{ShellExecuteExW, SEE_MASK_FLAG_NO_UI, SEE_MASK_NOCLOSEPROCESS, SHELLEXECUTEINFOW};

  fn to_wide(value: &str) -> Vec<u16> {
    std::ffi::OsStr::new(value).encode_wide().chain(std::iter::once(0)).collect()
  }

  let file = to_wide(&file_path.to_string_lossy());
  let verb = to_wide("open");
  let mut exec_info: SHELLEXECUTEINFOW = unsafe { std::mem::zeroed() };
  exec_info.cbSize = std::mem::size_of::<SHELLEXECUTEINFOW>() as u32;
  exec_info.fMask = if wait { SEE_MASK_NOCLOSEPROCESS | SEE_MASK_FLAG_NO_UI } else { SEE_MASK_FLAG_NO_UI };
  exec_info.lpVerb = verb.as_ptr();
  exec_info.lpFile = file.as_ptr();
  exec_info.nShow = 1; // SW_SHOWNORMAL
  let started = unsafe { ShellExecuteExW(&mut exec_info) };
  if started == 0 {
    // CAXA_NOT_FOUND 前缀：前端据此弹出“未找到本机 CAD”操作弹窗，而不是一闪而过的提示。
    return Err(format!(
      "CAXA_NOT_FOUND:本机未找到 CAXA 程序，且系统未配置图纸文件关联（文件：{}）。可在弹窗中选择 CAXA 程序或配置默认打开方式",
      file_path.display()
    ));
  }
  if wait && !exec_info.hProcess.is_null() {
    // DDE/代理启动时句柄可能为空，此时由只读清理线程的重试逻辑兜底。
    unsafe {
      WaitForSingleObject(exec_info.hProcess, INFINITE);
      CloseHandle(exec_info.hProcess);
    }
  }
  Ok(())
}

/// 缓存所有安装，保留启动失败时尝试其他安装的机会；无结果或缓存失效时重新扫描。
#[cfg(windows)]
fn find_local_caxa() -> Vec<PathBuf> {
  static CACHE: std::sync::Mutex<Vec<PathBuf>> = std::sync::Mutex::new(Vec::new());
  let mut cached = CACHE.lock().unwrap_or_else(|poisoned| poisoned.into_inner());
  if cached.is_empty() || cached.iter().any(|path| !path.is_file()) {
    *cached = scan_local_caxa();
  }
  cached.clone()
}

/// 本机扫描 CAXA 安装（注册表 Uninstall 键 + 常见目录），按版本号新→旧排序。
#[cfg(windows)]
fn scan_local_caxa() -> Vec<PathBuf> {
  let mut matches: Vec<PathBuf> = Vec::new();
  for directory in caxa_registry_directories() {
    matches.extend(glob_caxa_bin(&directory));
  }

  let mut roots: Vec<PathBuf> = Vec::new();
  for variable in ["ProgramFiles", "ProgramW6432", "ProgramFiles(x86)"] {
    if let Ok(value) = std::env::var(variable) {
      roots.push(PathBuf::from(value));
    }
  }
  for drive in ["C", "D", "E", "F"] {
    roots.push(PathBuf::from(format!("{drive}:\\")));
  }
  for root in roots {
    let caxa_root = root.join("CAXA");
    if caxa_root.is_dir() {
      matches.extend(glob_caxa_bin(&caxa_root));
    }
  }

  prioritize_caxa(&mut matches, None);
  matches
}

/// 递归查找安装目录下 Bin64\CDRAFT_M.exe 或 Bin\CDRAFT_M.exe（最多深入 3 层）。
#[cfg(windows)]
fn glob_caxa_bin(install_dir: &Path) -> Vec<PathBuf> {
  fn walk(dir: &Path, depth: usize, found: &mut Vec<PathBuf>) {
    let executable = dir.join("CDRAFT_M.exe");
    if executable.is_file() {
      found.push(executable);
    }
    for bin in ["Bin64", "Bin"] {
      let candidate = dir.join(bin).join("CDRAFT_M.exe");
      if candidate.is_file() {
        found.push(candidate);
      }
    }
    if depth == 0 {
      return;
    }
    if let Ok(entries) = fs::read_dir(dir) {
      for entry in entries.flatten() {
        let path = entry.path();
        if entry.file_type().map(|kind| kind.is_dir()).unwrap_or(false) {
          walk(&path, depth - 1, found);
        }
      }
    }
  }
  let mut found = Vec::new();
  walk(install_dir, 3, &mut found);
  found
}

/// 直接读取 Unicode 注册表，避免 reg.exe 的本地编码损坏中文安装路径。
#[cfg(windows)]
fn caxa_registry_directories() -> Vec<PathBuf> {
  use windows_sys::Win32::System::Registry::*;
  fn wide(text: &str) -> Vec<u16> { text.encode_utf16().chain(Some(0)).collect() }
  fn read_string(key: HKEY, subkey: &[u16], name: &str) -> String {
    let name = wide(name);
    let mut buffer = vec![0u16; 32768];
    let mut size = (buffer.len() * 2) as u32;
    let result = unsafe { RegGetValueW(key, subkey.as_ptr(), name.as_ptr(),
      RRF_RT_REG_SZ | RRF_RT_REG_EXPAND_SZ, std::ptr::null_mut(),
      buffer.as_mut_ptr().cast(), &mut size) };
    if result != 0 { return String::new(); }
    let length = buffer.iter().position(|value| *value == 0).unwrap_or(buffer.len());
    String::from_utf16_lossy(&buffer[..length])
  }
  let mut directories = Vec::new();
  for (hive, location) in [
    (HKEY_LOCAL_MACHINE, r"SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall"),
    (HKEY_LOCAL_MACHINE, r"SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall"),
    (HKEY_CURRENT_USER, r"Software\Microsoft\Windows\CurrentVersion\Uninstall"),
  ] {
    let mut key = std::ptr::null_mut();
    if unsafe { RegOpenKeyExW(hive, wide(location).as_ptr(), 0, KEY_READ, &mut key) } != 0 {
      continue;
    }
    let mut index = 0;
    loop {
      let mut name = [0u16; 256];
      let mut length = name.len() as u32;
      let result = unsafe { RegEnumKeyExW(key, index, name.as_mut_ptr(), &mut length,
        std::ptr::null(), std::ptr::null_mut(), std::ptr::null_mut(), std::ptr::null_mut()) };
      if result != 0 { break; }
      index += 1;
      let display_name = read_string(key, &name, "DisplayName");
      let install = read_string(key, &name, "InstallLocation");
      let subkey = String::from_utf16_lossy(&name[..length as usize]);
      if !install.trim().is_empty() && [display_name.as_str(), install.as_str(), subkey.as_str()]
        .iter().any(|value| value.to_ascii_lowercase().contains("caxa")) {
        directories.push(PathBuf::from(install.trim().trim_matches('"')));
      }
    }
    unsafe { RegCloseKey(key); }
  }
  directories
}

/// 只从 CAXA 安装目录起识别四位年份，避免 Program Files (x86)、Bin64 等干扰排序。
#[cfg(windows)]
fn caxa_version_year(path: &Path) -> u16 {
  let text = path.to_string_lossy().to_ascii_lowercase();
  let installation = text.find("caxa").map(|index| &text[index..]).unwrap_or(&text);
  installation.split(|character: char| !character.is_ascii_digit())
    .filter(|part| part.len() == 4)
    .filter_map(|part| part.parse::<u16>().ok())
    .filter(|year| (2000..2100).contains(year))
    .max().unwrap_or(0)
}

#[cfg(windows)]
fn prioritize_caxa(candidates: &mut Vec<PathBuf>, manual: Option<PathBuf>) {
  candidates.sort_by_key(|path| (
    std::cmp::Reverse(caxa_version_year(path)),
    path.to_string_lossy().to_ascii_lowercase(),
  ));
  candidates.dedup_by(|a, b| a.to_string_lossy().eq_ignore_ascii_case(&b.to_string_lossy()));
  // 用户明确指定的程序仍然优先于自动选择。
  if let Some(path) = manual {
    candidates.retain(|item| !item.to_string_lossy().eq_ignore_ascii_case(&path.to_string_lossy()));
    candidates.insert(0, path);
  }
}

#[cfg(all(test, windows))]
mod caxa_selection_tests {
  use super::*;

  #[test]
  fn newer_installations_win_over_server_hint_and_architecture_digits() {
    let old = PathBuf::from(r"C:\Program Files (x86)\CAXA\2015\Bin\CDRAFT_M.exe");
    let current = PathBuf::from(r"D:\CAXA\电子图板2022\Bin64\CDRAFT_M.exe");
    let newest = PathBuf::from(r"C:\Program Files\CAXA\2025\Bin64\CDRAFT_M.exe");
    let mut candidates = vec![old.clone(), current.clone(), newest.clone(), old.clone()];
    prioritize_caxa(&mut candidates, None);
    assert_eq!(candidates, vec![newest, current, old]);
  }

  #[test]
  fn explicit_user_choice_is_preserved_and_deduplicated() {
    let manual = PathBuf::from(r"D:\CAXA\2022\Bin64\CDRAFT_M.exe");
    let latest = PathBuf::from(r"D:\CAXA\2025\Bin64\CDRAFT_M.exe");
    let mut candidates = vec![latest.clone(), PathBuf::from(r"d:\caxa\2022\bin64\cdraft_m.exe")];
    prioritize_caxa(&mut candidates, Some(manual.clone()));
    assert_eq!(candidates, vec![manual, latest]);
  }

  #[test]
  fn version_ignores_unrelated_numbers_and_unknown_installations() {
    assert_eq!(caxa_version_year(Path::new(r"D:\backup2026\CAXA\2022\Bin64\CDRAFT_M.exe")), 2022);
    assert_eq!(caxa_version_year(Path::new(r"C:\Program Files (x86)\CAXA\Bin64\CDRAFT_M.exe")), 0);
  }
}

#[tauri::command]
async fn open_cad_readonly(api_base_url: String, access_token: String, storage_key: String) -> Result<(), String> {
  if access_token.trim().is_empty() {
    return Err("当前登录会话无效，请重新登录".to_string());
  }
  let base = api_base_url.trim_end_matches('/').to_string();
  let endpoint = format!("{}/editing/read-only", base);
  let response = reqwest::Client::new()
    .post(endpoint)
    .bearer_auth(&access_token)
    .json(&serde_json::json!({ "storageKey": storage_key }))
    .send()
    .await
    .map_err(|error| format!("请求只读打开失败：{}", error))?;
  if !response.status().is_success() {
    return Err(format!("请求只读打开失败：HTTP {}", response.status()));
  }
  let data = response
    .json::<ReadOnlyOpenResponse>()
    .await
    .map_err(|error| format!("解析只读打开信息失败：{}", error))?
    .data;
  let file_url = format!("{}{}", base, data.download_path);
  let file_response = reqwest::Client::new()
    .get(file_url)
    .bearer_auth(&access_token)
    .send()
    .await
    .map_err(|error| format!("下载只读副本失败：{}", error))?;
  if !file_response.status().is_success() {
    return Err(format!("下载只读副本失败：HTTP {}", file_response.status()));
  }
  let bytes = file_response
    .bytes()
    .await
    .map_err(|error| format!("读取只读副本失败：{}", error))?;
  let unique = std::time::SystemTime::now()
    .duration_since(std::time::UNIX_EPOCH)
    .map(|value| value.as_nanos())
    .unwrap_or(0);
  let temp_dir = std::env::temp_dir().join(format!("cadguanliq_readonly_{}", unique));
  fs::create_dir_all(&temp_dir).map_err(|error| format!("创建临时目录失败：{}", error))?;
  let file_path = temp_dir.join(&data.file_name);
  {
    let mut file = fs::File::create(&file_path).map_err(|error| format!("写入临时文件失败：{}", error))?;
    file.write_all(&bytes).map_err(|error| format!("写入临时文件失败：{}", error))?;
  }
  let caxa_path = (!data.caxa_path.trim().is_empty()).then(|| PathBuf::from(&data.caxa_path));
  // 同步打开：本机找不到 CAXA 时把错误返回给前端（弹操作弹窗），不再“提示成功却没打开”。
  let open_result = open_cad_file(caxa_path.as_deref(), &file_path, false);
  // 清理必须等 CAXA 真正打开文件之后才开始：启动中就删除会让 CAXA 打开已消失的文件（报格式错误）。
  std::thread::spawn(move || cleanup_readonly_dir(temp_dir, file_path));
  open_result
}

#[tauri::command]
fn ensure_smb_credential(host: String, username: String, password: String) -> Result<(), String> {
  #[cfg(windows)]
  {
    if host.trim().is_empty() || username.trim().is_empty() {
      return Err("服务器未配置 SMB 主机或访问账号".to_string());
    }
    let output = silent_command("cmdkey")
      .args([
        format!("/add:{}", host.trim()),
        format!("/user:{}", username.trim()),
        format!("/pass:{}", password),
      ])
      .output()
      .map_err(|error| format!("调用 cmdkey 失败：{}", error))?;
    if output.status.success() {
      Ok(())
    } else {
      Err(format!(
        "写入 Windows 凭据失败：{}",
        String::from_utf8_lossy(&output.stderr).trim()
      ))
    }
  }
  #[cfg(not(windows))]
  {
    let _ = (host, username, password);
    Err("SMB 凭据写入目前仅支持 Windows".to_string())
  }
}

fn ensure_config_file(app: &tauri::AppHandle) -> Result<PathBuf, String> {
  let path = config_file_path(app)?;
  if !path.exists() {
    fs::write(&path, default_config()).map_err(|error| error.to_string())?;
  }
  Ok(path)
}

fn attachments_dir(app: &tauri::AppHandle) -> Result<PathBuf, String> {
  use tauri::Manager;

  let directory = app.path().app_data_dir().map_err(|error| error.to_string())?.join("attachments");
  fs::create_dir_all(&directory).map_err(|error| error.to_string())?;
  Ok(directory)
}

fn attachment_path(app: &tauri::AppHandle, storage_key: &str) -> Result<PathBuf, String> {
  let normalized_key = storage_key.replace('\\', "/");
  let segments: Vec<&str> = normalized_key.split('/').collect();
  if normalized_key.is_empty() || segments.iter().any(|segment| {
    segment.is_empty() || *segment == "." || *segment == ".." || segment.contains(':')
  }) {
    return Err("附件存储键无效".to_string());
  }
  Ok(attachments_dir(app)?.join(segments.iter().collect::<PathBuf>()))
}

#[tauri::command]
fn write_attachment(app: tauri::AppHandle, storage_key: String, bytes: Vec<u8>) -> Result<(), String> {
  let path = attachment_path(&app, &storage_key)?;
  if let Some(parent) = path.parent() {
    fs::create_dir_all(parent).map_err(|error| error.to_string())?;
  }
  let temporary_path = path.with_extension("tmp");
  let mut temporary_file = fs::File::create(&temporary_path).map_err(|error| error.to_string())?;
  temporary_file.write_all(&bytes).map_err(|error| error.to_string())?;
  temporary_file.sync_all().map_err(|error| error.to_string())?;
  drop(temporary_file);
  fs::rename(&temporary_path, &path).map_err(|error| error.to_string())
}

#[tauri::command]
fn read_attachment(app: tauri::AppHandle, storage_key: String) -> Result<Vec<u8>, String> {
  let path = attachment_path(&app, &storage_key)?;
  fs::read(path).map_err(|error| error.to_string())
}

#[tauri::command]
fn delete_attachment(app: tauri::AppHandle, storage_key: String) -> Result<(), String> {
  let path = attachment_path(&app, &storage_key)?;
  if path.exists() {
    fs::remove_file(path).map_err(|error| error.to_string())?;
  }
  Ok(())
}

#[tauri::command]
fn open_generated_excel(app: tauri::AppHandle, file_name: String, bytes: Vec<u8>) -> Result<(), String> {
  use tauri::Manager;

  let safe_name = file_name
    .chars()
    .map(|character| if ['\\', '/', ':', '*', '?', '"', '<', '>', '|'].contains(&character) { '_' } else { character })
    .collect::<String>();
  let file_name = if safe_name.to_lowercase().ends_with(".xlsx") {
    safe_name
  } else {
    format!("{}.xlsx", safe_name)
  };
  let downloads_dir = app.path().download_dir().map_err(|error| error.to_string())?;
  fs::create_dir_all(&downloads_dir).map_err(|error| error.to_string())?;
  let path = downloads_dir.join(file_name);
  fs::write(&path, bytes).map_err(|error| error.to_string())?;
  silent_command("cmd")
    .args(["/C", "start", "", &path.to_string_lossy()])
    .spawn()
    .map_err(|error| error.to_string())?;
  Ok(())
}

#[tauri::command]
fn read_debug_mode(app: tauri::AppHandle) -> Result<bool, String> {
  let path = ensure_config_file(&app)?;
  let content = fs::read_to_string(path).map_err(|error| error.to_string())?;
  Ok(content.lines().any(|line| {
    let line = line.trim();
    line == "debug_mode = true" || line == "debug_mode=true"
  }))
}

#[tauri::command]
fn write_debug_mode(app: tauri::AppHandle, enabled: bool) -> Result<(), String> {
  let path = ensure_config_file(&app)?;
  let content = format!(
    "# 图枢运行配置\n# debug_mode = true 时使用本地 JSON 数据；false 时请求正式 API。\ndebug_mode = {}\n",
    enabled
  );
  fs::write(path, content).map_err(|error| error.to_string())
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
  tauri::Builder::default()
    .plugin(tauri_plugin_deep_link::init())
    .plugin(tauri_plugin_single_instance::init(|app, argv, _cwd| {
      use tauri::{Emitter, Manager};

      if let Some(window) = app.get_webview_window("main") {
        let _ = window.show();
        let _ = window.set_focus();
      }
      let urls: Vec<String> = argv.into_iter().filter(|argument| argument.starts_with("cadguanliq://")).collect();
      if !urls.is_empty() {
        let _ = app.emit("cad-deep-link", urls);
      }
    }))
    .invoke_handler(tauri::generate_handler![
      write_attachment,
      read_attachment,
      delete_attachment,
      open_generated_excel,
      open_cad_edit_session,
      open_cad_readonly,
      ensure_smb_credential,
      get_local_caxa_path,
      save_local_caxa_path,
      pick_caxa_executable,
      open_default_apps_settings,
      ensure_api_config,
      read_debug_mode,
      write_debug_mode,
      save_download_file
    ])
    .setup(|app| {
      use tauri::{
        menu::{Menu, MenuItem},
        tray::TrayIconBuilder,
        Manager,
      };

      use tauri_plugin_deep_link::DeepLinkExt;
      app.handle().deep_link().register_all()?;
      println!("[DeepLink] 已注册协议：cadguanliq://");

      let show_item = MenuItem::with_id(app, "show", "显示图枢", true, None::<&str>)?;
      let quit_item = MenuItem::with_id(app, "quit", "退出图枢", true, None::<&str>)?;
      let tray_menu = Menu::with_items(app, &[&show_item, &quit_item])?;
      let tray_icon = app
        .default_window_icon()
        .cloned()
        .expect("default window icon is not configured");

      TrayIconBuilder::new()
        .icon(tray_icon)
        .menu(&tray_menu)
        .tooltip("图枢 · CAD 图纸管理系统")
        .on_menu_event(|app, event| match event.id().as_ref() {
          "show" => {
            if let Some(window) = app.get_webview_window("main") {
              let _ = window.show();
              let _ = window.set_focus();
            }
          }
          "quit" => app.exit(0),
          _ => {}
        })
        .build(app)?;

      if cfg!(debug_assertions) {
        app.handle().plugin(
          tauri_plugin_log::Builder::default()
            .level(log::LevelFilter::Info)
            .build(),
        )?;
      }
      ensure_config_file(app.handle())?;
      // 启动时恢复用户手动指定的 CAXA 路径（保存于 caxa-path.json）。
      if let Ok(path) = stored_caxa_path(app.handle()) {
        if !path.trim().is_empty() {
          set_caxa_override(PathBuf::from(path));
        }
      }
      Ok(())
    })
    .run(tauri::generate_context!())
    .expect("error while running tauri application");
}
