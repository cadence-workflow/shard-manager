package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMultipleAuthEnabled(t *testing.T) {
	cfg := Authorization{
		OAuthAuthorizer: OAuthAuthorizer{
			Enable: true,
		},
		NoopAuthorizer: NoopAuthorizer{
			Enable: true,
		},
	}

	err := cfg.Validate()
	assert.EqualError(t, err, "[AuthorizationConfig] More than one authorizer is enabled")
}

func TestTTLIsZero(t *testing.T) {
	cfg := Authorization{
		OAuthAuthorizer: OAuthAuthorizer{
			Enable:         true,
			JwtCredentials: &JwtCredentials{},
			MaxJwtTTL:      0,
		},
		NoopAuthorizer: NoopAuthorizer{
			Enable: false,
		},
	}

	err := cfg.Validate()
	assert.EqualError(t, err, "[OAuthConfig] MaxTTL must be greater than 0")
}

func TestPublicKeyIsEmpty(t *testing.T) {
	cfg := Authorization{
		OAuthAuthorizer: OAuthAuthorizer{
			Enable: true,
			JwtCredentials: &JwtCredentials{
				Algorithm: "",
				PublicKey: "",
			},
			MaxJwtTTL: 1000000,
		},
		NoopAuthorizer: NoopAuthorizer{
			Enable: false,
		},
	}

	err := cfg.Validate()
	assert.EqualError(t, err, "[OAuthConfig] PublicKey can't be empty")
}

func TestAlgorithmIsInvalid(t *testing.T) {
	cfg := Authorization{
		OAuthAuthorizer: OAuthAuthorizer{
			Enable: true,
			JwtCredentials: &JwtCredentials{
				Algorithm: "SHA256",
				PublicKey: "public",
			},
			MaxJwtTTL: 1000000,
		},
		NoopAuthorizer: NoopAuthorizer{
			Enable: false,
		},
	}

	err := cfg.Validate()
	assert.EqualError(t, err, "[OAuthConfig] The only supported Algorithm is RS256")
}

func TestCorrectValidation(t *testing.T) {
	cfg := Authorization{
		OAuthAuthorizer: OAuthAuthorizer{
			Enable: true,
			JwtCredentials: &JwtCredentials{
				Algorithm: "RS256",
				PublicKey: "public",
			},
			MaxJwtTTL: 1000000,
		},
		NoopAuthorizer: NoopAuthorizer{
			Enable: false,
		},
	}

	err := cfg.Validate()
	assert.NoError(t, err)
}
