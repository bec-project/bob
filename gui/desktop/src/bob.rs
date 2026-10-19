//! Finding, joining and leaving bob from the desktop app.
//!
//! bob runs once per user, owned by a broker process. `bob-manager --config bob.toml` makes
//! sure the broker runs, starting it when none does, and prints its socket. The app connects
//! there, says hello and gets bob's connection once bob is up. It holds its lease for as long as
//! it keeps that connection open, so leaving bob, quitting and crashing all end it; the broker
//! stops bob 90 s after the last client is gone.
//!
//! The connect screen also lists every container with bob's `io.bob.launch` label on Docker or
//! Podman; their OpenCode password is read back from the container's environment, which only
//! someone who can already control the engine can do.

use std::io::{BufRead, BufReader, Write};
use std::net::Shutdown;
use std::os::unix::net::UnixStream;
use std::path::{Path, PathBuf};
use std::process::{Command, Stdio};
use std::sync::Mutex;
use std::time::Duration;

use serde::Serialize;
use serde_json::{json, Value};

const LAUNCH_LABEL: &str = "io.bob.launch";
const OPENCODE_PORT: &str = "4096/tcp";
/// bob's own startup limit is 90 s, and the broker may first remove the previous container.
const START_TIMEOUT: Duration = Duration::from_secs(180);

/// A bob container found on an engine, with what the GUI needs to connect to it.
#[derive(Serialize, Clone)]
pub struct RunningBob {
    pub url: String,
    pub password: String,
    pub container: String,
    pub runtime: String,
    pub workspace: String,
    pub image: String,
    pub started: String,
    /// The bob.toml of the bob this app started, when it started this one.
    pub owned_config: Option<String>,
}

/// A lease on bob: the open connection to the broker.
struct Owned {
    lease: UnixStream,
    config: String,
    container: String,
}

/// The leases this app holds; they end when the app exits, however it exits.
#[derive(Default)]
pub struct BobState {
    owned: Mutex<Vec<Owned>>,
}

/// GUI apps on macOS start with a minimal PATH; bob and the engines live elsewhere.
pub fn engine_path() -> String {
    let mut dirs: Vec<String> = std::env::var("PATH")
        .unwrap_or_default()
        .split(':')
        .filter(|d| !d.is_empty())
        .map(String::from)
        .collect();
    let home = std::env::var("HOME").unwrap_or_default();
    for extra in [
        "/usr/local/bin".to_string(),
        "/opt/homebrew/bin".to_string(),
        "/opt/podman/bin".to_string(),
        "/usr/bin".to_string(),
        "/bin".to_string(),
        format!("{home}/.docker/bin"),
        format!("{home}/go/bin"),
        format!("{home}/.local/bin"),
    ] {
        if !dirs.contains(&extra) {
            dirs.push(extra);
        }
    }
    dirs.join(":")
}

fn find_in_path(name: &str) -> Option<PathBuf> {
    engine_path()
        .split(':')
        .map(|dir| Path::new(dir).join(name))
        .find(|candidate| is_executable(candidate))
}

fn is_executable(path: &Path) -> bool {
    use std::os::unix::fs::PermissionsExt;
    path.metadata()
        .map(|m| m.is_file() && m.permissions().mode() & 0o111 != 0)
        .unwrap_or(false)
}

/// The bob executable to use when the user has not chosen one: $BOB_BIN, the build in this
/// source tree (build/bob-manager, which `npm run bob` creates), or `bob` on the PATH.
pub fn default_binary() -> Option<String> {
    if let Some(path) = std::env::var_os("BOB_BIN").map(PathBuf::from) {
        if is_executable(&path) {
            return Some(path.display().to_string());
        }
    }
    let repo = Path::new(env!("CARGO_MANIFEST_DIR")).join("../..");
    for candidate in [repo.join("build/bob-manager"), repo.join("bob")] {
        if is_executable(&candidate) {
            return candidate
                .canonicalize()
                .ok()
                .map(|p| p.display().to_string());
        }
    }
    find_in_path("bob").map(|p| p.display().to_string())
}

