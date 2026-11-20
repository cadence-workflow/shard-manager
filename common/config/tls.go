package config

import (
	"crypto/tls"
	"crypto/x509"
	"os"
)

type (
	// TLS describe TLS configuration
	TLS struct {
		Enabled bool `yaml:"enabled"`

		// CertPath and KeyPath are optional depending on server
		// config, but both fields must be omitted to avoid using a
		// client certificate
		CertFile string `yaml:"certFile"`
		KeyFile  string `yaml:"keyFile"`

		CaFile  string   `yaml:"caFile"` // optional depending on server config
		CaFiles []string `yaml:"caFiles"`
		// If you want to verify the hostname and server cert (like a wildcard for cass cluster) then you should turn this on
		// This option is basically the inverse of InSecureSkipVerify
		// See InSecureSkipVerify in http://golang.org/pkg/crypto/tls/ for more info
		EnableHostVerification bool `yaml:"enableHostVerification"`

		// Set RequireClientAuth to true if mutual TLS is desired.
		// In this mode, client will need to present their certificate which is signed by CA
		// that is specified with server "caFile"/"caFiles" or stored in server host level CA store.
		RequireClientAuth bool `yaml:"requireClientAuth"`

		ServerName string `yaml:"serverName"`
	}
)

// ToTLSConfig converts Cadence TLS config to crypto/tls.Config
func (config TLS) ToTLSConfig() (*tls.Config, error) {
	if !config.Enabled {
		return nil, nil
	}

	// Setup base TLS config
	// EnableHostVerification is a secure flag vs insecureSkipVerify is insecure so inverse the value
	tlsConfig := &tls.Config{
		InsecureSkipVerify: !config.EnableHostVerification,
	}

	// Setup server name
	if config.ServerName != "" {
		tlsConfig.ServerName = config.ServerName
	}

	// Load CA certs
	caFiles := config.CaFiles
	if config.CaFile != "" {
		caFiles = append(caFiles, config.CaFile)
	}

	if len(caFiles) > 0 {
		caCertPool := x509.NewCertPool()
		for _, caFile := range caFiles {
			caCert, err := os.ReadFile(caFile)
			if err != nil {
				return nil, err
			}
			caCertPool.AppendCertsFromPEM(caCert)
		}
		tlsConfig.RootCAs = caCertPool
	}

	// Enable mutual TLS
	if config.RequireClientAuth {
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		tlsConfig.ClientCAs = tlsConfig.RootCAs
	}

	// Load client cert
	if config.CertFile != "" && config.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(config.CertFile, config.KeyFile)
		if err != nil {
			return nil, err
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	return tlsConfig, nil
}
