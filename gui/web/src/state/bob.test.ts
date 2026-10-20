import { beforeEach, describe, expect, it, vi } from "vitest";
import type { BobConnection } from "../api/types";

vi.mock("../platform", () => ({
  joinBob: vi.fn(),
  leaveBob: vi.fn(async () => ({ state: "idle", error: null })),
}));

const platform = await import("../platform");
const { useStore } = await import("./store");

const connected: BobConnection[] = [];

beforeEach(() => {
  vi.mocked(platform.joinBob).mockReset();
  vi.mocked(platform.leaveBob).mockClear();
  connected.length = 0;
  useStore.setState({
    phase: "setup",
    bob: { state: "idle", error: null },
    connectError: undefined,
    notice: undefined,
    connection: undefined,
    // stands in for the OpenCode connection, which needs a server
    async connect(connection) {
      connected.push(connection);
      useStore.setState({ phase: "ready", connection });
    },
  });
});

const joined = (runningConfig = "/w/bob.toml") =>
  ({
    state: "joined",
    connection: { url: "http://127.0.0.1:4096", password: "p", container: "bob-1" },
    container: "bob-1",
    config: "/w/bob.toml",
    runningConfig,
  }) as const;

describe("joinBob", () => {
  it("connects to the bob Rust joined", async () => {
    vi.mocked(platform.joinBob).mockResolvedValue(joined());
    await useStore.getState().joinBob();
    expect(platform.joinBob).toHaveBeenCalledTimes(1);
    expect(connected).toEqual([{ url: "http://127.0.0.1:4096", password: "p", container: "bob-1" }]);
    expect(useStore.getState().bob.state).toBe("joined");
    expect(useStore.getState().notice).toBeUndefined();
  });

  it("says so when bob runs with another bob.toml", async () => {
    vi.mocked(platform.joinBob).mockResolvedValue(joined("/other/bob.toml"));
    await useStore.getState().joinBob();
    expect(useStore.getState().notice?.text).toContain("/other/bob.toml");
  });

  it("shows why a join failed and connects to nothing", async () => {
    vi.mocked(platform.joinBob).mockResolvedValue({ state: "idle", error: "start the docker engine first" });
    await useStore.getState().joinBob();
    expect(connected).toEqual([]);
    expect(useStore.getState().phase).toBe("setup");
    expect(useStore.getState().connectError).toBe("start the docker engine first");
  });

  it("refuses a connection without a password", async () => {
    vi.mocked(platform.joinBob).mockResolvedValue({ ...joined(), connection: { url: "http://x" } });
    await useStore.getState().joinBob();
    expect(connected).toEqual([]);
    expect(useStore.getState().connectError).toMatch(/without URL or password/);
  });
});

describe("leaveBob", () => {
  it("ends the lease and goes back to the connect screen", async () => {
    vi.mocked(platform.joinBob).mockResolvedValue(joined());
    await useStore.getState().joinBob();
    await useStore.getState().leaveBob();
    expect(platform.leaveBob).toHaveBeenCalledTimes(1);
    expect(useStore.getState().bob).toEqual({ state: "idle", error: null });
    expect(useStore.getState().phase).toBe("setup");
    expect(useStore.getState().connection).toBeUndefined();
  });
});
