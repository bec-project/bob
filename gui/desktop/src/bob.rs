//! Finding, joining and leaving bob from the desktop app.
//!
//! bob runs once per user, owned by a broker process. `bob-manager --config bob.toml` makes
//! sure the broker runs, starting it when none does, and prints its socket. The app connects
//! there, says hello and gets bob's connection once bob is up. It holds its lease for as long as
//! it keeps that connection open, so leaving bob, quitting and crashing all end it; the broker
//! stops bob 90 s after the last client is gone.
//!
//! The app holds at most one lease, in its `BobSession`. A watcher reads the lease connection
//! and marks the session lost when the broker closes it (bob stopped or failed). Every change of
//! the session's status is reported to the page.
//!
//! The connect screen also lists every container with bob's `io.bob.launch` label on Docker or
//! Podman; their OpenCode password is read back from the container's environment, which only
//! someone who can already control the engine can do.

use std::io::{BufRead, BufReader, Read, Write};
use std::net::Shutdown;
use std::os::unix::net::UnixStream;
use std::path::{Path, PathBuf};
use std::process::{Command, Stdio};
use std::sync::{Arc, Condvar, Mutex, MutexGuard};
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
}

/// Where the app stands with the user's bob. The page shows it and changes it only through
/// `BobSession::join` and `BobSession::leave`.
#[derive(Serialize, Clone, Debug, PartialEq)]
#[serde(
    tag = "state",
    rename_all = "lowercase",
    rename_all_fields = "camelCase"
)]
pub enum BobStatus {
    /// Not holding bob; `error` says why the last join failed.
    Idle { error: Option<String> },
    /// Joining bob, or starting it with this bob.toml.
    Joining { config: String },
    /// Holding bob. `running_config` is the bob.toml bob runs with: another one than `config`
    /// when another program started it first.
    Joined {
        connection: Value,
        container: String,
        config: String,
        running_config: String,
    },
    /// The broker ended the lease on this container: bob stopped or failed.
    Lost {
        reason: String,
        config: String,
        container: String,
    },
}

type Observer = Box<dyn Fn(&BobStatus) + Send + Sync>;

/// The app's one lease on bob and its status.
pub struct BobSession {
    inner: Mutex<Inner>,
    changed: Condvar,
    observer: Observer,
}

struct Inner {
    status: BobStatus,
    /// The connection to the broker while joining or joined; keeping it open holds the lease.
    lease: Option<UnixStream>,
    /// Bumped by every join and leave, so a join or watcher that was overtaken changes nothing.
    generation: u64,
}

impl BobSession {
    /// A session that reports every status change to `observer`.
    pub fn new(observer: Observer) -> Arc<Self> {
        Arc::new(BobSession {
            inner: Mutex::new(Inner {
                status: BobStatus::Idle { error: None },
                lease: None,
                generation: 0,
            }),
            changed: Condvar::new(),
            observer,
        })
    }

    pub fn status(&self) -> BobStatus {
        self.inner.lock().unwrap().status.clone()
    }

    fn set(&self, inner: &mut MutexGuard<Inner>, status: BobStatus) {
        inner.status = status;
        self.changed.notify_all();
        (self.observer)(&inner.status);
    }

    /// Joins the user's bob, starting it with this bob.toml when none runs, and returns the
    /// status once bob is up or the join failed. Holding bob already, it returns that; while
    /// another join runs, it waits for that one.
    pub fn join(self: &Arc<Self>, binary: &str, config: &str) -> BobStatus {
        match self.begin_join(config, true) {
            Ok(generation) => self.finish_join(generation, binary, config),
            Err(status) => status,
        }
    }

    /// Starts joining like `join` but returns at once; the status reports the outcome. The
    /// session is joining when this returns, so the page never sees it idle in between.
    pub fn join_in_background(self: &Arc<Self>, binary: &str, config: &str) {
        if let Ok(generation) = self.begin_join(config, false) {
            let (session, binary, config) =
                (Arc::clone(self), binary.to_string(), config.to_string());
            std::thread::spawn(move || session.finish_join(generation, &binary, &config));
        }
    }

