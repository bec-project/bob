//! Desktop shell for bob's chat GUI. The window shows the same frontend as the browser build;
//! these commands stand in for what the Vite server does there (see gui/web/server/bob-server.ts).

use std::collections::HashMap;
use std::time::Duration;

use serde::{Deserialize, Serialize};
use tauri::{Manager, Url};

mod bob;

const LOOPBACK_HOSTS: [&str; 3] = ["127.0.0.1", "localhost", "[::1]"];
const FORWARD_REQUEST: [&str; 6] = [
    "accept",
    "authorization",
    "content-type",
    "last-event-id",
    "mcp-protocol-version",
    "mcp-session-id",
];
const FORWARD_RESPONSE: [&str; 4] = [
    "cache-control",
    "content-type",
    "mcp-session-id",
    "www-authenticate",
];

#[derive(Deserialize)]
struct RelayRequest {
    url: String,
    method: String,
    headers: HashMap<String, String>,
    body: Option<String>,
}

#[derive(Serialize)]
struct RelayResponse {
    status: u16,
    headers: HashMap<String, String>,
    body: String,
}

/// The connection bob printed, when gui/web/scripts/bob-gui.mjs started this app with BOB_CONNECTION_FILE.
#[tauri::command]
fn bob_connection() -> Option<String> {
    let path = std::env::var_os("BOB_CONNECTION_FILE")?;
    std::fs::read_to_string(path).ok()
}

/// The target when it is a plain-HTTP URL on this machine's loopback interface.
fn loopback_target(raw: &str) -> Result<Url, String> {
    let url = Url::parse(raw).map_err(|error| format!("invalid URL {raw}: {error}"))?;
    let host = url.host_str().unwrap_or_default();
    if url.scheme() != "http" || !LOOPBACK_HOSTS.contains(&host) {
        return Err("only http:// URLs on 127.0.0.1, localhost or [::1] can be relayed".into());
    }
    if !url.username().is_empty() || url.password().is_some() {
        return Err("URLs with credentials are not relayed".into());
    }
    Ok(url)
}

fn relay(request: RelayRequest) -> Result<RelayResponse, String> {
    let url = loopback_target(&request.url)?;
    let config = ureq::Agent::config_builder()
        .http_status_as_error(false)
        .max_redirects(0)
        .timeout_global(Some(Duration::from_secs(120)))
        .build();
    let agent = ureq::Agent::new_with_config(config);
    let headers: Vec<(&str, &str)> = request
        .headers
        .iter()
        .filter(|(name, _)| FORWARD_REQUEST.contains(&name.to_ascii_lowercase().as_str()))
        .map(|(name, value)| (name.as_str(), value.as_str()))
        .collect();
    let result = match request.method.as_str() {
        "POST" => {
            let mut builder = agent.post(url.as_str());
            for (name, value) in &headers {
                builder = builder.header(*name, *value);
            }
            builder.send(request.body.unwrap_or_default().as_bytes())
        }
        "GET" | "DELETE" => {
            let mut builder = if request.method == "GET" {
                agent.get(url.as_str())
            } else {
                agent.delete(url.as_str())
            };
            for (name, value) in &headers {
                builder = builder.header(*name, *value);
            }
            builder.call()
        }
        other => return Err(format!("method {other} is not relayed")),
    };
    let mut response = result.map_err(|error| {
        format!(
            "cannot reach {}: {error}",
            url.origin().ascii_serialization()
        )
    })?;
    let status = response.status().as_u16();
    let mut out_headers = HashMap::new();
    for name in FORWARD_RESPONSE {
        if let Some(value) = response.headers().get(name).and_then(|v| v.to_str().ok()) {
            out_headers.insert(name.to_string(), value.to_string());
        }
    }
    let body = response
        .body_mut()
        .read_to_string()
        .map_err(|error| format!("cannot read the response: {error}"))?;
    Ok(RelayResponse {
        status,
        headers: out_headers,
        body,
    })
}

/// One HTTP request to a loopback server that sends no CORS headers (the BEC MCP server).
#[tauri::command]
async fn loopback_request(request: RelayRequest) -> Result<RelayResponse, String> {
    tauri::async_runtime::spawn_blocking(move || relay(request))
        .await
        .map_err(|error| error.to_string())?
}

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
    // Leases on bob are open connections: they end with the app, however it exits.
    tauri::Builder::default()
        .plugin(tauri_plugin_dialog::init())
        .manage(bob::BobState::default())
        .invoke_handler(tauri::generate_handler![
            bob_connection,
            loopback_request,
            open_external,
            save_download,
            bob_defaults,
            list_bobs,
            start_bob,
            stop_bob
        ])
        .run(tauri::generate_context!())
        .expect("error while running the bob desktop app");
}

#[derive(Serialize)]
struct BobDefaults {
    binary: Option<String>,
    config: Option<String>,
}

/// Where bob and a bob.toml probably are, for the connect screen to suggest.
#[tauri::command]
fn bob_defaults() -> BobDefaults {
    BobDefaults {
        binary: bob::default_binary(),
        config: bob::default_config(),
    }
}

