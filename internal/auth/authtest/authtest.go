// Package authtest is a software passkey (github.com/descope/virtualwebauthn)
// for tests: unit tests, server tests and integration tests use it to drive
// real WebAuthn ceremonies against Lor.
package authtest

import (
	"encoding/json"
	"fmt"

	"github.com/descope/virtualwebauthn"
)

// Passkey is a software authenticator holding one credential.
type Passkey struct {
	RP   virtualwebauthn.RelyingParty
	Auth virtualwebauthn.Authenticator
	Cred virtualwebauthn.Credential
}

// New returns a passkey for the relying party (rpID, origin). It performs
// user verification unless noUV is set.
func New(rpID, origin string, noUV bool) *Passkey {
	auth := virtualwebauthn.NewAuthenticatorWithOptions(virtualwebauthn.AuthenticatorOptions{UserNotVerified: noUV})
	return &Passkey{
		RP:   virtualwebauthn.RelyingParty{ID: rpID, Name: "Lor", Origin: origin},
		Auth: auth,
		Cred: virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2),
	}
}

// Register answers creation options (PublicKeyCredentialCreationOptionsJSON)
// and remembers the credential.
func (p *Passkey) Register(options []byte) ([]byte, error) {
	opts, err := virtualwebauthn.ParseAttestationOptions(string(options))
	if err != nil {
		return nil, fmt.Errorf("parse attestation options: %w", err)
	}
	resp := virtualwebauthn.CreateAttestationResponse(p.RP, p.Auth, p.Cred, *opts)
	p.Auth.Options.UserHandle = []byte(opts.UserID)
	p.Auth.AddCredential(p.Cred)
	return []byte(resp), nil
}

// Assert answers request options (PublicKeyCredentialRequestOptionsJSON).
func (p *Passkey) Assert(options []byte) ([]byte, error) {
	opts, err := virtualwebauthn.ParseAssertionOptions(string(options))
	if err != nil {
		return nil, fmt.Errorf("parse assertion options: %w", err)
	}
	return []byte(virtualwebauthn.CreateAssertionResponse(p.RP, p.Auth, p.Cred, *opts)), nil
}

// MustJSON marshals v or panics.
func MustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
