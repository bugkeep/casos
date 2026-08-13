package object

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
	"xorm.io/xorm"
)

const (
	localAdminID         = 1
	localAdminBCryptCost = 12
)

var (
	ErrLocalAdminExists   = errors.New("local administrator is already initialized")
	ErrInvalidCredentials = errors.New("invalid username or password")
)

// LocalAdmin is deliberately a singleton. Enterprise identity and multi-user
// administration remain the responsibility of the optional Casdoor provider.
type LocalAdmin struct {
	Id             int       `xorm:"pk" json:"id"`
	Name           string    `xorm:"varchar(64) notnull unique" json:"name"`
	PasswordHash   string    `xorm:"varchar(100) notnull" json:"-"`
	SessionVersion int64     `xorm:"notnull default 1" json:"-"`
	CreatedTime    time.Time `xorm:"created" json:"-"`
	UpdatedTime    time.Time `xorm:"updated" json:"-"`
}

func validateLocalAdminCredentials(username, password string) (string, error) {
	username = strings.TrimSpace(username)
	if utf8.RuneCountInString(username) < 3 || utf8.RuneCountInString(username) > 64 {
		return "", errors.New("username must contain 3 to 64 characters")
	}
	if len([]byte(password)) < 12 || len([]byte(password)) > 72 {
		return "", errors.New("password must contain 12 to 72 bytes")
	}
	return username, nil
}

func hashLocalAdminPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), localAdminBCryptCost)
	return string(hash), err
}

func checkLocalAdminPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func LocalAdminExists() (bool, error) {
	if ormer == nil || ormer.Engine == nil {
		return false, errors.New("database adapter is not initialized")
	}
	return ormer.Engine.ID(localAdminID).Exist(new(LocalAdmin))
}

func GetLocalAdmin() (*LocalAdmin, error) {
	if ormer == nil || ormer.Engine == nil {
		return nil, errors.New("database adapter is not initialized")
	}
	admin := &LocalAdmin{Id: localAdminID}
	found, err := ormer.Engine.ID(localAdminID).Get(admin)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrInvalidCredentials
	}
	return admin, nil
}

func SetupLocalAdmin(username, password string) (*LocalAdmin, error) {
	username, err := validateLocalAdminCredentials(username, password)
	if err != nil {
		return nil, err
	}
	hash, err := hashLocalAdminPassword(password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	admin := &LocalAdmin{Id: localAdminID, Name: username, PasswordHash: hash, SessionVersion: 1}

	_, err = ormer.Engine.Transaction(func(session *xorm.Session) (interface{}, error) {
		exists, txErr := session.ID(localAdminID).Exist(new(LocalAdmin))
		if txErr != nil {
			return nil, txErr
		}
		if exists {
			return nil, ErrLocalAdminExists
		}
		if _, txErr = session.Insert(admin); txErr != nil {
			if isDuplicateLocalAdminError(txErr) {
				return nil, ErrLocalAdminExists
			}
			if existsAfter, _ := session.ID(localAdminID).Exist(new(LocalAdmin)); existsAfter {
				return nil, ErrLocalAdminExists
			}
			return nil, txErr
		}
		return admin, nil
	})
	if err != nil {
		return nil, err
	}
	return admin, nil
}

func isDuplicateLocalAdminError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "duplicate") ||
		strings.Contains(message, "unique constraint") ||
		strings.Contains(message, "unique failed")
}

func AuthenticateLocalAdmin(username, password string) (*LocalAdmin, error) {
	admin, err := GetLocalAdmin()
	if err != nil || !strings.EqualFold(strings.TrimSpace(username), admin.Name) || !checkLocalAdminPassword(admin.PasswordHash, password) {
		return nil, ErrInvalidCredentials
	}
	return admin, nil
}

func ChangeLocalAdminPassword(currentPassword, newPassword string) (*LocalAdmin, error) {
	return updateLocalAdminPassword(currentPassword, newPassword, true)
}

func RecoverLocalAdminPassword(newPassword string) (*LocalAdmin, error) {
	return updateLocalAdminPassword("", newPassword, false)
}

func updateLocalAdminPassword(currentPassword, newPassword string, verifyCurrent bool) (*LocalAdmin, error) {
	result, err := ormer.Engine.Transaction(func(session *xorm.Session) (interface{}, error) {
		admin := &LocalAdmin{Id: localAdminID}
		found, txErr := session.ID(localAdminID).ForUpdate().Get(admin)
		if txErr != nil {
			return nil, txErr
		}
		if !found || (verifyCurrent && !checkLocalAdminPassword(admin.PasswordHash, currentPassword)) {
			return nil, ErrInvalidCredentials
		}
		if _, txErr = validateLocalAdminCredentials(admin.Name, newPassword); txErr != nil {
			return nil, txErr
		}
		hash, txErr := hashLocalAdminPassword(newPassword)
		if txErr != nil {
			return nil, fmt.Errorf("hash password: %w", txErr)
		}
		admin.PasswordHash = hash
		admin.SessionVersion++
		if _, txErr = session.ID(localAdminID).Cols("password_hash", "session_version", "updated_time").Update(admin); txErr != nil {
			return nil, txErr
		}
		return admin, nil
	})
	if err != nil {
		return nil, err
	}
	return result.(*LocalAdmin), nil
}

func IsLocalSessionCurrent(version int64) bool {
	admin, err := GetLocalAdmin()
	return err == nil && admin.SessionVersion == version
}
