import { describe, expect, it } from "vitest";
import { DeviceRef, formatScanCall, parseScanCall, ScanCallError, scanToolArgs } from "./scanCall";

describe("parseScanCall", () => {
  it("reads a call as typed in the BEC client", () => {
    const call = parseScanCall("scans.line_scan(dev.samx, -1, 1.5e0, steps=10, relative=True)");
    expect(call.scanName).toBe("line_scan");
    expect(call.args).toEqual([new DeviceRef("samx"), -1, 1.5]);
    expect(call.kwargs).toEqual({ steps: 10, relative: true });
    expect(scanToolArgs(call)).toEqual({ scan: "line_scan", args: ["samx", -1, 1.5], kwargs: { steps: 10, relative: true } });
  });

  it("accepts the other spellings and nested literals", () => {
    const call = parseScanCall('bec.scans.grid_scan(dev["samx"], -2, 2, 5, dev.samy, -1, 1, 3, exp_time=0.1, md={"sample": "Fe"}, positions=[(1, 2), [3, None]]);');
    expect(scanToolArgs(call).args[0]).toBe("samx");
    expect(call.kwargs.md).toEqual({ sample: "Fe" });
    expect(call.kwargs.positions).toEqual([[1, 2], [3, null]]);
    expect(parseScanCall("acquire()").scanName).toBe("acquire");
  });

  it("round-trips what formatScanCall writes", () => {
    const text = formatScanCall({ scan: "line_scan", args: ["samx", -1, 1], kwargs: { steps: 10, relative: false, name: "it's" } });
    expect(text).toBe("scans.line_scan(dev.samx, -1, 1, steps=10, relative=False, name='it\\'s')");
    expect(scanToolArgs(parseScanCall(text))).toEqual({ scan: "line_scan", args: ["samx", -1, 1], kwargs: { steps: 10, relative: false, name: "it's" } });
  });

  it("refuses anything that is not a literal", () => {
    for (const bad of [
      "",
      "scans.line_scan(os.system('rm -rf /'))",
      "scans.line_scan(dev.samx, x)",
      "scans.line_scan(dev.samx + 1)",
      "scans.line_scan(steps=3, dev.samx)",
      "scans.line_scan(dev.samx",
      "scans.line_scan('unterminated)",
      "other.line_scan()",
      "scans.line_scan() and more",
      "scans.line_scan(__import__('os'))",
    ]) {
      expect(() => parseScanCall(bad), bad).toThrow(ScanCallError);
    }
  });
});