/// bob.toml beside the source tree, when the app runs from one and the file exists.
pub fn default_config() -> Option<String> {
    let path = Path::new(env!("CARGO_MANIFEST_DIR")).join("../../bob.toml");
    path.canonicalize().ok().map(|p| p.display().to_string())
}

fn engines() -> Vec<(&'static str, PathBuf)> {
    ["docker", "podman"]
        .into_iter()
        .filter_map(|name| find_in_path(name).map(|path| (name, path)))
        .collect()
}

fn output(program: &Path, args: &[&str]) -> Option<String> {
    let out = Command::new(program)
        .args(args)
        .env("PATH", engine_path())
        .stdin(Stdio::null())
        .stderr(Stdio::null())
        .output()
        .ok()?;
    out.status
        .success()
        .then(|| String::from_utf8_lossy(&out.stdout).into_owned())
}

/// The connection details of one `docker inspect` entry, if it is a running bob.
pub fn running_bob(entry: &Value, runtime: &str) -> Option<RunningBob> {
    let config = entry.get("Config")?;
    config.get("Labels")?.get(LAUNCH_LABEL)?;
    let password = config
        .get("Env")?
        .as_array()?
        .iter()
        .filter_map(Value::as_str)
        .find_map(|e| e.strip_prefix("OPENCODE_SERVER_PASSWORD="))?
        .to_string();
    let binding = entry
        .pointer("/NetworkSettings/Ports")?
        .get(OPENCODE_PORT)?
        .as_array()?
        .first()?;
    let port = binding.get("HostPort")?.as_str()?;
    let host = match binding.get("HostIp").and_then(Value::as_str).unwrap_or("") {
        "" | "0.0.0.0" => "127.0.0.1".to_string(),
        "::" => "[::1]".to_string(),
        ip if ip.contains(':') => format!("[{ip}]"),
        ip => ip.to_string(),
    };
    let text = |pointer: &str| {
        entry
            .pointer(pointer)
            .and_then(Value::as_str)
            .unwrap_or("")
            .to_string()
    };
    Some(RunningBob {
        url: format!("http://{host}:{port}"),
        password,
        container: text("/Name").trim_start_matches('/').to_string(),
        runtime: runtime.to_string(),
        workspace: text("/Config/WorkingDir"),
        image: text("/Config/Image"),
        started: text("/State/StartedAt"),
        owned_config: None,
    })
}

/// Every running bob on Docker and Podman.
pub fn list(state: &BobState) -> Vec<RunningBob> {
    let mut found = Vec::new();
    for (runtime, engine) in engines() {
        let filter = format!("label={LAUNCH_LABEL}");
        let Some(ids) = output(&engine, &["ps", "--quiet", "--filter", &filter]) else {
            continue;
        };
        let ids: Vec<&str> = ids.split_whitespace().collect();
        if ids.is_empty() {
            continue;
        }
        let mut args = vec!["inspect"];
        args.extend(ids);
        let Some(json) = output(&engine, &args) else {
            continue;
        };
        if let Ok(Value::Array(entries)) = serde_json::from_str::<Value>(&json) {
            found.extend(entries.iter().filter_map(|e| running_bob(e, runtime)));
        }
    }
    let owned = state.owned.lock().unwrap();
    for bob in &mut found {
        bob.owned_config = owned
            .iter()
            .find(|o| o.container == bob.container)
            .map(|o| o.config.clone());
    }
    found
}

fn tail(text: &str, lines: usize) -> String {
    let all: Vec<&str> = text.lines().collect();
    all[all.len().saturating_sub(lines)..].join("\n")
}

