import ipaddress
import socket
import unittest
from unittest.mock import mock_open, patch

from network import get_rules, main, require_seccomp, resolve_endpoints


class NetworkTests(unittest.TestCase):
    @patch("network.socket.getaddrinfo")
    def test_invalid_ports_and_empty_credentials_fail_before_resolution(self, lookup):
        for endpoint in (
            "http://model.example:0/",
            "http://model.example:65536/",
            "http://@model.example/",
            "http://:@model.example/",
        ):
            with self.subTest(endpoint=endpoint), self.assertRaises(ValueError):
                resolve_endpoints([endpoint])
        lookup.assert_not_called()

    @patch("network.socket.getaddrinfo")
    def test_mapped_ipv6_addresses_use_ipv4_rules(self, lookup):
        lookup.return_value = [(socket.AF_INET6, 0, 0, "", ("::ffff:192.0.2.1", 8000, 0, 0))]
        allowed, hosts = resolve_endpoints(["http://[::ffff:192.0.2.1]:8000/api/"])
        self.assertEqual(allowed, {(ipaddress.ip_address("192.0.2.1"), 8000)})
        self.assertIn(("192.0.2.1", "::ffff:192.0.2.1"), hosts)
        self.assertIn("-d 192.0.2.1 -p tcp --dport 8000", get_rules(allowed, 4))
        self.assertNotIn("--dport 8000", get_rules(allowed, 6))
        lookup.return_value = [(socket.AF_INET6, 0, 0, "", ("::ffff:127.0.0.1", 8000, 0, 0))]
        with self.assertRaises(ValueError):
            resolve_endpoints(["http://[::ffff:127.0.0.1]:8000/"])

    def test_seccomp_filter_is_required(self):
        for status in ("Seccomp:\t0\n", "Seccomp:\t1\n", "Name:\tpython3\n"):
            with (
                self.subTest(status=status),
                patch("builtins.open", mock_open(read_data=status)),
                self.assertRaisesRegex(RuntimeError, "seccomp filtering is required"),
            ):
                require_seccomp()
        with patch("builtins.open", mock_open(read_data="Name:\tpython3\nSeccomp:\t2\n")):
            require_seccomp()

    def test_missing_seccomp_stops_bootstrap_before_network_setup(self):
        with (
            patch("builtins.open", mock_open(read_data="Seccomp:\t0\n")),
            patch("network.resolve_endpoints") as resolve,
            patch("network.subprocess.run") as install,
        ):
            with self.assertRaisesRegex(RuntimeError, "seccomp filtering is required"):
                main()
            resolve.assert_not_called()
            install.assert_not_called()

    @patch("network.socket.getaddrinfo")
    def test_resolves_all_addresses_and_default_ports(self, lookup):
        lookup.return_value = [
            (socket.AF_INET, 0, 0, "", ("192.0.2.1", 443)),
            (socket.AF_INET6, 0, 0, "", ("2001:db8::1", 443, 0, 0)),
        ]
        allowed, hosts = resolve_endpoints(
            ["https://model.example/agent/", "http://mcp.example:8000/mcp"]
        )
        self.assertEqual(len(allowed), 4)
        self.assertIn((ipaddress.ip_address("2001:db8::1"), 8000), allowed)
        self.assertIn(("192.0.2.1", "model.example"), hosts)
        ipv4 = get_rules(allowed, 4)
        ipv6 = get_rules(allowed, 6)
        for rules in (ipv4, ipv6):
            self.assertIn(":OUTPUT DROP", rules)
            self.assertIn("--ctstate ESTABLISHED --ctdir REPLY", rules)
            self.assertNotIn("-p udp", rules)
            self.assertNotIn("--dport 53", rules)
        self.assertIn("-d 192.0.2.1 -p tcp --dport 443", ipv4)
        self.assertIn("-d 2001:db8::1 -p tcp --dport 8000", ipv6)
        self.assertNotIn("2001:db8", ipv4)
        self.assertNotIn("192.0.2", ipv6)

    @patch("network.socket.getaddrinfo")
    def test_missing_hosts_and_loopback_fail_closed(self, lookup):
        lookup.side_effect = socket.gaierror("unknown host")
        with self.assertRaises(socket.gaierror):
            resolve_endpoints(["https://missing.invalid/"])
        lookup.side_effect = None
        lookup.return_value = [(socket.AF_INET, 0, 0, "", ("127.0.0.1", 8000))]
        with self.assertRaises(ValueError):
            resolve_endpoints(["http://localhost:8000/"])
        with self.assertRaises(ValueError):
            resolve_endpoints([])


if __name__ == "__main__":
    unittest.main()