    /// Marks the session joining and returns the join's generation, or the status to return
    /// instead: the lease held already, or (when `wait` is set) the end of a join in progress.
    fn begin_join(&self, config: &str, wait: bool) -> Result<u64, BobStatus> {
        let mut inner = self.inner.lock().unwrap();
        if matches!(inner.status, BobStatus::Joining { .. }) {
            if !wait {
                return Err(inner.status.clone());
            }
            let inner = self
                .changed
                .wait_while(inner, |i| matches!(i.status, BobStatus::Joining { .. }))
                .unwrap();
            return Err(inner.status.clone());
        }
        if matches!(inner.status, BobStatus::Joined { .. }) {
            return Err(inner.status.clone());
        }
        inner.generation += 1;
        let generation = inner.generation;
        self.set(
            &mut inner,
            BobStatus::Joining {
                config: config.to_string(),
            },
        );
        Ok(generation)
    }

    fn finish_join(self: &Arc<Self>, generation: u64, binary: &str, config: &str) -> BobStatus {
        let result = self.connect(binary, config, generation);
        let mut inner = self.inner.lock().unwrap();
        if inner.generation != generation {
            // left while joining: the lease, if any, is already shut down
            return inner.status.clone();
        }
        match result {
            Ok(joined) => {
                let Some(lease) = inner.lease.as_ref().and_then(|l| l.try_clone().ok()) else {
                    inner.lease = None;
                    self.set(
                        &mut inner,
                        BobStatus::Idle {
                            error: Some("lost the connection to bob's broker".into()),
                        },
                    );
                    return inner.status.clone();
                };
                self.set(&mut inner, joined);
                self.watch(lease, generation);
            }
            Err(error) => {
                if let Some(lease) = inner.lease.take() {
                    let _ = lease.shutdown(Shutdown::Both);
                }
                self.set(&mut inner, BobStatus::Idle { error: Some(error) });
            }
        }
        inner.status.clone()
    }

    /// Ends the lease (and a join in progress). bob itself stops when no other client holds it.
    pub fn leave(&self) -> BobStatus {
        let mut inner = self.inner.lock().unwrap();
        inner.generation += 1;
        if let Some(lease) = inner.lease.take() {
            let _ = lease.shutdown(Shutdown::Both);
        }
        if inner.status != (BobStatus::Idle { error: None }) {
            self.set(&mut inner, BobStatus::Idle { error: None });
        }
        inner.status.clone()
    }

    fn current(&self, generation: u64) -> bool {
        self.inner.lock().unwrap().generation == generation
    }

    /// Runs `bob-manager`, connects to the broker and waits for bob. The connection is kept in
    /// the session as soon as it is open, so `leave` can end the wait.
    fn connect(&self, binary: &str, config: &str, generation: u64) -> Result<BobStatus, String> {
        let socket = locate(binary, config, &|| !self.current(generation))?;
        let stream = UnixStream::connect(&socket)
            .map_err(|error| format!("cannot reach bob's broker at {socket}: {error}"))?;
        {
            let mut inner = self.inner.lock().unwrap();
            if inner.generation != generation {
                return Err("left before bob was up".into());
            }
            inner.lease = Some(
                stream
                    .try_clone()
                    .map_err(|error| format!("cannot use bob's broker connection: {error}"))?,
            );
        }
        let reply = hello(&stream, config)?;
        let connection = reply
            .get("connection")
            .filter(|c| c.is_object())
            .ok_or("bob's broker sent no connection")?
            .clone();
        let container = connection
            .get("container")
            .and_then(Value::as_str)
            .unwrap_or_default()
            .to_string();
        let running_config = reply
            .get("config")
            .and_then(Value::as_str)
            .unwrap_or(config)
            .to_string();
        Ok(BobStatus::Joined {
            connection,
            container,
            config: config.to_string(),
            running_config,
        })
    }

