import ipaddress
import socket
import unittest
from unittest.mock import patch

from network import get_rules, resolve_endpoints


class NetworkTests(unittest.TestCase):
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