/// bob containers running on Docker or Podman, with their connections.
#[tauri::command]
async fn list_bobs(app: tauri::AppHandle) -> Result<Vec<bob::RunningBob>, String> {
    tauri::async_runtime::spawn_blocking(move || bob::list(&app.state::<bob::BobState>()))
        .await
        .map_err(|error| error.to_string())
}

/// Joins the user's bob, starting it with a bob.toml when none runs. Returns bob's connection.
#[tauri::command]
async fn start_bob(
    app: tauri::AppHandle,
    binary: String,
    config: String,
) -> Result<String, String> {
    tauri::async_runtime::spawn_blocking(move || {
        bob::start(&app.state::<bob::BobState>(), &binary, &config)
    })
    .await
    .map_err(|error| error.to_string())?
}

/// Ends this app's lease on a bob (by container name), or all of them.
#[tauri::command]
fn stop_bob(app: tauri::AppHandle, container: Option<String>) {
    bob::stop(&app.state::<bob::BobState>(), container.as_deref());
}

#[cfg(test)]
mod tests {
    use super::{loopback_target, relay, RelayRequest};
    use std::collections::HashMap;
    use std::io::{BufRead, BufReader, Read, Write};
    use std::net::TcpListener;
    use std::thread;

    /// Serves one canned MCP-style response and returns the raw request it received.
    fn one_shot_server() -> (String, thread::JoinHandle<String>) {
        let listener = TcpListener::bind("127.0.0.1:0").unwrap();
        let url = format!(
            "http://127.0.0.1:{}/mcp",
            listener.local_addr().unwrap().port()
        );
        let handle = thread::spawn(move || {
            let (stream, _) = listener.accept().unwrap();
            let mut reader = BufReader::new(stream);
            let mut head = String::new();
            let mut length = 0;
            loop {
                let mut line = String::new();
                reader.read_line(&mut line).unwrap();
                if let Some(value) = line.to_ascii_lowercase().strip_prefix("content-length:") {
                    length = value.trim().parse().unwrap();
                }
                head.push_str(&line);
                if line == "\r\n" {
                    break;
                }
            }
            let mut body = vec![0; length];
            reader.read_exact(&mut body).unwrap();
            let reply = "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{}}\n\n";
            write!(
                reader.get_mut(),
                "HTTP/1.1 200 OK\r\ncontent-type: text/event-stream\r\nmcp-session-id: abc\r\n\
                 set-cookie: dropped=1\r\ncontent-length: {}\r\nconnection: close\r\n\r\n{reply}",
                reply.len()
            )
            .unwrap();
            head + &String::from_utf8(body).unwrap()
        });
        (url, handle)
    }

    #[test]
    fn relays_a_post_and_forwards_only_mcp_headers() {
        let (url, server) = one_shot_server();
        let headers = HashMap::from([
            ("content-type".to_string(), "application/json".to_string()),
            (
                "accept".to_string(),
                "application/json, text/event-stream".to_string(),
            ),
            ("mcp-session-id".to_string(), "s1".to_string()),
            ("cookie".to_string(), "secret=1".to_string()),
        ]);
        let response = relay(RelayRequest {
            url,
            method: "POST".into(),
            headers,
            body: Some("{\"jsonrpc\":\"2.0\",\"id\":1}".into()),
        })
        .unwrap();
        let request = server.join().unwrap().to_ascii_lowercase();

        assert_eq!(response.status, 200);
        assert_eq!(
            response.headers.get("mcp-session-id").map(String::as_str),
            Some("abc")
        );
        assert!(!response.headers.contains_key("set-cookie"));
        assert!(response.body.contains("\"result\""));
        assert!(request.starts_with("post /mcp http/1.1"));
        assert_eq!(request.matches("\r\naccept:").count(), 1, "{request}");
        assert!(request.contains("accept: application/json, text/event-stream"));
        assert!(request.contains("mcp-session-id: s1"));
        assert!(!request.contains("cookie"));
        assert!(!request.contains("\r\norigin:"));
        assert!(request.ends_with("{\"jsonrpc\":\"2.0\",\"id\":1}"));
    }

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

    #[test]
    fn refuses_other_methods() {
        let request = RelayRequest {
            url: "http://127.0.0.1:9/mcp".into(),
            method: "PUT".into(),
            headers: HashMap::new(),
            body: None,
        };
        assert!(relay(request).is_err());
    }

    #[test]
    fn relays_only_loopback_http() {
        assert!(loopback_target("http://127.0.0.1:8000/mcp").is_ok());
        assert!(loopback_target("http://localhost:8000/mcp").is_ok());
        assert!(loopback_target("http://[::1]:8000/mcp").is_ok());
        assert!(loopback_target("https://127.0.0.1:8000/mcp").is_err());
        assert!(loopback_target("http://example.com/mcp").is_err());
        assert!(loopback_target("http://127.0.0.1.example.com/mcp").is_err());
        assert!(loopback_target("http://user:pw@127.0.0.1/mcp").is_err());
        assert!(loopback_target("file:///etc/passwd").is_err());
    }
}