    /// Waits on the lease until the broker closes it, then marks the session lost unless it was
    /// left or joined again meanwhile.
    fn watch(self: &Arc<Self>, lease: UnixStream, generation: u64) {
        let session = Arc::clone(self);
        std::thread::spawn(move || {
            let reason = watch_lease(lease);
            let mut inner = session.inner.lock().unwrap();
            if inner.generation != generation {
                return;
            }
            inner.lease = None;
            let BobStatus::Joined {
                config, container, ..
            } = inner.status.clone()
            else {
                return;
            };
            session.set(
                &mut inner,
                BobStatus::Lost {
                    reason,
                    config,
                    container,
                },
            );
        });
    }
}

/// Blocks until the lease connection ends and says how.
fn watch_lease(mut lease: UnixStream) -> String {
    let mut buffer = [0u8; 256];
    loop {
        match lease.read(&mut buffer) {
            Ok(0) => return "bob stopped: its broker ended this app's lease".into(),
            Ok(_) => {}
            Err(error) if error.kind() == std::io::ErrorKind::Interrupted => {}
            Err(error) => return format!("lost the connection to bob's broker: {error}"),
        }
    }
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
/// source tree (build/bob-manager, which `npm run bob` builds), or `bob` on the PATH.
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
    })
}

/// Every running bob on Docker and Podman.
pub fn list() -> Vec<RunningBob> {
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
    found
}

fn tail(text: &str, lines: usize) -> String {
    let all: Vec<&str> = text.lines().collect();
    all[all.len().saturating_sub(lines)..].join("\n")
}

/// Runs `bob-manager --config <bob.toml>`, which makes sure the user's broker runs, and returns
/// the socket it prints. Stops early, killing bob-manager, once `cancelled` says so.
fn locate(binary: &str, config: &str, cancelled: &dyn Fn() -> bool) -> Result<String, String> {
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
        .arg("--config")
        .arg(config)
        .current_dir(launch_dir)
        .env("PATH", engine_path())
        .stdin(Stdio::null())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .spawn()
        .map_err(|error| format!("cannot run {}: {error}", binary.display()))?;
    let read_all = |mut pipe: Box<dyn Read + Send>| {
        std::thread::spawn(move || {
            let mut text = String::new();
            let _ = pipe.read_to_string(&mut text);
            text
        })
    };
    let stdout = read_all(Box::new(child.stdout.take().expect("piped stdout")));
    let stderr = read_all(Box::new(child.stderr.take().expect("piped stderr")));
    let status = loop {
        if cancelled() {
            let _ = child.kill();
            let _ = child.wait();
            return Err("left before bob was up".into());
        }
        match child.try_wait() {
            Ok(Some(status)) => break status,
            Ok(None) => std::thread::sleep(Duration::from_millis(50)),
            Err(error) => return Err(format!("cannot wait for {}: {error}", binary.display())),
        }
    };
    let stdout = stdout.join().unwrap_or_default();
    if !status.success() {
        let stderr = stderr.join().unwrap_or_default();
        return Err(format!(
            "bob could not start ({status}):\n{}",
            tail(&stderr, 20)
        ));
    }
    stdout
        .lines()
        .find_map(|line| {
            let value: Value = serde_json::from_str(line).ok()?;
            value.get("socket")?.as_str().map(String::from)
        })
        .ok_or_else(|| format!("bob printed no broker socket:\n{}", tail(&stdout, 20)))
}

