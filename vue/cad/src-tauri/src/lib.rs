use std::{
  fs,
  io::Write,
  path::{Path, PathBuf},
};

use serde_json::Value;

fn data_file_path(app: &tauri::AppHandle) -> Result<PathBuf, String> {
  use tauri::Manager;

  let data_dir = app.path().app_data_dir().map_err(|error| error.to_string())?;
  fs::create_dir_all(&data_dir).map_err(|error| error.to_string())?;
  Ok(data_dir.join("data-document.json"))
}

fn config_file_path(app: &tauri::AppHandle) -> Result<PathBuf, String> {
  use tauri::Manager;

  let data_dir = app.path().app_data_dir().map_err(|error| error.to_string())?;
  fs::create_dir_all(&data_dir).map_err(|error| error.to_string())?;
  Ok(data_dir.join("config.toml"))
}

fn default_config() -> &'static str {
  "# 图枢运行配置\n# debug_mode = true 时使用本地 JSON 数据；false 时请求正式 API。\ndebug_mode = false\n"
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
  use windows_sys::Win32::NetworkManagement::WNet::{WNetAddConnection2W, NETRESOURCEW, RESOURCETYPE_DISK};

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
  let result = unsafe { WNetAddConnection2W(&resource, password_ptr, username_ptr, 0) };
  if result != 0 && result != 85 {
    return Err(format!("建立 SMB 连接失败：Windows 错误码 {}", result));
  }
  let caxa_path = if data.caxa_path.trim().is_empty() {
    return Err("后端未返回 CAXA 程序路径，请重新启动最新 Go 后端".to_string());
  } else {
    PathBuf::from(&data.caxa_path)
  };
  if !caxa_path.is_file() {
    return Err(format!("后端返回的 CAXA 程序不存在：{}", caxa_path.display()));
  }
  println!("[CAD] 使用 CAXA 打开文件：{} -> {}", caxa_path.display(), data.unc_path);
  let mut cmd = std::process::Command::new(&caxa_path);
  if let Some(parent) = Path::new(&data.unc_path).parent() {
    cmd.current_dir(parent);
  }
  cmd.arg(&data.unc_path)
    .spawn()
    .map_err(|error| format!("启动 CAXA 失败：{}", error))?;
  Ok(())
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

fn backup_file_path(path: &Path) -> PathBuf {
  path.with_extension("json.bak")
}

fn recover_data_file(path: &Path) -> Result<(), String> {
  let backup_path = backup_file_path(path);
  if !path.exists() && backup_path.exists() {
    fs::rename(&backup_path, path).map_err(|error| error.to_string())?;
  } else if path.exists() && backup_path.exists() {
    fs::remove_file(backup_path).map_err(|error| error.to_string())?;
  }
  Ok(())
}

#[tauri::command]
fn read_data_document(app: tauri::AppHandle) -> Result<Option<Value>, String> {
  let path = data_file_path(&app)?;
  recover_data_file(&path)?;
  if !path.exists() {
    return Ok(None);
  }

  let content = fs::read_to_string(path).map_err(|error| error.to_string())?;
  serde_json::from_str(&content).map(Some).map_err(|error| error.to_string())
}

#[tauri::command]
fn write_data_document(app: tauri::AppHandle, document: Value) -> Result<(), String> {
  let path = data_file_path(&app)?;
  let temporary_path = path.with_extension("json.tmp");
  let backup_path = backup_file_path(&path);
  let content = serde_json::to_string_pretty(&document).map_err(|error| error.to_string())?;

  recover_data_file(&path)?;
  let mut temporary_file = fs::File::create(&temporary_path).map_err(|error| error.to_string())?;
  temporary_file
    .write_all(content.as_bytes())
    .map_err(|error| error.to_string())?;
  temporary_file.sync_all().map_err(|error| error.to_string())?;
  drop(temporary_file);

  if path.exists() {
    fs::rename(&path, &backup_path).map_err(|error| error.to_string())?;
  }

  if let Err(error) = fs::rename(&temporary_path, &path) {
    let restore_result = if !path.exists() && backup_path.exists() {
      fs::rename(&backup_path, &path)
    } else {
      Ok(())
    };

    return match restore_result {
      Ok(()) => Err(error.to_string()),
      Err(restore_error) => Err(format!(
        "替换业务数据文件失败：{}；恢复旧文件失败：{}",
        error, restore_error
      )),
    };
  }

  if backup_path.exists() {
    fs::remove_file(backup_path).map_err(|error| error.to_string())?;
  }

  Ok(())
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
  std::process::Command::new("cmd")
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
      read_data_document,
      write_data_document,
      write_attachment,
      read_attachment,
      delete_attachment,
      open_generated_excel,
      open_cad_edit_session,
      read_debug_mode,
      write_debug_mode
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
      Ok(())
    })
    .run(tauri::generate_context!())
    .expect("error while running tauri application");
}
