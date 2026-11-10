package services

import (
	"testing"
	"golang.org/x/crypto/ssh"
)

func TestSSHKexAlgorithms(t *testing.T) {
	config := &ssh.ClientConfig{
		User: "test",
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		HostKeyAlgorithms: sshHostKeyAlgorithms,
		Config: ssh.Config{
			Ciphers:      sshCiphers,
			KeyExchanges: sshKexAlgorithms,
			MACs:         sshMACs,
		},
	}
	config.SetDefaults()
	t.Logf("KeyExchanges configured: %v", config.KeyExchanges)
	t.Logf("Ciphers configured: %v", config.Ciphers)
	t.Logf("MACs configured: %v", config.MACs)
}
