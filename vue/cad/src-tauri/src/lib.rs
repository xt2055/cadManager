#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
  tauri::Builder::default()
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
      Ok(())
    })
    .run(tauri::generate_context!())
    .expect("error while running tauri application");
}
