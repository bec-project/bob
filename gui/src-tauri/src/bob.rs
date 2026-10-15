//! Finding, joining and leaving bob from the desktop app.
//!
//! The app uses the user's shared bob: it runs `bob --shared`, which attaches to the bob this
//! user already runs or starts it, and holds a lease while that child process lives. The app
//! ends the lease with SIGINT (or by exiting, which the child notices); bob's broker removes the
//! container once the last consumer is gone.
//!
//! The connect screen also lists every container with bob's `io.bob.launch` label on Docker or
//! Podman, for bobs started without --shared; their OpenCode password is read back from the
//! container's environment, which only someone who can already control the engine can do.

use std::collections::VecDeque;
use std::io::{BufRead, BufReader};
use std::path::{Path, PathBuf};
use std::process::{Child, Command, Stdio};
use std::sync::mpsc;
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant};

use serde::Serialize;
use serde_json::Value;

const LAUNCH_LABEL: &str = "io.bob.launch";
const OPENCODE_PORT: &str = "4096/tcp";
const LOG_LINES: usize = 200;
const START_TIMEOUT: Duration = Duration::from_secs(180);
const STOP_TIMEOUT: Duration = Duration::from_secs(90);

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

struct Owned {
    child: Child,
    config: String,
    container: String,
}

/// The bobs this app started; they are stopped when the app quits.
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

fn push_log(log: &Arc<Mutex<VecDeque<String>>>, line: String) {
    let mut log = log.lock().unwrap();
    if log.len() == LOG_LINES {
        log.pop_front();
    }
    log.push_back(line);
}

fn log_tail(log: &Arc<Mutex<VecDeque<String>>>, lines: usize) -> String {
    let log = log.lock().unwrap();
    let skip = log.len().saturating_sub(lines);
    log.iter()
        .skip(skip)
        .cloned()
        .collect::<Vec<_>>()
        .join("\n")
}

/// Joins the user's shared bob, starting it with this bob.toml when none runs, and returns its
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
    let mut child = Command::new(binary)
        .args(["--shared", "--consumer", "bob-gui", "--config"])
        .arg(config)
        .current_dir(launch_dir)
        .env("PATH", engine_path())
        .stdin(Stdio::null())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .spawn()
        .map_err(|error| format!("cannot run {}: {error}", binary.display()))?;

    let log = Arc::new(Mutex::new(VecDeque::new()));
    let (tx, rx) = mpsc::channel::<String>();
    let stdout = child.stdout.take().unwrap();
    let stderr = child.stderr.take().unwrap();
    {
        let log = log.clone();
        std::thread::spawn(move || {
            let mut sent = false;
            for line in BufReader::new(stdout).lines().map_while(Result::ok) {
                let is_connection = !sent
                    && serde_json::from_str::<Value>(&line)
                        .map(|v| v.get("url").is_some() && v.get("password").is_some())
                        .unwrap_or(false);
                if is_connection {
                    sent = true;
                    let _ = tx.send(line);
                } else {
                    push_log(&log, line);
                }
            }
        });
    }
    {
        let log = log.clone();
        std::thread::spawn(move || {
            for line in BufReader::new(stderr).lines().map_while(Result::ok) {
                push_log(&log, line);
            }
        });
    }

    let deadline = Instant::now() + START_TIMEOUT;
    let line = loop {
        if let Ok(line) = rx.recv_timeout(Duration::from_millis(200)) {
            break line;
        }
        if let Ok(Some(status)) = child.try_wait() {
            return Err(format!(
                "bob exited ({status}) before it was ready:\n{}",
                log_tail(&log, 20)
            ));
        }
        if Instant::now() > deadline {
            interrupt(&mut child);
            let _ = wait_for(&mut child, STOP_TIMEOUT);
            return Err(format!(
                "bob was not ready after {} s:\n{}",
                START_TIMEOUT.as_secs(),
                log_tail(&log, 20)
            ));
        }
    };
    let container = serde_json::from_str::<Value>(&line)
        .ok()
        .and_then(|v| v.get("container").and_then(Value::as_str).map(String::from))
        .unwrap_or_default();
    state.owned.lock().unwrap().push(Owned {
        child,
        config: config.to_string(),
        container,
    });
    Ok(line)
}