/// Says hello on the broker connection and reads its one reply line, waiting for bob to be up.
fn hello(stream: &UnixStream, config: &str) -> Result<Value, String> {
    let hello = json!({"consumer": "bob-gui", "pid": std::process::id(), "config": config});
    writeln!(&*stream, "{hello}")
        .map_err(|error| format!("cannot talk to bob's broker: {error}"))?;
    stream.set_read_timeout(Some(START_TIMEOUT)).ok();
    let mut reply = String::new();
    BufReader::new(stream)
        .read_line(&mut reply)
        .map_err(|error| format!("bob's broker did not answer: {error}"))?;
    stream.set_read_timeout(None).ok();
    let reply: Value = serde_json::from_str(reply.trim())
        .map_err(|_| "bob's broker closed the connection before bob was up".to_string())?;
    if let Some(error) = reply.get("error").and_then(Value::as_str) {
        return Err(error.to_string());
    }
    Ok(reply)
}

#[cfg(test)]
mod tests {
    use super::{list, running_bob, BobSession, BobStatus};
    use serde_json::json;
    use std::io::{BufRead, BufReader, Read, Write};
    use std::os::unix::fs::PermissionsExt;
    use std::os::unix::net::{UnixListener, UnixStream};
    use std::path::{Path, PathBuf};
    use std::sync::{mpsc, Arc, Mutex};
    use std::time::Duration;

    const UP: &str = r#"{"connection":{"url":"http://127.0.0.1:1","password":"p","container":"fake-1"},"config":"CONFIG","consumers":1}"#;

    /// A stand-in for bob-manager.
    fn fake_bob(dir: &Path, script: &str) -> String {
        let path = dir.join("fake-bob");
        std::fs::write(&path, script).unwrap();
        std::fs::set_permissions(&path, std::fs::Permissions::from_mode(0o755)).unwrap();
        path.display().to_string()
    }

    /// A stand-in for bob-manager that prints the socket of a broker the test runs.
    fn printing_socket(dir: &Path, socket: &Path) -> String {
        fake_bob(
            dir,
            &format!(
                "#!/bin/sh\necho '{{\"socket\":\"{}\"}}'\n",
                socket.display()
            ),
        )
    }

    fn temp_dir(name: &str) -> PathBuf {
        let dir = std::env::temp_dir().join(format!("bob-gui-{name}-{}", std::process::id()));
        let _ = std::fs::remove_dir_all(&dir);
        std::fs::create_dir_all(&dir).unwrap();
        std::fs::write(dir.join("bob.toml"), "").unwrap();
        dir
    }

    /// What a fake broker saw from its clients.
    #[derive(Debug)]
    enum Seen {
        Hello(String),
        LeaseEnded,
    }

    /// A broker that answers every client with `reply` (with CONFIG replaced by `config`), or
    /// never answers when `reply` is empty, and reports hellos and ended leases. Sending on the
    /// returned channel closes every open lease, as a broker does when bob stops.
    fn fake_broker(
        socket: &Path,
        reply: &str,
        config: &str,
    ) -> (mpsc::Receiver<Seen>, mpsc::Sender<()>) {
        let listener = UnixListener::bind(socket).unwrap();
        let (tx, rx) = mpsc::channel();
        let (close_tx, close_rx) = mpsc::channel::<()>();
        let reply = reply.replace("CONFIG", config);
        let open: Arc<Mutex<Vec<UnixStream>>> = Arc::default();
        let closer = Arc::clone(&open);
        std::thread::spawn(move || {
            if close_rx.recv().is_ok() {
                for stream in closer.lock().unwrap().drain(..) {
                    let _ = stream.shutdown(std::net::Shutdown::Both);
                }
            }
        });
        std::thread::spawn(move || {
            for stream in listener.incoming() {
                let stream = stream.unwrap();
                open.lock().unwrap().push(stream.try_clone().unwrap());
                let tx = tx.clone();
                let reply = reply.clone();
                std::thread::spawn(move || {
                    let mut reader = BufReader::new(stream.try_clone().unwrap());
                    let mut hello = String::new();
                    reader.read_line(&mut hello).unwrap();
                    let _ = tx.send(Seen::Hello(hello));
                    if !reply.is_empty() {
                        let _ = writeln!(&stream, "{reply}");
                    }
                    let mut rest = String::new();
                    let _ = reader.read_to_string(&mut rest);
                    let _ = tx.send(Seen::LeaseEnded);
                });
            }
        });
        (rx, close_tx)
    }

