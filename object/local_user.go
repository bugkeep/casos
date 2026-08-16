package object

import (
	"errors"
	"fmt"
	"strings"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/casosorg/casos/conf"
	"golang.org/x/crypto/bcrypt"
	"xorm.io/xorm"
)

const (
	LocalUserOwner         = "local"
	maxPasswordLengthBytes = 72
	minPasswordLengthBytes = 12
	unknownPasswordHash    = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"
)

var ErrLocalAdminAlreadyInitialized = errors.New("local administrator is already initialized")

type LocalUser struct {
	Owner        string `xorm:"varchar(100) notnull pk" json:"owner"`
	Name         string `xorm:"varchar(100) notnull pk" json:"name"`
	DisplayName  string `xorm:"varchar(100)" json:"displayName"`
	PasswordHash string `xorm:"varchar(150)" json:"-"`
	IsAdmin      bool   `xorm:"notnull default false" json:"isAdmin"`
}

func IsLocalSigninEnabled() bool {
	for _, key := range []string{"casdoorEndpoint", "clientId", "clientSecret", "casdoorOrganization", "casdoorApplication"} {
		if strings.TrimSpace(conf.GetConfigString(key)) != "" {
			return false
		}
	}
	return true
}

func IsCasdoorConfigured() bool {
	for _, key := range []string{"casdoorEndpoint", "clientId", "clientSecret", "casdoorOrganization", "casdoorApplication"} {
		if strings.TrimSpace(conf.GetConfigString(key)) == "" {
			return false
		}
	}
	return true
}

func normalizeLocalUsername(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("username cannot be empty")
	}
	if strings.Contains(name, "/") {
		return "", fmt.Errorf("username cannot contain slash")
	}
	if len(name) > 100 {
		return "", fmt.Errorf("username is too long")
	}
	return name, nil
}

func ValidateLocalPassword(password string) error {
	if len(password) < minPasswordLengthBytes {
		return fmt.Errorf("password cannot be shorter than %d bytes", minPasswordLengthBytes)
	}
	if len(password) > maxPasswordLengthBytes {
		return fmt.Errorf("password cannot be longer than %d bytes", maxPasswordLengthBytes)
	}
	return nil
}

func getLocalUser(name string) (*LocalUser, error) {
	return getLocalUserFromEngine(ormer.Engine, name)
}

func getLocalUserFromEngine(engine *xorm.Engine, name string) (*LocalUser, error) {
	name, err := normalizeLocalUsername(name)
	if err != nil {
		return nil, err
	}
	user := &LocalUser{Owner: LocalUserOwner, Name: name}
	existed, err := engine.Get(user)
	if err != nil || !existed {
		return nil, err
	}
	return user, nil
}

func VerifyLocalUser(name, password string) (*LocalUser, bool, error) {
	user, err := getLocalUser(name)
	if err != nil {
		compareUnknownPassword(password)
		return nil, false, err
	}
	if user == nil || len(password) > maxPasswordLengthBytes {
		compareUnknownPassword(password)
		return nil, false, nil
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return nil, false, nil
	}
	return user, true, nil
}

func compareUnknownPassword(password string) {
	if len(password) <= maxPasswordLengthBytes {
		_ = bcrypt.CompareHashAndPassword([]byte(unknownPasswordHash), []byte(password))
	}
}

func (user *LocalUser) ToSessionUser() casdoorsdk.User {
	displayName := user.DisplayName
	if displayName == "" {
		displayName = user.Name
	}
	return casdoorsdk.User{
		Owner:       LocalUserOwner,
		Name:        user.Name,
		Id:          LocalUserOwner + ":" + user.Name,
		Type:        "local",
		DisplayName: displayName,
		IsAdmin:     user.IsAdmin,
	}
}

func IsLocalAdminInitialized() (bool, error) {
	return isLocalAdminInitialized(ormer.Engine)
}

func isLocalAdminInitialized(engine *xorm.Engine) (bool, error) {
	user, err := getLocalUserFromEngine(engine, "admin")
	return user != nil, err
}

func InitializeLocalAdmin(password string) (*LocalUser, error) {
	return initializeLocalAdmin(ormer.Engine, password)
}

func initializeLocalAdmin(engine *xorm.Engine, password string) (*LocalUser, error) {
	if err := ValidateLocalPassword(password); err != nil {
		return nil, err
	}
	initialized, err := isLocalAdminInitialized(engine)
	if err != nil {
		return nil, err
	}
	if initialized {
		return nil, ErrLocalAdminAlreadyInitialized
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	user := &LocalUser{
		Owner:        LocalUserOwner,
		Name:         "admin",
		DisplayName:  "Admin",
		PasswordHash: string(passwordHash),
		IsAdmin:      true,
	}
	_, err = engine.Insert(user)
	if err != nil {
		if initialized, lookupErr := isLocalAdminInitialized(engine); lookupErr == nil && initialized {
			return nil, ErrLocalAdminAlreadyInitialized
		}
		return nil, err
	}
	return user, nil
}
