#pragma once

#include <QJsonObject>
#include <QString>

#include <optional>

// The connection to bob's OpenCode server: {url, password, container, ...}.
namespace Connection {

// bob's connection object (as the broker sends it), or nothing when it lacks the URL or password.
std::optional<QJsonObject> fromJson(const QJsonValue &value);
// Reads what the user pasted: bob's connection JSON, or a bare URL (with the password given
// apart). Nothing when it is neither.
std::optional<QJsonObject> parse(const QString &text, const QString &password);

}  // namespace Connection