    /// A session whose status changes the test receives.
    fn session() -> (Arc<BobSession>, mpsc::Receiver<BobStatus>) {
        let (tx, rx) = mpsc::channel();
        let tx = Mutex::new(tx);
        let session = BobSession::new(Box::new(move |status| {
            let _ = tx.lock().unwrap().send(status.clone());
        }));
        (session, rx)
    }

    fn next(rx: &mpsc::Receiver<Seen>) -> Seen {
        rx.recv_timeout(Duration::from_secs(5))
            .expect("the broker saw nothing")
    }

    #[test]
    fn holds_one_lease_until_left() {
        let dir = temp_dir("lease");
        let config = dir.join("bob.toml").display().to_string();
        let socket = dir.join("b.sock");
        let (broker, _close) = fake_broker(&socket, UP, &config);
        let binary = printing_socket(&dir, &socket);
        let (session, statuses) = session();

        let status = session.join(&binary, &config);
        let BobStatus::Joined {
            connection,
            container,
            running_config,
            ..
        } = &status
        else {
            panic!("{status:?}");
        };
        assert_eq!(connection["url"], "http://127.0.0.1:1");
        assert_eq!(container, "fake-1");
        assert_eq!(running_config, &config);
        let Seen::Hello(hello) = next(&broker) else {
            panic!("no hello");
        };
        assert!(hello.contains("\"consumer\":\"bob-gui\""), "{hello}");
        assert!(hello.contains(&config), "{hello}");
        assert!(matches!(
            statuses.recv().unwrap(),
            BobStatus::Joining { .. }
        ));
        assert_eq!(statuses.recv().unwrap(), status);

        // joining again keeps the one lease
        assert_eq!(session.join(&binary, &config), status);
        assert!(broker.recv_timeout(Duration::from_millis(300)).is_err());

        assert_eq!(session.leave(), BobStatus::Idle { error: None });
        assert!(matches!(next(&broker), Seen::LeaseEnded));
        assert_eq!(statuses.recv().unwrap(), BobStatus::Idle { error: None });
        // leaving ends the watcher quietly: no lost status follows
        assert!(statuses.recv_timeout(Duration::from_millis(300)).is_err());
        std::fs::remove_dir_all(&dir).unwrap();
    }

    #[test]
    fn reports_a_lost_bob_and_joins_again() {
        let dir = temp_dir("lost");
        let config = dir.join("bob.toml").display().to_string();
        let socket = dir.join("b.sock");
        let (broker, close) = fake_broker(&socket, UP, &config);
        let binary = printing_socket(&dir, &socket);
        let (session, statuses) = session();
        assert!(matches!(
            session.join(&binary, &config),
            BobStatus::Joined { .. }
        ));
        assert!(matches!(next(&broker), Seen::Hello(_)));
        close.send(()).unwrap();
        let lost = statuses
            .iter()
            .find(|s| matches!(s, BobStatus::Lost { .. }))
            .unwrap();
        assert_eq!(
            lost,
            BobStatus::Lost {
                reason: "bob stopped: its broker ended this app's lease".into(),
                config: config.clone(),
                container: "fake-1".into(),
            }
        );
        assert_eq!(session.status(), lost);
        assert!(matches!(
            session.join(&binary, &config),
            BobStatus::Joined { .. }
        ));
        session.leave();
        std::fs::remove_dir_all(&dir).unwrap();
    }

    #[test]
    fn reports_bob_running_with_another_config() {
        let dir = temp_dir("other");
        let config = dir.join("bob.toml").display().to_string();
        let socket = dir.join("b.sock");
        let (_broker, _close) = fake_broker(&socket, UP, "/elsewhere/bob.toml");
        let binary = printing_socket(&dir, &socket);
        let (session, _statuses) = session();
        let BobStatus::Joined {
            config: asked,
            running_config,
            ..
        } = session.join(&binary, &config)
        else {
            panic!("not joined");
        };
        assert_eq!(asked, config);
        assert_eq!(running_config, "/elsewhere/bob.toml");
        session.leave();
        std::fs::remove_dir_all(&dir).unwrap();
    }

