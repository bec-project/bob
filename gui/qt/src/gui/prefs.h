#pragma once

#include <QObject>
#include <QQmlEngine>
#include <QSettings>
#include <QVariantMap>

// Per-user choices about the window, kept with QSettings. Nothing secret goes here; how bob
// starts is the app's own settings.json (Bob).
class Prefs : public QObject
{
    Q_OBJECT
    QML_ELEMENT
    QML_SINGLETON
    // system, light or dark
    Q_PROPERTY(QString theme READ theme WRITE setTheme NOTIFY changed)
    // providerID/modelID the user picked, or empty for OpenCode's default
    Q_PROPERTY(QString model READ model WRITE setModel NOTIFY changed)
    // Whether to let the model think before answering, where the model can switch it off
    Q_PROPERTY(bool thinking READ thinking WRITE setThinking NOTIFY changed)
    Q_PROPERTY(QString agent READ agent WRITE setAgent NOTIFY changed)
    Q_PROPERTY(bool sidebar READ sidebar WRITE setSidebar NOTIFY changed)
    // The side panel that is open
    Q_PROPERTY(QString panel READ panel WRITE setPanel NOTIFY changed)
    // Widths set by dragging a splitter, by element ("sidebar", or a side panel's key)
    Q_PROPERTY(QVariantMap widths READ widths NOTIFY changed)

public:
    // Not default-constructible, so QML takes the app's instance from create().
    explicit Prefs(QObject *parent);

    static Prefs *create(QQmlEngine *, QJSEngine *);
    static void setInstance(Prefs *prefs);

    QString theme() const;
    void setTheme(const QString &theme);
    QString model() const;
    void setModel(const QString &model);
    bool thinking() const;
    void setThinking(bool thinking);
    QString agent() const;
    void setAgent(const QString &agent);
    bool sidebar() const;
    void setSidebar(bool sidebar);
    QString panel() const;
    void setPanel(const QString &panel);
    QVariantMap widths() const;

    // Remembers a width; a negative one goes back to the default.
    Q_INVOKABLE void setWidth(const QString &key, int width);

signals:
    void changed();

private:
    void write(const QString &key, const QVariant &value);

    QSettings m_settings;
};