/// Joins the user's bob, starting it with this bob.toml when none runs, and returns its
/// connection line once OpenCode is up. The lease lasts until `stop` or the app exits.
pub fn start(state: &BobState, binary: &str, config: &str) -> Result<String, String> {
    let config_path = Path::new(config);
    if !config_path.is_file() {
        return Err(format!("{config} is not a file"));
    }
    let binary = Path::new(binary);
    if !is_executable(binary) {
        return Err(format!("{} is not an executable", binary.display()));
    }
    // bob resolves relative paths beside the config; host commands start in the launch directory
    let launch_dir = config_path.parent().unwrap_or(Path::new("/"));
    let located = Command::new(binary)
        .arg("--config")
        .arg(config)
        .current_dir(launch_dir)
        .env("PATH", engine_path())
        .stdin(Stdio::null())
        .output()
        .map_err(|error| format!("cannot run {}: {error}", binary.display()))?;
    let stdout = String::from_utf8_lossy(&located.stdout);
    if !located.status.success() {
        let stderr = String::from_utf8_lossy(&located.stderr);
        return Err(format!(
            "bob could not start ({}):\n{}",
            located.status,
            tail(&stderr, 20)
        ));
    }
    let socket = stdout
        .lines()
        .find_map(|line| {
            let value: Value = serde_json::from_str(line).ok()?;
            value.get("socket")?.as_str().map(String::from)
        })
        .ok_or_else(|| format!("bob printed no broker socket:\n{}", tail(&stdout, 20)))?;
    let (line, lease) = lease(&socket, config)?;
    let container = serde_json::from_str::<Value>(&line)
        .ok()
        .and_then(|v| v.get("container").and_then(Value::as_str).map(String::from))
        .unwrap_or_default();
    state.owned.lock().unwrap().push(Owned {
        lease,
        config: config.to_string(),
        container,
    });
    Ok(line)
}

/// Asks the broker for bob: one hello line out, one reply line back. Keeping the returned
/// connection open holds the lease.
fn lease(socket: &str, config: &str) -> Result<(String, UnixStream), String> {
    let mut stream = UnixStream::connect(socket)
        .map_err(|error| format!("cannot reach bob's broker at {socket}: {error}"))?;
    let hello = json!({"consumer": "bob-gui", "pid": std::process::id(), "config": config});
    writeln!(stream, "{hello}").map_err(|error| format!("cannot talk to bob's broker: {error}"))?;
    stream.set_read_timeout(Some(START_TIMEOUT)).ok();
    let mut reply = String::new();
    BufReader::new(&stream)
        .read_line(&mut reply)
        .map_err(|error| format!("bob's broker did not answer: {error}"))?;
    stream.set_read_timeout(None).ok();
    let reply: Value = serde_json::from_str(reply.trim())
        .map_err(|_| "bob's broker closed the connection before bob was up".to_string())?;
    if let Some(error) = reply.get("error").and_then(Value::as_str) {
        return Err(error.to_string());
    }
    let connection = reply
        .get("connection")
        .ok_or("bob's broker sent no connection")?;
    if let Some(running) = reply.get("config").and_then(Value::as_str) {
        if running != config {
            eprintln!(
                "bob already runs for this user with {running}; using it instead of {config}"
            );
        }
    }
    Ok((connection.to_string(), stream))
}

/// Ends this app's leases (all of them, or the one on this container) by closing their
/// connections. bob itself stops when no other client holds it.
pub fn stop(state: &BobState, container: Option<&str>) {
    state.owned.lock().unwrap().retain(|owned| {
        let leave = container.is_none_or(|c| owned.container == c);
        if leave {
            let _ = owned.lease.shutdown(Shutdown::Both);
        }
        !leave
    });
}

#[cfg(test)]
fn owns_any(state: &BobState) -> bool {
    !state.owned.lock().unwrap().is_empty()
}