    #[test]
    fn leaving_ends_a_join_in_progress() {
        let dir = temp_dir("cancel");
        let config = dir.join("bob.toml").display().to_string();
        let socket = dir.join("b.sock");
        // a broker that never answers: bob is still starting
        let (broker, _close) = fake_broker(&socket, "", &config);
        let binary = printing_socket(&dir, &socket);
        let (session, statuses) = session();
        let join = {
            let session = Arc::clone(&session);
            std::thread::spawn(move || session.join(&binary, &config))
        };
        assert!(matches!(next(&broker), Seen::Hello(_)));
        assert!(matches!(
            statuses.recv().unwrap(),
            BobStatus::Joining { .. }
        ));
        session.leave();
        assert!(matches!(next(&broker), Seen::LeaseEnded));
        assert_eq!(join.join().unwrap(), BobStatus::Idle { error: None });
        assert_eq!(session.status(), BobStatus::Idle { error: None });
        std::fs::remove_dir_all(&dir).unwrap();
    }

    #[test]
    fn leaving_stops_bob_manager() {
        let dir = temp_dir("locate");
        let config = dir.join("bob.toml").display().to_string();
        // bob-manager still waiting for a broker to answer
        let binary = fake_bob(&dir, "#!/bin/sh\nexec sleep 30\n");
        let (session, statuses) = session();
        let join = {
            let session = Arc::clone(&session);
            std::thread::spawn(move || session.join(&binary, &config))
        };
        assert!(matches!(
            statuses.recv().unwrap(),
            BobStatus::Joining { .. }
        ));
        let started = std::time::Instant::now();
        session.leave();
        assert_eq!(join.join().unwrap(), BobStatus::Idle { error: None });
        assert!(started.elapsed() < Duration::from_secs(5));
        std::fs::remove_dir_all(&dir).unwrap();
    }

    #[test]
    fn a_second_join_waits_for_the_first() {
        let dir = temp_dir("wait");
        let config = dir.join("bob.toml").display().to_string();
        let socket = dir.join("b.sock");
        // bob-manager takes a moment, so the second join finds the first one running
        let binary = fake_bob(
            &dir,
            &format!(
                "#!/bin/sh\nsleep 0.5\necho '{{\"socket\":\"{}\"}}'\n",
                socket.display()
            ),
        );
        let (broker, _close) = fake_broker(&socket, UP, &config);
        let (session, _statuses) = session();
        let first = {
            let (session, binary, config) = (Arc::clone(&session), binary.clone(), config.clone());
            std::thread::spawn(move || session.join(&binary, &config))
        };
        while !matches!(session.status(), BobStatus::Joining { .. }) {
            std::thread::sleep(Duration::from_millis(10));
        }
        let second = session.join(&binary, &config);
        assert_eq!(first.join().unwrap(), second);
        assert!(matches!(second, BobStatus::Joined { .. }));
        assert!(matches!(next(&broker), Seen::Hello(_)));
        assert!(
            broker.recv_timeout(Duration::from_millis(300)).is_err(),
            "one hello only"
        );
        session.leave();
        std::fs::remove_dir_all(&dir).unwrap();
    }

    #[test]
    fn joins_in_the_background() {
        let dir = temp_dir("background");
        let config = dir.join("bob.toml").display().to_string();
        let socket = dir.join("b.sock");
        let (broker, _close) = fake_broker(&socket, UP, &config);
        let binary = printing_socket(&dir, &socket);
        let (session, statuses) = session();
        session.join_in_background(&binary, &config);
        // joining at once, before the broker answered
        assert_eq!(
            session.status(),
            BobStatus::Joining {
                config: config.clone()
            }
        );
        assert!(matches!(
            statuses
                .iter()
                .find(|s| !matches!(s, BobStatus::Joining { .. }))
                .unwrap(),
            BobStatus::Joined { .. }
        ));
        assert!(matches!(next(&broker), Seen::Hello(_)));
        session.leave();
        std::fs::remove_dir_all(&dir).unwrap();
    }