fn interrupt(child: &mut Child) {
    // SIGINT ends this app's lease, as Ctrl-C does in a terminal; the broker stops bob when it
    // was the last one
    unsafe {
        libc::kill(child.id() as libc::pid_t, libc::SIGINT);
    }
}

fn wait_for(child: &mut Child, timeout: Duration) -> bool {
    let deadline = Instant::now() + timeout;
    while Instant::now() < deadline {
        if let Ok(Some(_)) = child.try_wait() {
            return true;
        }
        std::thread::sleep(Duration::from_millis(200));
    }
    false
}

/// Ends this app's leases (all of them, or the one on this container). bob itself stops when
/// no other consumer holds it. Returns the containers whose lease did not end in time.
pub fn stop(state: &BobState, container: Option<&str>) -> Vec<String> {
    let mut stopping: Vec<Owned> = {
        let mut owned = state.owned.lock().unwrap();
        let (chosen, kept): (Vec<_>, Vec<_>) = owned
            .drain(..)
            .partition(|o| container.is_none_or(|c| o.container == c));
        *owned = kept;
        chosen
    };
    for bob in &mut stopping {
        interrupt(&mut bob.child);
    }
    let mut left = Vec::new();
    for bob in &mut stopping {
        if !wait_for(&mut bob.child, STOP_TIMEOUT) {
            left.push(bob.container.clone());
        }
    }
    left
}

pub fn owns_any(state: &BobState) -> bool {
    !state.owned.lock().unwrap().is_empty()
}

#[cfg(test)]
mod tests {
    use super::{list, owns_any, running_bob, start, stop, BobState};
    use serde_json::json;
    use std::os::unix::fs::PermissionsExt;
    use std::path::PathBuf;

    /// A stand-in for bob: prints a connection line, then waits for SIGINT like bob does.
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

    #[test]
    fn starts_bob_and_stops_it_with_sigint() {
        let dir = temp_dir("start");
        let marker = dir.join("stopped");
        let script = format!(
            "#!/bin/sh\ntrap 'touch {}; exit 0' INT\necho starting >&2\n\
             echo '{{\"url\":\"http://127.0.0.1:1\",\"password\":\"p\",\"container\":\"fake-1\"}}'\n\
             while :; do sleep 0.1; done\n",
            marker.display()
        );
        let binary = fake_bob(&dir, &script);
        let state = BobState::default();
        let config = dir.join("bob.toml").display().to_string();
        let line = start(&state, binary.to_str().unwrap(), &config).unwrap();
        assert!(line.contains("\"container\":\"fake-1\""));
        assert!(owns_any(&state));
        assert!(stop(&state, Some("other")).is_empty());
        assert!(
            owns_any(&state),
            "stopping another container must keep this one"
        );
        assert!(stop(&state, None).is_empty());
        assert!(!owns_any(&state));
        assert!(
            marker.exists(),
            "bob must get SIGINT so it can remove its container"
        );
        std::fs::remove_dir_all(&dir).unwrap();
    }

    #[test]
    fn reports_a_bob_that_exits_before_it_is_ready() {
        let dir = temp_dir("fail");
        let binary = fake_bob(
            &dir,
            "#!/bin/sh\necho 'start the docker engine first' >&2\nexit 1\n",
        );
        let state = BobState::default();
        let config = dir.join("bob.toml").display().to_string();
        let error = start(&state, binary.to_str().unwrap(), &config).unwrap_err();
        assert!(error.contains("exited"), "{error}");
        assert!(error.contains("start the docker engine first"), "{error}");
        assert!(!owns_any(&state));
        assert!(start(&state, binary.to_str().unwrap(), "/nonexistent/bob.toml").is_err());
        std::fs::remove_dir_all(&dir).unwrap();
    }

    /// Joins (starts) a real shared bob, finds it among the running containers, ends the lease
    /// and checks that the broker removes the container. Needs an engine and an image; use a
    /// private BOB_RUNTIME_DIR so the test does not join your own shared bob:
    /// BOB_RUNTIME_DIR=$(mktemp -d) BOB_GUI_E2E_BIN=build/bob-manager
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
        assert!(stop(&state, None).is_empty());
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
