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
    .invoke_handler(tauri::generate_handler![
      read_data_document,
      write_data_document,
      write_attachment,
      read_attachment,
      delete_attachment,
      open_generated_excel,
      read_debug_mode,
      write_debug_mode
    ])
    .setup(|app| {
      use tauri::{
        menu::{Menu, MenuItem},
        tray::TrayIconBuilder,
        Manager,
      };

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
