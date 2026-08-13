package auth

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/gob"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

type User struct {
	Id          string `json:"id"`
	Owner       string `json:"owner"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Avatar      string `json:"avatar"`
	IsAdmin     bool   `json:"isAdmin"`
	Provider    string `json:"provider"`
}

type SessionIdentity struct {
	User           User
	SessionVersion int64
	CSRFToken      string
}

func init() {
	gob.Register(SessionIdentity{})
	gob.Register(User{})
	gob.Register(casdoorsdk.Claims{})
}

func NewCSRFToken() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

// NormalizeSession supports sessions written by earlier Casdoor-only builds.
// Callers should replace legacy values with the returned identity so access
// tokens are removed from long-lived file sessions.
func NormalizeSession(value interface{}) (*SessionIdentity, bool) {
	switch session := value.(type) {
	case SessionIdentity:
		return &session, false
	case *SessionIdentity:
		if session == nil {
			return nil, false
		}
		copy := *session
		return &copy, false
	case casdoorsdk.Claims:
		return FromCasdoorUser(&session.User), true
	case *casdoorsdk.Claims:
		if session == nil {
			return nil, false
		}
		return FromCasdoorUser(&session.User), true
	default:
		return nil, false
	}
}

func FromCasdoorUser(user *casdoorsdk.User) *SessionIdentity {
	if user == nil {
		return nil
	}
	return &SessionIdentity{User: User{
		Id:          user.Id,
		Owner:       user.Owner,
		Name:        user.Name,
		DisplayName: user.DisplayName,
		Avatar:      user.Avatar,
		IsAdmin:     user.IsAdmin,
		Provider:    "casdoor",
	}}
}
