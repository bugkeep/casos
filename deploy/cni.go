package deploy

import "fmt"

const legacyBridgeCNIConfigPath = "/etc/cni/net.d/10-casos-bridge.conflist"

func bridgeCNIConfig(podCIDR string) string {
	return fmt.Sprintf(`{
  "cniVersion": "1.0.0",
  "name": "casos-bridge",
  "plugins": [
    {
      "type": "bridge",
      "bridge": "cni0",
      "isGateway": true,
      "ipMasq": true,
      "ipam": {
        "type": "host-local",
        "ranges": [[{"subnet": %q}]],
        "routes": [{"dst": "0.0.0.0/0"}]
      }
    },
    {"type": "portmap", "capabilities": {"portMappings": true}},
    {"type": "loopback"}
  ]
}
`, podCIDR)
}

// flannelCNIConfig keeps the per-node Pod network on cni0 while delegating
// cross-node routing and subnet allocation to flanneld.
func flannelCNIConfig() string {
	return `{
  "cniVersion": "0.3.1",
  "name": "cbr0",
  "plugins": [
    {
      "type": "flannel",
      "delegate": {
        "bridge": "cni0",
        "hairpinMode": true,
        "isDefaultGateway": true,
        "ipMasq": true
      }
    },
    {
      "type": "portmap",
      "capabilities": {"portMappings": true}
    },
  ]
}
`
}

func removeLegacyBridgeCNICommand() string {
	return fmt.Sprintf("rm -f %s", shellSingleQuote(legacyBridgeCNIConfigPath))
}