    #[test]
    fn reports_a_start_failure() {
        let dir = temp_dir("fail");
        let config = dir.join("bob.toml").display().to_string();
        let binary = fake_bob(
            &dir,
            "#!/bin/sh\necho 'start the docker engine first' >&2\nexit 1\n",
        );
        let (session, _statuses) = session();
        let BobStatus::Idle { error: Some(error) } = session.join(&binary, &config) else {
            panic!("joined");
        };
        assert!(error.contains("could not start"), "{error}");
        assert!(error.contains("start the docker engine first"), "{error}");
        assert!(matches!(
            session.join(&binary, "/nonexistent/bob.toml"),
            BobStatus::Idle { error: Some(_) }
        ));

        // bob that failed in the broker: the client gets the broker's error
        let socket = dir.join("e.sock");
        let (broker, _close) = fake_broker(&socket, r#"{"error":"no space left on device"}"#, "");
        let binary = printing_socket(&dir, &socket);
        assert_eq!(
            session.join(&binary, &config),
            BobStatus::Idle {
                error: Some("no space left on device".into())
            }
        );
        assert!(matches!(next(&broker), Seen::Hello(_)));
        assert!(matches!(next(&broker), Seen::LeaseEnded));
        std::fs::remove_dir_all(&dir).unwrap();
    }

    #[test]
    fn status_reaches_the_page_as_tagged_json() {
        let joined = BobStatus::Joined {
            connection: json!({"url": "u"}),
            container: "c".into(),
            config: "a".into(),
            running_config: "b".into(),
        };
        assert_eq!(
            serde_json::to_value(&joined).unwrap(),
            json!({"state": "joined", "connection": {"url": "u"}, "container": "c", "config": "a", "runningConfig": "b"})
        );
        assert_eq!(
            serde_json::to_value(BobStatus::Idle { error: None }).unwrap(),
            json!({"state": "idle", "error": null})
        );
    }

    /// Joins (starts) a real bob, finds it among the running containers, leaves it and checks
    /// that the broker removes the container. Needs an engine and an image; use a private
    /// BOB_RUNTIME_DIR so the test does not join your own bob:
    /// BOB_RUNTIME_DIR=$(mktemp -d) BOB_GUI_E2E_BIN=$PWD/../../build/bob-manager
    /// BOB_GUI_E2E_CONFIG=/path/bob.toml cargo test -- --ignored
    #[test]
    #[ignore]
    fn real_bob_lifecycle() {
        let binary = std::env::var("BOB_GUI_E2E_BIN").expect("BOB_GUI_E2E_BIN");
        let config = std::env::var("BOB_GUI_E2E_CONFIG").expect("BOB_GUI_E2E_CONFIG");
        let (session, _statuses) = session();
        let BobStatus::Joined {
            connection,
            container,
            ..
        } = session.join(&binary, &config)
        else {
            panic!("not joined: {:?}", session.status());
        };
        let found = list()
            .into_iter()
            .find(|b| b.container == container)
            .expect("listed");
        assert_eq!(found.password, connection["password"].as_str().unwrap());
        assert_eq!(
            found.url.trim_end_matches('/'),
            connection["url"].as_str().unwrap().trim_end_matches('/')
        );
        session.leave();
        // the broker removes the container once the linger time after the last lease is over
        let deadline = std::time::Instant::now() + Duration::from_secs(150);
        while list().iter().any(|b| b.container == container) {
            assert!(
                std::time::Instant::now() < deadline,
                "container outlived its last lease"
            );
            std::thread::sleep(Duration::from_millis(500));
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
