"""Install a fixed outbound allowlist before the agent starts; fail closed."""

import ipaddress
import json
import os
import socket
import subprocess
from urllib.parse import urlsplit


def require_seccomp():
    with open("/proc/self/status") as file:
        for line in file:
            if line.startswith("Seccomp:") and line.split()[1:] == ["2"]:
                return
    raise RuntimeError("seccomp filtering is required; enable the engine's default profile")


def resolve_endpoints(urls):
    allowed = set()
    hosts = set()
    for value in urls:
        url = urlsplit(value)
        if (
            url.scheme not in ("http", "https")
            or not url.hostname
            or url.username is not None
            or url.password is not None
        ):
            raise ValueError("expected an HTTP(S) endpoint without credentials")
        port = url.port
        if port is None:
            port = 443 if url.scheme == "https" else 80
        if not 1 <= port <= 65535:
            raise ValueError("invalid endpoint port")
        for _, _, _, _, address in socket.getaddrinfo(url.hostname, port, type=socket.SOCK_STREAM):
            ip = ipaddress.ip_address(address[0])
            if isinstance(ip, ipaddress.IPv6Address) and ip.ipv4_mapped is not None:
                ip = ip.ipv4_mapped
            if ip.is_loopback:
                raise ValueError(
                    "remote endpoints cannot use loopback; use a reachable host address"
                )
            allowed.add((ip, port))
            hosts.add((str(ip), url.hostname))
    if not allowed:
        raise ValueError("at least one endpoint is required")
    return allowed, hosts


def get_rules(allowed, version):
    loopback = "127.0.0.1/32" if version == 4 else "::1/128"
    rules = [
        "*filter",
        ":INPUT ACCEPT [0:0]",
        ":FORWARD DROP [0:0]",
        ":OUTPUT DROP [0:0]",
        # Only local TCP services; Docker's loopback DNS resolver is excluded.
        f"-A OUTPUT -o lo -d {loopback} -p tcp -j ACCEPT",
        # Replies to incoming OpenCode/MCP connections, never arbitrary new egress.
        "-A OUTPUT -p tcp -m conntrack --ctstate ESTABLISHED --ctdir REPLY -j ACCEPT",
    ]
    for ip, port in sorted(allowed, key=lambda item: (str(item[0]), item[1])):
        if ip.version == version:
            rules.append(f"-A OUTPUT -d {ip} -p tcp --dport {port} -j ACCEPT")
    # IPv6 needs neighbour discovery to reach the explicitly permitted addresses.
    if version == 6:
        rules.extend(
            f"-A OUTPUT -p ipv6-icmp --icmpv6-type {kind} -j ACCEPT" for kind in (135, 136)
        )
    return "\n".join(rules + ["COMMIT", ""])


def main():
    require_seccomp()
    allowed, hosts = resolve_endpoints(json.loads(os.environ["BOB_EGRESS_URLS"]))
    # Pin DNS results for this launch. DNS is blocked once the policy is installed.
    with open("/etc/hosts", "a") as file:
        file.writelines(f"\n{ip} {hostname}\n" for ip, hostname in sorted(hosts))
    for version, tool in ((4, "iptables-restore"), (6, "ip6tables-restore")):
        subprocess.run([tool], input=get_rules(allowed, version), text=True, check=True)


if __name__ == "__main__":
    main()
