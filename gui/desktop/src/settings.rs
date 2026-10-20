//! The desktop app's own settings: how to start bob. Rust reads them when the app opens, before
//! the page loads, so it can join bob by itself. Look and layout stay in the page.

use std::path::{Path, PathBuf};
use std::sync::Mutex;

use serde::{Deserialize, Serialize};

use crate::bob;

/// What the user chose; unset paths fall back to bob's defaults.
#[derive(Serialize, Deserialize, Clone, Debug, PartialEq)]
#[serde(rename_all = "camelCase", default)]
pub struct Settings {
    /// The bob.toml to start bob with.
    pub config: Option<String>,
    /// The bob executable.
    pub binary: Option<String>,
    /// Join (or start) bob when the app opens.
    pub auto_join: bool,
}

impl Default for Settings {
    fn default() -> Self {
        Settings {
            config: None,
            binary: None,
            auto_join: true,
        }
    }
}

impl Settings {
    /// An empty path counts as unset.
    fn normalized(mut self) -> Self {
        self.config = self.config.filter(|c| !c.trim().is_empty());
        self.binary = self.binary.filter(|b| !b.trim().is_empty());
        self
    }
}

/// The settings with the defaults the page shows for unset paths.
#[derive(Serialize, Clone, Debug, PartialEq)]
#[serde(rename_all = "camelCase")]
pub struct SettingsView {
    #[serde(flatten)]
    pub settings: Settings,
    pub default_config: Option<String>,
    pub default_binary: Option<String>,
}

/// The settings file and its current content.
pub struct SettingsStore {
    path: PathBuf,
    current: Mutex<Settings>,
}

impl SettingsStore {
    /// Reads the settings file; a missing or unreadable one gives the defaults.
    pub fn open(path: PathBuf) -> Self {
        let current = match std::fs::read_to_string(&path) {
            Ok(text) => serde_json::from_str::<Settings>(&text)
                .map(Settings::normalized)
                .unwrap_or_else(|error| {
                    eprintln!("ignoring {}: {error}", path.display());
                    Settings::default()
                }),
            Err(_) => Settings::default(),
        };
        SettingsStore {
            path,
            current: Mutex::new(current),
        }
    }

    pub fn get(&self) -> Settings {
        self.current.lock().unwrap().clone()
    }

    pub fn view(&self) -> SettingsView {
        SettingsView {
            settings: self.get(),
            default_config: bob::default_config(),
            default_binary: bob::default_binary(),
        }
    }

    /// Saves new settings; they apply from the next join.
    pub fn set(&self, settings: Settings) -> Result<SettingsView, String> {
        let settings = settings.normalized();
        write_atomically(&self.path, &settings)?;
        *self.current.lock().unwrap() = settings;
        Ok(self.view())
    }

    /// The executable and bob.toml to join with: the chosen ones, else bob's defaults.
    pub fn resolved(&self) -> (Option<String>, Option<String>) {
        let settings = self.get();
        (
            settings.binary.or_else(bob::default_binary),
            settings.config.or_else(bob::default_config),
        )
    }
}

fn write_atomically(path: &Path, settings: &Settings) -> Result<(), String> {
    let fail = |error: std::io::Error| format!("cannot save {}: {error}", path.display());
    if let Some(dir) = path.parent() {
        std::fs::create_dir_all(dir).map_err(fail)?;
    }
    let text = serde_json::to_string_pretty(settings).expect("settings serialize");
    let temporary = path.with_extension("json.tmp");
    std::fs::write(&temporary, text + "\n").map_err(fail)?;
    std::fs::rename(&temporary, path).map_err(fail)
}

#[cfg(test)]
mod tests {
    use super::{Settings, SettingsStore};

    fn temp_file(name: &str) -> std::path::PathBuf {
        let dir =
            std::env::temp_dir().join(format!("bob-gui-settings-{name}-{}", std::process::id()));
        let _ = std::fs::remove_dir_all(&dir);
        dir.join("nested").join("settings.json")
    }

    #[test]
    fn starts_from_defaults_and_keeps_what_is_saved() {
        let path = temp_file("save");
        let store = SettingsStore::open(path.clone());
        assert_eq!(store.get(), Settings::default());
        assert!(store.get().auto_join);
        let chosen = Settings {
            config: Some("/w/bob.toml".into()),
            binary: Some(" ".into()),
            auto_join: false,
        };
        let view = store.set(chosen).unwrap();
        assert_eq!(view.settings.binary, None, "a blank path counts as unset");
        let reopened = SettingsStore::open(path.clone());
        assert_eq!(reopened.get().config.as_deref(), Some("/w/bob.toml"));
        assert!(!reopened.get().auto_join);
        assert_eq!(reopened.resolved().1.as_deref(), Some("/w/bob.toml"));
        std::fs::remove_dir_all(path.parent().unwrap().parent().unwrap()).unwrap();
    }

    #[test]
    fn falls_back_to_defaults_for_a_broken_file() {
        let path = temp_file("broken");
        std::fs::create_dir_all(path.parent().unwrap()).unwrap();
        std::fs::write(&path, "{not json").unwrap();
        assert_eq!(SettingsStore::open(path.clone()).get(), Settings::default());
        // fields the file leaves out take their defaults
        std::fs::write(&path, r#"{"config": "/w/bob.toml"}"#).unwrap();
        let settings = SettingsStore::open(path.clone()).get();
        assert_eq!(settings.config.as_deref(), Some("/w/bob.toml"));
        assert!(settings.auto_join);
        std::fs::remove_dir_all(path.parent().unwrap().parent().unwrap()).unwrap();
    }

    #[test]
    fn reaches_the_page_in_camel_case() {
        let view = super::SettingsView {
            settings: Settings::default(),
            default_config: Some("/repo/bob.toml".into()),
            default_binary: None,
        };
        assert_eq!(
            serde_json::to_value(view).unwrap(),
            serde_json::json!({"config": null, "binary": null, "autoJoin": true,
                "defaultConfig": "/repo/bob.toml", "defaultBinary": null})
        );
    }
}
