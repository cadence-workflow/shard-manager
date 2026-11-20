package config

import (
	"errors"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

type (
	Authorization struct {
		OAuthAuthorizer OAuthAuthorizer `yaml:"oauthAuthorizer"`
		NoopAuthorizer  NoopAuthorizer  `yaml:"noopAuthorizer"`
	}

	NoopAuthorizer struct {
		Enable bool `yaml:"enable"`
	}

	OAuthAuthorizer struct {
		Enable bool `yaml:"enable"`
		// Max of TTL in the claim
		MaxJwtTTL int64 `yaml:"maxJwtTTL"`
		// Credentials to verify/create the JWT using public/private keys
		JwtCredentials *JwtCredentials `yaml:"jwtCredentials"`
		// Provider
		Provider *OAuthProvider `yaml:"provider"`
	}

	JwtCredentials struct {
		// support: RS256 (RSA using SHA256)
		Algorithm string `yaml:"algorithm"`
		// Public Key Path for verifying JWT token passed in from external clients
		PublicKey string `yaml:"publicKey"`
	}

	// OAuthProvider is used to validate tokens provided by 3rd party Identity Provider service
	OAuthProvider struct {
		JWKSURL             string `yaml:"jwksURL"`
		GroupsAttributePath string `yaml:"groupsAttributePath"`
		AdminAttributePath  string `yaml:"adminAttributePath"`
	}
)

// Validate validates the persistence config
func (a *Authorization) Validate() error {
	if a.OAuthAuthorizer.Enable && a.NoopAuthorizer.Enable {
		return fmt.Errorf("[AuthorizationConfig] More than one authorizer is enabled")
	}

	if a.OAuthAuthorizer.Enable {
		if err := a.validateOAuth(); err != nil {
			return err
		}
	}

	return nil
}

func (a *Authorization) validateOAuth() error {
	oauthConfig := a.OAuthAuthorizer

	if oauthConfig.MaxJwtTTL <= 0 {
		return fmt.Errorf("[OAuthConfig] MaxTTL must be greater than 0")
	}

	if oauthConfig.JwtCredentials == nil && oauthConfig.Provider == nil {
		return errors.New("jwtCredentials or provider must be provided")
	}

	if oauthConfig.JwtCredentials != nil {
		if oauthConfig.JwtCredentials.PublicKey == "" {
			return fmt.Errorf("[OAuthConfig] PublicKey can't be empty")
		}

		if oauthConfig.JwtCredentials.Algorithm != jwt.SigningMethodRS256.Name {
			return fmt.Errorf("[OAuthConfig] The only supported Algorithm is RS256")
		}
	}

	if oauthConfig.Provider != nil {
		if oauthConfig.Provider.JWKSURL == "" {
			return fmt.Errorf("[OAuthConfig] JWKSURL is not set")
		}
	}

	return nil
}