#[cfg(test)]
mod tests {
    use super::{list, owns_any, running_bob, start, stop, BobState};
    use serde_json::json;
    use std::io::{BufRead, BufReader, Read, Write};
    use std::os::unix::fs::PermissionsExt;
    use std::os::unix::net::UnixListener;
    use std::path::PathBuf;
    use std::sync::mpsc;
    use std::time::Duration;

    /// A stand-in for bob-manager: prints the socket of a broker the test runs.
    fn fake_bob(dir: &std::path::Path, script: &str) -> PathBuf {
        let path = dir.join("fake-bob");
        std::fs::write(&path, script).unwrap();
        std::fs::set_permissions(&path, std::fs::Permissions::from_mode(0o755)).unwrap();
        path
    }

    fn temp_dir(name: &str) -> PathBuf {
        let dir = std::env::temp_dir().join(format!("bob-gui-{name}-{}", std::process::id()));
        std::fs::create_dir_all(&dir).unwrap();
        std::fs::write(dir.join("bob.toml"), "").unwrap();
        dir
    }

    /// A broker that answers one client with `reply` and reports when its lease ends.
    fn fake_broker(socket: &std::path::Path, reply: &'static str) -> mpsc::Receiver<String> {
        let listener = UnixListener::bind(socket).unwrap();
        let (tx, rx) = mpsc::channel();
        std::thread::spawn(move || {
            let (stream, _) = listener.accept().unwrap();
            let mut reader = BufReader::new(stream.try_clone().unwrap());
            let mut hello = String::new();
            reader.read_line(&mut hello).unwrap();
            tx.send(hello).unwrap();
            writeln!(&stream, "{reply}").unwrap();
            let mut rest = String::new();
            let _ = reader.read_to_string(&mut rest);
            tx.send("lease ended".to_string()).unwrap();
        });
        rx
    }

    #[test]
    fn holds_a_lease_until_stopped() {
        let dir = temp_dir("lease");
        let socket = dir.join("b.sock");
        let broker = fake_broker(
            &socket,
            r#"{"connection":{"url":"http://127.0.0.1:1","password":"p","container":"fake-1"},"config":"x","consumers":1}"#,
        );
        let binary = fake_bob(
            &dir,
            &format!(
                "#!/bin/sh\necho '{{\"socket\":\"{}\"}}'\n",
                socket.display()
            ),
        );
        let state = BobState::default();
        let config = dir.join("bob.toml").display().to_string();
        let line = start(&state, binary.to_str().unwrap(), &config).unwrap();
        assert!(line.contains("\"container\":\"fake-1\""), "{line}");
        let hello = broker.recv_timeout(Duration::from_secs(5)).unwrap();
        assert!(hello.contains("\"consumer\":\"bob-gui\""), "{hello}");
        assert!(hello.contains(&config), "{hello}");
        assert!(owns_any(&state));
        stop(&state, Some("other"));
        assert!(owns_any(&state), "leaving another bob must keep this lease");
        assert!(broker.recv_timeout(Duration::from_millis(300)).is_err());
        stop(&state, None);
        assert!(!owns_any(&state));
        assert_eq!(
            broker.recv_timeout(Duration::from_secs(5)).unwrap(),
            "lease ended",
            "closing the connection must end the lease"
        );
        std::fs::remove_dir_all(&dir).unwrap();
    }

