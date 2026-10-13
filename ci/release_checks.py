"""Validate release manifests and select the newest published stable version."""

import hashlib
import re
import sys
from pathlib import Path

TARGETS = ("linux_amd64", "linux_arm64", "darwin_amd64", "darwin_arm64")


def version_tuple(version):
    if not re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+", version):
        raise ValueError("Expected a stable semantic version")
    return tuple(map(int, version.split(".")))


def verify_checksums(directory, version):
    version_tuple(version)
    expected = {f"bob-manager_{version}_{target}.tar.gz" for target in TARGETS}
    checksums = {}
    for line in (directory / "SHA256SUMS").read_text().splitlines():
        entry = re.fullmatch(r"([0-9a-fA-F]{64}) [ *](\S+)", line)
        if not entry or entry[2] not in expected or entry[2] in checksums:
            raise ValueError("Checksum manifest must list each expected archive exactly once")
        checksums[entry[2]] = entry[1].lower()
    if set(checksums) != expected:
        raise ValueError("Checksum manifest must list each expected archive exactly once")
    for name, checksum in checksums.items():
        with (directory / name).open("rb") as archive:
            if hashlib.file_digest(archive, "sha256").hexdigest() != checksum:
                raise ValueError(f"Checksum mismatch: {name}")


def is_latest(version, tags):
    candidate = version_tuple(version)
    versions = {
        version_tuple(tag[1:]) for tag in tags if re.fullmatch(r"v[0-9]+\.[0-9]+\.[0-9]+", tag)
    }
    if candidate not in versions:
        raise ValueError("Candidate version is not a published stable release")
    return candidate == max(versions)


def resolve_tag(tags):
    stable = [tag for tag in tags if re.fullmatch(r"v[0-9]+\.[0-9]+\.[0-9]+", tag)]
    if len(stable) > 1:
        raise ValueError("Multiple stable releases tag this commit; refusing ambiguous delivery")
    return stable[0] if stable else ""


def main():
    if len(sys.argv) == 4 and sys.argv[1] == "checksums":
        verify_checksums(Path(sys.argv[2]), sys.argv[3])
    elif len(sys.argv) == 2 and sys.argv[1] == "tag":
        print(resolve_tag(sys.stdin.read().splitlines()))
    elif len(sys.argv) == 3 and sys.argv[1] == "latest":
        print(str(is_latest(sys.argv[2], sys.stdin.read().splitlines())).lower())
    else:
        raise ValueError(
            "Usage: release_checks.py checksums DIRECTORY VERSION | latest VERSION | tag"
        )


if __name__ == "__main__":
    try:
        main()
    except (ValueError, OSError) as error:
        sys.exit(str(error))
