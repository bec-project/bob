//! bob's desktop app: a window around the chat GUI in gui/web. The page talks to OpenCode itself;
//! these commands do what a page cannot (start and hold bob, open links, save files).

use std::sync::Arc;

use tauri::{Emitter, Manager, Url};

mod bob;
mod settings;

/// Opens an http(s) link in the default browser.
#[tauri::command]
fn open_external(url: String) -> Result<(), String> {
    let parsed = Url::parse(&url).map_err(|error| error.to_string())?;
    if parsed.scheme() != "http" && parsed.scheme() != "https" {
        return Err("only http(s) links can be opened".into());
    }
    let opener = if cfg!(target_os = "macos") {
        "open"
    } else {
        "xdg-open"
    };
    std::process::Command::new(opener)
        .arg(parsed.as_str())
        .spawn()
        .map(|_| ())
        .map_err(|error| format!("cannot run {opener}: {error}"))
}

/// A free path for `name` in `dir`: the name itself, or the name with " (2)", " (3)", … added.
fn free_path(dir: &std::path::Path, name: &str) -> std::path::PathBuf {
    let path = std::path::Path::new(name);
    let stem = path.file_stem().and_then(|s| s.to_str()).unwrap_or("chat");
    let extension = path.extension().and_then(|s| s.to_str()).unwrap_or("md");
    let mut candidate = dir.join(format!("{stem}.{extension}"));
    let mut n = 2;
    while candidate.exists() {
        candidate = dir.join(format!("{stem} ({n}).{extension}"));
        n += 1;
    }
    candidate
}

/// Saves an exported chat in the user's Downloads folder and returns its path.
#[tauri::command]
fn save_download(name: String, text: String) -> Result<String, String> {
    // only a plain file name: no directories, nothing hidden
    let name = name.replace(['/', '\\'], "_");
    let name = name.trim_start_matches('.');
    if name.is_empty() {
        return Err("empty file name".into());
    }
    let home = std::env::var_os("HOME").ok_or("HOME is not set")?;
    let downloads = std::path::Path::new(&home).join("Downloads");
    let dir = if downloads.is_dir() {
        downloads
    } else {
        std::path::PathBuf::from(&home)
    };
    let path = free_path(&dir, name);
    std::fs::write(&path, text)
        .map_err(|error| format!("cannot write {}: {error}", path.display()))?;
    Ok(path.display().to_string())
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    // The lease on bob is an open connection: it ends with the app, however it exits.
    tauri::Builder::default()
        .plugin(tauri_plugin_dialog::init())
        .setup(|app| {
            let handle = app.handle().clone();
            let session = bob::BobSession::new(Box::new(move |status| {
                let _ = handle.emit("bob-status", status);
            }));
            let settings =
                settings::SettingsStore::open(app.path().app_config_dir()?.join("settings.json"));
            // Join (or start) bob before the page has loaded; it shows the status it finds.
            if settings.get().auto_join {
                if let (Some(binary), Some(config)) = settings.resolved() {
                    session.join_in_background(&binary, &config);
                }
            }
            app.manage(session);
            app.manage(settings);
            Ok(())
        })
        .invoke_handler(tauri::generate_handler![
            open_external,
            save_download,
            bob_settings,
            set_bob_settings,
            list_bobs,
            bob_status,
            bob_join,
            bob_leave
        ])
        .run(tauri::generate_context!())
        .expect("error while running the bob desktop app");
}

/// How the app starts bob, with the defaults for what is unset.
#[tauri::command]
fn bob_settings(app: tauri::AppHandle) -> settings::SettingsView {
    app.state::<settings::SettingsStore>().view()
}

/// Saves how the app starts bob; it applies from the next join.
#[tauri::command]
fn set_bob_settings(
    app: tauri::AppHandle,
    settings: settings::Settings,
) -> Result<settings::SettingsView, String> {
    app.state::<settings::SettingsStore>().set(settings)
}

/// bob containers running on Docker or Podman, with their connections.
#[tauri::command]
async fn list_bobs() -> Result<Vec<bob::RunningBob>, String> {
    tauri::async_runtime::spawn_blocking(bob::list)
        .await
        .map_err(|error| error.to_string())
}

fn session(app: &tauri::AppHandle) -> Arc<bob::BobSession> {
    Arc::clone(&app.state::<Arc<bob::BobSession>>())
}

/// Where the app stands with bob; every change also arrives as a `bob-status` event.
#[tauri::command]
fn bob_status(app: tauri::AppHandle) -> bob::BobStatus {
    session(&app).status()
}

/// Joins the user's bob, starting it with the chosen bob.toml when none runs. Returns the status
/// once bob is up or the join failed; the app holds one lease at most.
#[tauri::command]
async fn bob_join(app: tauri::AppHandle) -> Result<bob::BobStatus, String> {
    let session = session(&app);
    let (binary, config) = app.state::<settings::SettingsStore>().resolved();
    let Some(config) = config else {
        return Ok(bob::BobStatus::Idle {
            error: Some("Choose bob's configuration file (bob.toml) first.".into()),
        });
    };
    let binary = binary.unwrap_or_default();
    tauri::async_runtime::spawn_blocking(move || session.join(&binary, &config))
        .await
        .map_err(|error| error.to_string())
}

/// Ends the app's lease on bob, or a join in progress; bob stops once no client holds it.
#[tauri::command]
fn bob_leave(app: tauri::AppHandle) -> bob::BobStatus {
    session(&app).leave()
}

#[cfg(test)]
mod tests {
    #[test]
    fn numbers_a_download_that_exists() {
        let dir = std::env::temp_dir().join(format!("bob-gui-test-{}", std::process::id()));
        std::fs::create_dir_all(&dir).unwrap();
        let first = super::free_path(&dir, "chat.md");
        assert_eq!(first, dir.join("chat.md"));
        std::fs::write(&first, "x").unwrap();
        assert_eq!(super::free_path(&dir, "chat.md"), dir.join("chat (2).md"));
        std::fs::remove_dir_all(&dir).unwrap();
    }
}