    #[test]
    fn reports_a_start_failure() {
        let dir = temp_dir("fail");
        let binary = fake_bob(
            &dir,
            "#!/bin/sh\necho 'start the docker engine first' >&2\nexit 1\n",
        );
        let state = BobState::default();
        let config = dir.join("bob.toml").display().to_string();
        let error = start(&state, binary.to_str().unwrap(), &config).unwrap_err();
        assert!(error.contains("could not start"), "{error}");
        assert!(error.contains("start the docker engine first"), "{error}");
        assert!(!owns_any(&state));
        assert!(start(&state, binary.to_str().unwrap(), "/nonexistent/bob.toml").is_err());

        // bob that failed in the broker: the client gets the broker's error
        let socket = dir.join("e.sock");
        let _broker = fake_broker(&socket, r#"{"error":"no space left on device"}"#);
        let binary = fake_bob(
            &dir,
            &format!(
                "#!/bin/sh\necho '{{\"socket\":\"{}\"}}'\n",
                socket.display()
            ),
        );
        let error = start(&state, binary.to_str().unwrap(), &config).unwrap_err();
        assert_eq!(error, "no space left on device");
        assert!(!owns_any(&state));
        std::fs::remove_dir_all(&dir).unwrap();
    }

    /// Joins (starts) a real bob, finds it among the running containers, ends the lease and
    /// checks that the broker removes the container. Needs an engine and an image; use a
    /// private BOB_RUNTIME_DIR so the test does not join your own bob:
    /// BOB_RUNTIME_DIR=$(mktemp -d) BOB_GUI_E2E_BIN=$PWD/../../build/bob-manager
    /// BOB_GUI_E2E_CONFIG=/path/bob.toml cargo test -- --ignored
    #[test]
    #[ignore]
    fn real_bob_lifecycle() {
        let binary = std::env::var("BOB_GUI_E2E_BIN").expect("BOB_GUI_E2E_BIN");
        let config = std::env::var("BOB_GUI_E2E_CONFIG").expect("BOB_GUI_E2E_CONFIG");
        let state = BobState::default();
        let line = start(&state, &binary, &config).unwrap();
        let connection: serde_json::Value = serde_json::from_str(&line).unwrap();
        let container = connection["container"].as_str().unwrap().to_string();
        let running = list(&state);
        let found = running
            .iter()
            .find(|b| b.container == container)
            .expect("listed");
        assert_eq!(found.password, connection["password"].as_str().unwrap());
        assert_eq!(
            found.url.trim_end_matches('/'),
            connection["url"].as_str().unwrap().trim_end_matches('/')
        );
        assert_eq!(found.owned_config.as_deref(), Some(config.as_str()));
        stop(&state, None);
        // the broker removes the container once the linger time after the last lease is over
        let deadline = std::time::Instant::now() + std::time::Duration::from_secs(150);
        while list(&state).iter().any(|b| b.container == container) {
            assert!(
                std::time::Instant::now() < deadline,
                "container outlived its last lease"
            );
            std::thread::sleep(std::time::Duration::from_millis(500));
        }
    }

    fn inspect(host_ip: &str) -> serde_json::Value {
        json!({
            "Name": "/bob-501-abc",
            "State": {"StartedAt": "2026-10-13T14:44:43Z"},
            "Config": {
                "Image": "localhost/bob",
                "WorkingDir": "/work",
                "Labels": {"io.bob.launch": "x"},
                "Env": ["HOME=/home/opencode", "OPENCODE_SERVER_PASSWORD=s3cret", "API_KEY=k"]
            },
            "NetworkSettings": {"Ports": {"4096/tcp": [{"HostIp": host_ip, "HostPort": "52997"}]}}
        })
    }

    #[test]
    fn reads_the_connection_from_inspect() {
        let bob = running_bob(&inspect("127.0.0.1"), "docker").unwrap();
        assert_eq!(bob.url, "http://127.0.0.1:52997");
        assert_eq!(bob.password, "s3cret");
        assert_eq!(bob.container, "bob-501-abc");
        assert_eq!(bob.workspace, "/work");
        assert_eq!(
            running_bob(&inspect("0.0.0.0"), "docker").unwrap().url,
            "http://127.0.0.1:52997"
        );
    }

    #[test]
    fn skips_containers_that_are_not_bob() {
        let mut other = inspect("127.0.0.1");
        other["Config"]["Labels"] = json!({});
        assert!(running_bob(&other, "docker").is_none());
        let mut unpublished = inspect("127.0.0.1");
        unpublished["NetworkSettings"]["Ports"] = json!({});
        assert!(running_bob(&unpublished, "docker").is_none());
    }
}
