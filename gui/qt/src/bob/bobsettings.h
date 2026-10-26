#pragma once

#include <QString>

// The app's own settings: how to start bob. They are read when the app opens, before any window
// shows, so the app can join bob by itself. Look and layout are Prefs.
struct BobSettings
{
    // The bob.toml to start bob with; empty means bob's default.
    QString config;
    // The bob executable; empty means bob's default.
    QString binary;
    // Join (or start) bob when the app opens.
    bool autoJoin = true;

    bool operator==(const BobSettings &other) const
    {
        return config == other.config && binary == other.binary && autoJoin == other.autoJoin;
    }
};

// settings.json and its current content. The file is the Tauri app's (same folder, same keys), so
// both apps start bob the same way.
class BobSettingsStore
{
public:
    // Reads the file; a missing or unreadable one gives the defaults, missing keys theirs.
    explicit BobSettingsStore(const QString &path);

    BobSettings get() const { return m_current; }
    // Saves new settings (blank paths count as unset); returns an error, or an empty string.
    QString set(BobSettings settings);
    // The executable and bob.toml to join with: the chosen ones, else bob's defaults.
    QString resolvedBinary() const;
    QString resolvedConfig() const;
    QString path() const { return m_path; }

private:
    QString m_path;
    BobSettings m_current;
};
