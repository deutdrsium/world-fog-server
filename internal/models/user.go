package models

import "github.com/go-webauthn/webauthn/webauthn"

// User implements the webauthn.User interface.
type User struct {
	ID          string
	DisplayName string
	Credentials []webauthn.Credential
}

func (u *User) WebAuthnID() []byte                         { return []byte(u.ID) }
func (u *User) WebAuthnName() string                       { return u.ID }
func (u *User) WebAuthnDisplayName() string                { return u.DisplayName }
func (u *User) WebAuthnCredentials() []webauthn.Credential { return u.Credentials }
func (u *User) WebAuthnIcon() string                       { return "" } // deprecated, required by interface
